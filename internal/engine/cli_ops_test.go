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

package engine_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"testing"

	"github.com/scrothers/pmmcp/internal/engine"
)

// TestHelperProcess is re-executed as the container CLI by newFakeRunner. It
// prints HELPER_STDOUT/HELPER_STDERR and exits HELPER_EXIT so the CLIRunner
// methods can be exercised without a real docker/podman binary, portably (no
// dependency on shell tools).
func TestHelperProcess(_ *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	fmt.Fprint(os.Stdout, os.Getenv("HELPER_STDOUT"))
	if m := os.Getenv("HELPER_STDERR"); m != "" {
		fmt.Fprint(os.Stderr, m)
	}
	code, _ := strconv.Atoi(os.Getenv("HELPER_EXIT"))
	os.Exit(code)
}

type helperResp struct {
	stdout string
	stderr string
	exit   int
}

// newFakeRunner serves CLI invocations from TestHelperProcess, keyed by
// subcommand (arg[0]).
func newFakeRunner(byCmd map[string]helperResp) engine.CLIRunner {
	return engine.CLIRunner{
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
	}
}

func TestCLIRunnerVersion(t *testing.T) {
	t.Parallel()
	r := newFakeRunner(map[string]helperResp{
		"version": {stdout: `{"Client":{"Version":"27.1.1"},"Server":{"Version":"26.0.2"}}`},
	})
	v, err := r.Version(context.Background())
	if err != nil {
		t.Fatalf("Version: %v", err)
	}
	if v.Client != "27.1.1" || v.Server != "26.0.2" {
		t.Fatalf("Version = %+v", v)
	}

	t.Run("bad json", func(t *testing.T) {
		t.Parallel()
		r := newFakeRunner(map[string]helperResp{"version": {stdout: "not json"}})
		if _, err := r.Version(context.Background()); err == nil {
			t.Fatal("want parse error")
		}
	})
	t.Run("cli error", func(t *testing.T) {
		t.Parallel()
		r := newFakeRunner(map[string]helperResp{"version": {stderr: "cannot connect", exit: 1}})
		if _, err := r.Version(context.Background()); err == nil {
			t.Fatal("want error")
		}
	})
}

func TestCLIRunnerInspect(t *testing.T) {
	t.Parallel()

	t.Run("full", func(t *testing.T) {
		t.Parallel()
		j := `[{"Id":"cid","Name":"/svc","State":{"Status":"exited","Running":false,"ExitCode":2,` +
			`"StartedAt":"2026-08-08T04:00:00.5Z","FinishedAt":"2026-08-08T04:01:00Z"},` +
			`"Config":{"Image":"redis:7","Labels":{"k":"v"}}}]`
		r := newFakeRunner(map[string]helperResp{"inspect": {stdout: j}})
		st, err := r.Inspect(context.Background(), "cid")
		if err != nil {
			t.Fatalf("Inspect: %v", err)
		}
		if st.Name != "svc" || st.Image != "redis:7" || st.State != engine.StateExited || st.ExitCode != 2 {
			t.Fatalf("got %+v", st)
		}
		if st.StartedAt.IsZero() || st.FinishedAt.IsZero() {
			t.Fatalf("times not parsed: %+v", st)
		}
	})

	t.Run("image falls back to top-level when config image empty", func(t *testing.T) {
		t.Parallel()
		j := `[{"Id":"cid","Name":"x","Image":"sha256:abc","State":{"Status":"running","Running":true},"Config":{}}]`
		r := newFakeRunner(map[string]helperResp{"inspect": {stdout: j}})
		st, err := r.Inspect(context.Background(), "cid")
		if err != nil {
			t.Fatalf("Inspect: %v", err)
		}
		if st.Image != "sha256:abc" {
			t.Fatalf("Image = %q, want sha256:abc fallback", st.Image)
		}
	})

	t.Run("unknown state normalizes", func(t *testing.T) {
		t.Parallel()
		j := `[{"Id":"cid","State":{"Status":"weird"}}]`
		r := newFakeRunner(map[string]helperResp{"inspect": {stdout: j}})
		st, _ := r.Inspect(context.Background(), "cid")
		if st.State != engine.StateUnknown {
			t.Fatalf("State = %q, want unknown", st.State)
		}
	})

	t.Run("empty array is not found", func(t *testing.T) {
		t.Parallel()
		r := newFakeRunner(map[string]helperResp{"inspect": {stdout: "[]"}})
		if _, err := r.Inspect(context.Background(), "cid"); !errors.Is(err, engine.ErrNotFound) {
			t.Fatalf("err = %v, want ErrNotFound", err)
		}
	})

	t.Run("bad json", func(t *testing.T) {
		t.Parallel()
		r := newFakeRunner(map[string]helperResp{"inspect": {stdout: "{not-an-array}"}})
		if _, err := r.Inspect(context.Background(), "cid"); err == nil {
			t.Fatal("want parse error")
		}
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()
		r := newFakeRunner(map[string]helperResp{"inspect": {stderr: "no container with name or ID cid", exit: 1}})
		if _, err := r.Inspect(context.Background(), "cid"); !errors.Is(err, engine.ErrNotFound) {
			t.Fatalf("err = %v, want ErrNotFound", err)
		}
	})

	t.Run("empty id", func(t *testing.T) {
		t.Parallel()
		r := newFakeRunner(nil)
		if _, err := r.Inspect(context.Background(), ""); !errors.Is(err, engine.ErrNotFound) {
			t.Fatalf("err = %v, want ErrNotFound", err)
		}
	})
}

func TestCLIRunnerWait(t *testing.T) {
	t.Parallel()
	r := newFakeRunner(map[string]helperResp{"wait": {stdout: "0\n"}})
	code, err := r.Wait(context.Background(), "cid")
	if err != nil || code != 0 {
		t.Fatalf("code=%d err=%v", code, err)
	}
	t.Run("empty id", func(t *testing.T) {
		t.Parallel()
		r := newFakeRunner(nil)
		if _, err := r.Wait(context.Background(), ""); !errors.Is(err, engine.ErrNotFound) {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestCLIRunnerRemove(t *testing.T) {
	t.Parallel()
	r := newFakeRunner(map[string]helperResp{"rm": {}})
	if err := r.Remove(context.Background(), "cid", false); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	empty := newFakeRunner(nil)
	if err := empty.Remove(context.Background(), "", true); !errors.Is(err, engine.ErrNotFound) {
		t.Fatalf("empty id err = %v", err)
	}
}

func TestCLIRunnerPullImage(t *testing.T) {
	t.Parallel()
	r := newFakeRunner(map[string]helperResp{"pull": {}})
	if err := r.PullImage(context.Background(), "alpine"); err != nil {
		t.Fatalf("PullImage: %v", err)
	}
	empty := newFakeRunner(nil)
	if err := empty.PullImage(context.Background(), ""); !errors.Is(err, engine.ErrInvalidSpec) {
		t.Fatalf("empty image err = %v", err)
	}
}

func TestCLIRunnerList(t *testing.T) {
	t.Parallel()
	j := `[{"Id":"cid1","Name":"/a","State":{"Status":"running","Running":true},"Config":{"Image":"img","Labels":{"io.pmmcp.proc_id":"p1"}}}]`
	r := newFakeRunner(map[string]helperResp{
		"ps":      {stdout: "cid1\n"},
		"inspect": {stdout: j},
	})
	got, err := r.List(context.Background(), map[string]string{"io.pmmcp.proc_id": "p1"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 || got[0].ID != "cid1" || got[0].Labels["io.pmmcp.proc_id"] != "p1" {
		t.Fatalf("List = %+v", got)
	}
}

// TestCLIRunnerEnvHonored proves run() passes CLIRunner.Env to the subprocess:
// GO_WANT_HELPER_PROCESS is supplied only via Env, and HELPER_EXIT=1 makes the
// helper fail — so PullImage errors only if the env actually reached the child.
func TestCLIRunnerEnvHonored(t *testing.T) {
	t.Parallel()
	r := engine.CLIRunner{
		Binary:   "docker",
		LookPath: func(string) (string, error) { return "/usr/bin/docker", nil },
		Env:      []string{"GO_WANT_HELPER_PROCESS=1", "HELPER_EXIT=1"},
		Command: func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
			// Deliberately do NOT set cmd.Env here; run() must apply r.Env.
			return exec.CommandContext(ctx, os.Args[0], "-test.run=^TestHelperProcess$")
		},
	}
	if err := r.PullImage(context.Background(), "alpine"); err == nil {
		t.Fatal("PullImage succeeded; CLIRunner.Env was not applied to the subprocess")
	}
}
