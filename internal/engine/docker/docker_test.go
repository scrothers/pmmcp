// Copyright 2026 Steven Crothers
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package docker_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"

	"github.com/scrothers/pmmcp/internal/engine"
	"github.com/scrothers/pmmcp/internal/engine/docker"
)

// TestHelperProcess is re-executed as the "docker" CLI by fakeCLI. It prints
// HELPER_STDOUT/HELPER_STDERR and exits HELPER_EXIT, so tests drive engine
// behavior without a real docker binary — portably across OSes, with no
// dependency on shell tools like true/false/echo.
func TestHelperProcess(_ *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	fmt.Fprint(os.Stdout, os.Getenv("HELPER_STDOUT"))
	if msg := os.Getenv("HELPER_STDERR"); msg != "" {
		fmt.Fprint(os.Stderr, msg)
	}
	code, _ := strconv.Atoi(os.Getenv("HELPER_EXIT"))
	os.Exit(code)
}

// resp is a canned CLI response for one subcommand.
type resp struct {
	stdout string
	stderr string
	exit   int
}

// fakeCLI builds a docker Engine whose CLI invocations are served by
// TestHelperProcess, keyed by subcommand (arg[0]). An unlisted subcommand
// succeeds with empty output.
func fakeCLI(byCmd map[string]resp) *docker.Engine {
	return docker.NewWithCLI(engine.CLIRunner{
		Binary:   "docker",
		LookPath: func(string) (string, error) { return "/usr/bin/docker", nil },
		Command: func(ctx context.Context, _ string, arg ...string) *exec.Cmd {
			sub := ""
			if len(arg) > 0 {
				sub = arg[0]
			}
			r := byCmd[sub]
			c := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestHelperProcess$")
			c.Env = append(os.Environ(),
				"GO_WANT_HELPER_PROCESS=1",
				"HELPER_STDOUT="+r.stdout,
				"HELPER_STDERR="+r.stderr,
				"HELPER_EXIT="+strconv.Itoa(r.exit),
			)
			return c
		},
	})
}

func versionJSON(client, server string) string {
	if server == "" {
		return `{"Client":{"Version":"` + client + `"}}`
	}
	return `{"Client":{"Version":"` + client + `"},"Server":{"Version":"` + server + `"}}`
}

func TestName(t *testing.T) {
	t.Parallel()
	if got := docker.New().Name(); got != "docker" {
		t.Fatalf("Name = %q, want docker", got)
	}
}

// TestAvailable covers docker's daemon-aware availability: the binary must be on
// PATH AND the daemon must answer with a server version.
func TestAvailable(t *testing.T) {
	t.Parallel()

	t.Run("reachable", func(t *testing.T) {
		t.Parallel()
		e := fakeCLI(map[string]resp{"version": {stdout: versionJSON("27.1.1", "27.1.1")}})
		if !e.Available(context.Background()) {
			t.Fatal("Available = false, want true (daemon reachable)")
		}
	})

	t.Run("binary missing", func(t *testing.T) {
		t.Parallel()
		e := docker.NewWithCLI(engine.CLIRunner{
			Binary:   "docker",
			LookPath: func(string) (string, error) { return "", exec.ErrNotFound },
		})
		if e.Available(context.Background()) {
			t.Fatal("Available = true, want false (binary missing)")
		}
	})

	t.Run("daemon down", func(t *testing.T) {
		t.Parallel()
		e := fakeCLI(map[string]resp{"version": {stderr: "Cannot connect to the Docker daemon", exit: 1}})
		if e.Available(context.Background()) {
			t.Fatal("Available = true, want false (daemon down)")
		}
	})

	t.Run("no server version", func(t *testing.T) {
		t.Parallel()
		// Binary and client present, but no Server block (daemon unreachable).
		e := fakeCLI(map[string]resp{"version": {stdout: versionJSON("27.1.1", "")}})
		if e.Available(context.Background()) {
			t.Fatal("Available = true, want false (no server version)")
		}
	})
}

func TestWithHost(t *testing.T) {
	t.Parallel()
	e := docker.New(docker.WithHost("tcp://192.0.2.10:2375"))
	want := "DOCKER_HOST=tcp://192.0.2.10:2375"
	found := false
	for _, kv := range docker.CLIEnv(e) {
		if kv == want {
			found = true
		}
	}
	if !found {
		t.Fatalf("WithHost did not set %q; env=%v", want, docker.CLIEnv(e))
	}
	if env := docker.CLIEnv(docker.New(docker.WithHost(""))); len(env) != 0 {
		t.Fatalf("empty host should not set env, got %v", env)
	}
}

func TestVersion(t *testing.T) {
	t.Parallel()
	e := fakeCLI(map[string]resp{"version": {stdout: versionJSON("27.1.1", "26.0.2")}})
	v, err := e.Version(context.Background())
	if err != nil {
		t.Fatalf("Version: %v", err)
	}
	if v.Client != "27.1.1" || v.Server != "26.0.2" {
		t.Fatalf("Version = %+v, want client 27.1.1 server 26.0.2", v)
	}
}

func TestInspect(t *testing.T) {
	t.Parallel()

	t.Run("exited oom unhealthy", func(t *testing.T) {
		t.Parallel()
		j := `[{"Id":"abc123","Name":"/db","Image":"sha256:deadbeef",` +
			`"State":{"Status":"exited","Running":false,"ExitCode":137,"OOMKilled":true,` +
			`"StartedAt":"2026-08-08T04:00:00Z","FinishedAt":"2026-08-08T04:05:00Z",` +
			`"Health":{"Status":"unhealthy"}},` +
			`"Config":{"Image":"postgres:16","Labels":{"io.pmmcp.proc_id":"proc-1"}}}]`
		e := fakeCLI(map[string]resp{"inspect": {stdout: j}})
		st, err := e.Inspect(context.Background(), "abc123")
		if err != nil {
			t.Fatalf("Inspect: %v", err)
		}
		if st.ID != "abc123" {
			t.Errorf("ID = %q", st.ID)
		}
		if st.Name != "db" {
			t.Errorf("Name = %q, want db (leading slash trimmed)", st.Name)
		}
		if st.Image != "postgres:16" {
			t.Errorf("Image = %q, want config image postgres:16", st.Image)
		}
		if st.State != engine.StateExited {
			t.Errorf("State = %q, want exited", st.State)
		}
		if st.Running {
			t.Error("Running = true, want false")
		}
		if st.ExitCode != 137 || !st.OOMKilled {
			t.Errorf("ExitCode=%d OOMKilled=%v, want 137/true", st.ExitCode, st.OOMKilled)
		}
		if st.Health != "unhealthy" {
			t.Errorf("Health = %q, want unhealthy", st.Health)
		}
		if st.Labels["io.pmmcp.proc_id"] != "proc-1" {
			t.Errorf("Labels = %v", st.Labels)
		}
		if st.StartedAt.IsZero() || st.FinishedAt.IsZero() {
			t.Errorf("times not parsed: started=%v finished=%v", st.StartedAt, st.FinishedAt)
		}
	})

	t.Run("running no health", func(t *testing.T) {
		t.Parallel()
		j := `[{"Id":"c1","Name":"web","State":{"Status":"running","Running":true},"Config":{"Image":"nginx"}}]`
		e := fakeCLI(map[string]resp{"inspect": {stdout: j}})
		st, err := e.Inspect(context.Background(), "c1")
		if err != nil {
			t.Fatalf("Inspect: %v", err)
		}
		if !st.Running || st.State != engine.StateRunning || st.Health != "" {
			t.Fatalf("got %+v", st)
		}
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()
		e := fakeCLI(map[string]resp{"inspect": {stderr: "Error: No such object: nope", exit: 1}})
		if _, err := e.Inspect(context.Background(), "nope"); !errors.Is(err, engine.ErrNotFound) {
			t.Fatalf("err = %v, want ErrNotFound", err)
		}
	})

	t.Run("empty id", func(t *testing.T) {
		t.Parallel()
		if _, err := docker.New().Inspect(context.Background(), ""); !errors.Is(err, engine.ErrNotFound) {
			t.Fatalf("err = %v, want ErrNotFound", err)
		}
	})

	t.Run("unavailable", func(t *testing.T) {
		t.Parallel()
		e := docker.NewWithCLI(engine.CLIRunner{Binary: "docker", LookPath: func(string) (string, error) { return "", exec.ErrNotFound }})
		if _, err := e.Inspect(context.Background(), "c1"); !errors.Is(err, engine.ErrUnavailable) {
			t.Fatalf("err = %v, want ErrUnavailable", err)
		}
	})
}

func TestWait(t *testing.T) {
	t.Parallel()

	t.Run("exit code", func(t *testing.T) {
		t.Parallel()
		e := fakeCLI(map[string]resp{"wait": {stdout: "137\n"}})
		code, err := e.Wait(context.Background(), "cid")
		if err != nil || code != 137 {
			t.Fatalf("code=%d err=%v, want 137/nil", code, err)
		}
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()
		e := fakeCLI(map[string]resp{"wait": {stderr: "no such container: x", exit: 1}})
		if _, err := e.Wait(context.Background(), "x"); !errors.Is(err, engine.ErrNotFound) {
			t.Fatalf("err = %v, want ErrNotFound", err)
		}
	})

	t.Run("unparseable", func(t *testing.T) {
		t.Parallel()
		e := fakeCLI(map[string]resp{"wait": {stdout: "not-a-number"}})
		if _, err := e.Wait(context.Background(), "x"); err == nil {
			t.Fatal("want parse error")
		}
	})

	t.Run("empty id", func(t *testing.T) {
		t.Parallel()
		if _, err := docker.New().Wait(context.Background(), ""); !errors.Is(err, engine.ErrNotFound) {
			t.Fatalf("err = %v, want ErrNotFound", err)
		}
	})
}

func TestRemove(t *testing.T) {
	t.Parallel()

	t.Run("force", func(t *testing.T) {
		t.Parallel()
		e := fakeCLI(map[string]resp{"rm": {}})
		if err := e.Remove(context.Background(), "cid", true); err != nil {
			t.Fatalf("Remove: %v", err)
		}
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()
		e := fakeCLI(map[string]resp{"rm": {stderr: "No such container: gone", exit: 1}})
		if err := e.Remove(context.Background(), "gone", false); !errors.Is(err, engine.ErrNotFound) {
			t.Fatalf("err = %v, want ErrNotFound", err)
		}
	})

	t.Run("empty id", func(t *testing.T) {
		t.Parallel()
		if err := docker.New().Remove(context.Background(), "", false); !errors.Is(err, engine.ErrNotFound) {
			t.Fatalf("err = %v, want ErrNotFound", err)
		}
	})
}

func TestPullImage(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		e := fakeCLI(map[string]resp{"pull": {stdout: "Status: Downloaded newer image for alpine:3\n"}})
		if err := e.PullImage(context.Background(), "alpine:3"); err != nil {
			t.Fatalf("PullImage: %v", err)
		}
	})

	t.Run("empty image", func(t *testing.T) {
		t.Parallel()
		if err := docker.New().PullImage(context.Background(), ""); !errors.Is(err, engine.ErrInvalidSpec) {
			t.Fatalf("err = %v, want ErrInvalidSpec", err)
		}
	})

	t.Run("cli error", func(t *testing.T) {
		t.Parallel()
		e := fakeCLI(map[string]resp{"pull": {stderr: "manifest unknown", exit: 1}})
		if err := e.PullImage(context.Background(), "nope:404"); err == nil {
			t.Fatal("want error")
		}
	})
}

func TestList(t *testing.T) {
	t.Parallel()

	t.Run("populated via inspect", func(t *testing.T) {
		t.Parallel()
		j := `[{"Id":"cid1","Name":"/web","State":{"Status":"running","Running":true},` +
			`"Config":{"Image":"nginx","Labels":{"io.pmmcp.proc_id":"proc-1"}}}]`
		e := fakeCLI(map[string]resp{
			"ps":      {stdout: "cid1\n"},
			"inspect": {stdout: j},
		})
		got, err := e.List(context.Background(), map[string]string{"io.pmmcp.proc_id": "proc-1"})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(got) != 1 || got[0].ID != "cid1" || got[0].Image != "nginx" || got[0].State != engine.StateRunning {
			t.Fatalf("List = %+v", got)
		}
	})

	t.Run("skips container that vanished between ps and inspect", func(t *testing.T) {
		t.Parallel()
		e := fakeCLI(map[string]resp{
			"ps":      {stdout: "gone1\n"},
			"inspect": {stderr: "No such object: gone1", exit: 1},
		})
		got, err := e.List(context.Background(), nil)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(got) != 0 {
			t.Fatalf("List = %+v, want empty (vanished container skipped)", got)
		}
	})

	t.Run("ps error", func(t *testing.T) {
		t.Parallel()
		e := fakeCLI(map[string]resp{"ps": {stderr: "boom", exit: 1}})
		if _, err := e.List(context.Background(), nil); err == nil {
			t.Fatal("want error")
		}
	})
}

func TestRunEmptyImage(t *testing.T) {
	t.Parallel()
	if _, err := docker.New().Run(context.Background(), engine.RunSpec{}); !errors.Is(err, engine.ErrInvalidSpec) {
		t.Fatalf("err = %v, want ErrInvalidSpec", err)
	}
}

func TestRunReturnsID(t *testing.T) {
	t.Parallel()
	e := fakeCLI(map[string]resp{"run": {stdout: "container-xyz\n"}})
	id, err := e.Run(context.Background(), engine.RunSpec{Image: "alpine"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if id != "container-xyz" {
		t.Fatalf("id = %q, want container-xyz", id)
	}
}

func TestStop(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		e := fakeCLI(map[string]resp{"stop": {}})
		if err := e.Stop(context.Background(), "cid-123", time.Second); err != nil {
			t.Fatalf("Stop: %v", err)
		}
	})

	t.Run("cli error", func(t *testing.T) {
		t.Parallel()
		e := fakeCLI(map[string]resp{"stop": {stderr: "boom", exit: 1}})
		if err := e.Stop(context.Background(), "cid-123", time.Second); err == nil {
			t.Fatal("Stop: want error")
		}
	})

	t.Run("unavailable", func(t *testing.T) {
		t.Parallel()
		e := docker.NewWithCLI(engine.CLIRunner{Binary: "docker", LookPath: func(string) (string, error) { return "", exec.ErrNotFound }})
		if err := e.Stop(context.Background(), "cid-123", time.Second); !errors.Is(err, engine.ErrUnavailable) {
			t.Fatalf("err = %v, want ErrUnavailable", err)
		}
	})
}

func TestLogs(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		e := fakeCLI(map[string]resp{"logs": {stdout: "log-line\n"}})
		out, err := e.Logs(context.Background(), "cid-123", 10)
		if err != nil {
			t.Fatalf("Logs: %v", err)
		}
		if out != "log-line\n" {
			t.Fatalf("out = %q, want %q", out, "log-line\n")
		}
	})

	t.Run("unavailable", func(t *testing.T) {
		t.Parallel()
		e := docker.NewWithCLI(engine.CLIRunner{Binary: "docker", LookPath: func(string) (string, error) { return "", exec.ErrNotFound }})
		if _, err := e.Logs(context.Background(), "cid-123", 10); !errors.Is(err, engine.ErrUnavailable) {
			t.Fatalf("err = %v, want ErrUnavailable", err)
		}
	})
}
