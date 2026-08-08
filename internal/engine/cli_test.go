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
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/scrothers/pmmcp/internal/engine"
)

func TestCLIRunnerUnavailable(t *testing.T) {
	t.Parallel()
	r := engine.CLIRunner{
		Binary: "definitely-not-a-real-container-cli-xyz",
		LookPath: func(string) (string, error) {
			return "", exec.ErrNotFound
		},
	}
	ctx := context.Background()
	if r.Available(ctx) {
		t.Fatal("Available = true, want false")
	}
	_, err := r.Run(ctx, engine.RunSpec{Image: "alpine"})
	if !errors.Is(err, engine.ErrUnavailable) {
		t.Fatalf("Run err = %v, want ErrUnavailable", err)
	}
	if err := r.Stop(ctx, "abc", time.Second); !errors.Is(err, engine.ErrUnavailable) {
		t.Fatalf("Stop err = %v, want ErrUnavailable", err)
	}
	if _, err := r.Logs(ctx, "abc", 10); !errors.Is(err, engine.ErrUnavailable) {
		t.Fatalf("Logs err = %v, want ErrUnavailable", err)
	}
}

func TestCLIRunnerEmptyImage(t *testing.T) {
	t.Parallel()
	r := engine.CLIRunner{
		Binary: "podman",
		LookPath: func(string) (string, error) {
			return "/usr/bin/podman", nil
		},
	}
	_, err := r.Run(context.Background(), engine.RunSpec{})
	if !errors.Is(err, engine.ErrInvalidSpec) {
		t.Fatalf("err = %v, want ErrInvalidSpec", err)
	}
}

// captureRunner returns a CLIRunner whose Command hook records the argv and
// then runs a harmless command that prints a fake container id.
func captureRunner(got *[]string) engine.CLIRunner {
	return engine.CLIRunner{
		Binary:   "podman",
		LookPath: func(string) (string, error) { return "/usr/bin/podman", nil },
		Command: func(ctx context.Context, name string, arg ...string) *exec.Cmd {
			*got = append([]string{name}, arg...)
			return exec.CommandContext(ctx, "echo", "cid-123")
		},
	}
}

func indexOfSeq(args, sub []string) int {
	for i := 0; i+len(sub) <= len(args); i++ {
		match := true
		for j := range sub {
			if args[i+j] != sub[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

func hasSeq(t *testing.T, args, sub []string) {
	t.Helper()
	if indexOfSeq(args, sub) < 0 {
		t.Errorf("argv %v missing subsequence %v", args, sub)
	}
}

func TestRunArgsStrictHardening(t *testing.T) {
	t.Parallel()
	var got []string
	r := captureRunner(&got)
	id, err := r.Run(context.Background(), engine.RunSpec{
		Name:           "pmmcp-app",
		Image:          "img:1",
		Command:        []string{"server", "--port", "8080"},
		Env:            map[string]string{"B": "2", "A": "1"},
		Ports:          []string{"8080:8080"},
		Labels:         map[string]string{"io.pmmcp.proc_id": "proc-1"},
		User:           "1000:1000",
		CapDrop:        []string{"ALL"},
		SecurityOpt:    []string{"no-new-privileges"},
		ReadOnlyRootfs: true,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if id != "cid-123" {
		t.Fatalf("id = %q, want cid-123", id)
	}
	if got[0] != "/usr/bin/podman" || got[1] != "run" || got[2] != "-d" {
		t.Fatalf("prefix = %v", got[:3])
	}
	hasSeq(t, got, []string{"--rm"})
	hasSeq(t, got, []string{"--name", "pmmcp-app"})
	hasSeq(t, got, []string{"--user", "1000:1000"})
	hasSeq(t, got, []string{"--read-only"})
	hasSeq(t, got, []string{"--cap-drop", "ALL"})
	hasSeq(t, got, []string{"--security-opt", "no-new-privileges"})
	hasSeq(t, got, []string{"--label", "io.pmmcp.proc_id=proc-1"})
	hasSeq(t, got, []string{"-p", "127.0.0.1:8080:8080"})
	// Env keys emitted in sorted order.
	if a, b := indexOfSeq(got, []string{"-e", "A=1"}), indexOfSeq(got, []string{"-e", "B=2"}); a < 0 || b < 0 || a > b {
		t.Errorf("env not sorted: A@%d B@%d in %v", a, b, got)
	}
	// Never privileged unless requested.
	if indexOfSeq(got, []string{"--privileged"}) >= 0 {
		t.Errorf("unexpected --privileged in %v", got)
	}
	// Image precedes the command argv.
	img := indexOfSeq(got, []string{"img:1"})
	cmd := indexOfSeq(got, []string{"server", "--port", "8080"})
	if img < 0 || cmd < 0 || img > cmd {
		t.Errorf("image@%d must precede command@%d in %v", img, cmd, got)
	}
}

func TestRunArgsPublishAllInterfaces(t *testing.T) {
	t.Parallel()
	var got []string
	r := captureRunner(&got)
	if _, err := r.Run(context.Background(), engine.RunSpec{
		Image:                "img:1",
		Ports:                []string{"8080:8080"},
		PublishAllInterfaces: true,
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	hasSeq(t, got, []string{"-p", "8080:8080"})
	if indexOfSeq(got, []string{"-p", "127.0.0.1:8080:8080"}) >= 0 {
		t.Errorf("loopback prefix applied despite PublishAllInterfaces: %v", got)
	}
}

func TestRunArgsPortForms(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"8080:8080":           "127.0.0.1:8080:8080",
		"8080":                "127.0.0.1::8080",
		"127.0.0.1:5432:5432": "127.0.0.1:5432:5432", // host IP already present
		"0.0.0.0:9000:9000":   "0.0.0.0:9000:9000",
		"8080:8080/udp":       "127.0.0.1:8080:8080/udp",
		"[::1]:8080:8080":     "[::1]:8080:8080",
	}
	for in, want := range cases {
		var got []string
		r := captureRunner(&got)
		if _, err := r.Run(context.Background(), engine.RunSpec{Image: "img:1", Ports: []string{in}}); err != nil {
			t.Fatalf("Run(%q): %v", in, err)
		}
		if indexOfSeq(got, []string{"-p", want}) < 0 {
			t.Errorf("port %q -> want -p %q in %v", in, want, got)
		}
	}
}

func TestRunArgsNoRemove(t *testing.T) {
	t.Parallel()
	var got []string
	r := captureRunner(&got)
	if _, err := r.Run(context.Background(), engine.RunSpec{Image: "img:1", NoRemove: true}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if indexOfSeq(got, []string{"--rm"}) >= 0 {
		t.Errorf("--rm present despite NoRemove: %v", got)
	}
}

func TestStopArgsSubSecondImmediate(t *testing.T) {
	t.Parallel()
	var got []string
	r := captureRunner(&got)
	if err := r.Stop(context.Background(), "cid-123", 100*time.Millisecond); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	hasSeq(t, got, []string{"-t", "0"})
}

func TestAvailableDefaultLookPath(t *testing.T) {
	t.Parallel()
	// No LookPath override: exercises the real exec.LookPath fallback branch.
	r := engine.CLIRunner{Binary: "sh"}
	if !r.Available(context.Background()) {
		t.Fatal("Available = false, want true (sh should be on PATH)")
	}
}

func TestRunCtxCanceled(t *testing.T) {
	t.Parallel()
	r := engine.CLIRunner{Binary: "podman"}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := r.Run(ctx, engine.RunSpec{Image: "img:1"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("Run err = %v, want context.Canceled", err)
	}
}

func TestRunEmptyContainerIDFromCLI(t *testing.T) {
	t.Parallel()
	r := engine.CLIRunner{
		Binary:   "podman",
		LookPath: func(string) (string, error) { return "/usr/bin/podman", nil },
		Command: func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
			return exec.CommandContext(ctx, "true")
		},
	}
	_, err := r.Run(context.Background(), engine.RunSpec{Image: "img:1"})
	if err == nil {
		t.Fatal("Run: want error for empty container id")
	}
	if got := err.Error(); !strings.Contains(got, "empty container id") {
		t.Fatalf("err = %q, want empty container id message", got)
	}
}

func TestStopCLIErrorWrapped(t *testing.T) {
	t.Parallel()
	r := engine.CLIRunner{
		Binary:   "podman",
		LookPath: func(string) (string, error) { return "/usr/bin/podman", nil },
		Command: func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
			return exec.CommandContext(ctx, "false")
		},
	}
	err := r.Stop(context.Background(), "cid-123", time.Second)
	if err == nil {
		t.Fatal("Stop: want error")
	}
	if got := err.Error(); !strings.Contains(got, "engine/podman: stop:") {
		t.Fatalf("err = %q, want wrapped engine/podman: stop: prefix", got)
	}
}

func TestRunArgsPrivileged(t *testing.T) {
	t.Parallel()
	var got []string
	r := captureRunner(&got)
	if _, err := r.Run(context.Background(), engine.RunSpec{Image: "img:1", Privileged: true}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	hasSeq(t, got, []string{"--privileged"})
}

func TestRunDefaultCommand(t *testing.T) {
	t.Parallel()
	// No Command override: exercises the real exec.CommandContext fallback
	// branch in run(). "true" is on PATH on every Linux host.
	r := engine.CLIRunner{
		Binary:   "true",
		LookPath: func(string) (string, error) { return "/usr/bin/true", nil },
	}
	if _, err := r.Run(context.Background(), engine.RunSpec{Image: "img:1"}); err == nil {
		t.Fatal("Run: want empty-container-id error, since `true` prints nothing")
	}
}

func TestAvailableCtxCanceled(t *testing.T) {
	t.Parallel()
	r := engine.CLIRunner{
		Binary: "podman",
		LookPath: func(string) (string, error) {
			t.Fatal("LookPath should not be called when ctx is already canceled")
			return "", nil
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if r.Available(ctx) {
		t.Fatal("Available = true, want false for canceled ctx")
	}
}

func TestAvailableSuccess(t *testing.T) {
	t.Parallel()
	r := engine.CLIRunner{
		Binary:   "podman",
		LookPath: func(string) (string, error) { return "/usr/bin/podman", nil },
	}
	if !r.Available(context.Background()) {
		t.Fatal("Available = false, want true")
	}
}

func TestAvailableLookPathFailure(t *testing.T) {
	t.Parallel()
	r := engine.CLIRunner{
		Binary:   "podman",
		LookPath: func(string) (string, error) { return "", exec.ErrNotFound },
	}
	if r.Available(context.Background()) {
		t.Fatal("Available = true, want false")
	}
}

func TestStopCtxCanceled(t *testing.T) {
	t.Parallel()
	r := engine.CLIRunner{Binary: "podman"}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := r.Stop(ctx, "cid-123", time.Second); !errors.Is(err, context.Canceled) {
		t.Fatalf("Stop err = %v, want context.Canceled", err)
	}
}

func TestStopEmptyContainerID(t *testing.T) {
	t.Parallel()
	r := engine.CLIRunner{Binary: "podman"}
	if err := r.Stop(context.Background(), "", time.Second); !errors.Is(err, engine.ErrNotFound) {
		t.Fatalf("Stop err = %v, want ErrNotFound", err)
	}
}

func TestStopMultiSecondTimeoutSuccess(t *testing.T) {
	t.Parallel()
	var got []string
	r := captureRunner(&got)
	if err := r.Stop(context.Background(), "cid-123", 5*time.Second); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	hasSeq(t, got, []string{"-t", "5"})
	hasSeq(t, got, []string{"stop"})
}

func TestLogsCtxCanceled(t *testing.T) {
	t.Parallel()
	r := engine.CLIRunner{Binary: "podman"}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := r.Logs(ctx, "cid-123", 10); !errors.Is(err, context.Canceled) {
		t.Fatalf("Logs err = %v, want context.Canceled", err)
	}
}

func TestLogsEmptyContainerID(t *testing.T) {
	t.Parallel()
	r := engine.CLIRunner{Binary: "podman"}
	if _, err := r.Logs(context.Background(), "", 10); !errors.Is(err, engine.ErrNotFound) {
		t.Fatalf("Logs err = %v, want ErrNotFound", err)
	}
}

func TestLogsTailBranches(t *testing.T) {
	t.Parallel()
	t.Run("with tail", func(t *testing.T) {
		t.Parallel()
		var got []string
		r := captureRunner(&got)
		if _, err := r.Logs(context.Background(), "cid-123", 50); err != nil {
			t.Fatalf("Logs: %v", err)
		}
		hasSeq(t, got, []string{"--tail", "50"})
	})
	t.Run("without tail", func(t *testing.T) {
		t.Parallel()
		var got []string
		r := captureRunner(&got)
		if _, err := r.Logs(context.Background(), "cid-123", 0); err != nil {
			t.Fatalf("Logs: %v", err)
		}
		if indexOfSeq(got, []string{"--tail"}) >= 0 {
			t.Errorf("unexpected --tail in %v", got)
		}
	})
}

func TestLogsSuccess(t *testing.T) {
	t.Parallel()
	r := engine.CLIRunner{
		Binary:   "podman",
		LookPath: func(string) (string, error) { return "/usr/bin/podman", nil },
		Command: func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
			return exec.CommandContext(ctx, "echo", "log-line")
		},
	}
	out, err := r.Logs(context.Background(), "cid-123", 10)
	if err != nil {
		t.Fatalf("Logs: %v", err)
	}
	if out != "log-line\n" {
		t.Fatalf("out = %q, want %q", out, "log-line\n")
	}
}

func TestLogsCLIErrorWrapped(t *testing.T) {
	t.Parallel()
	r := engine.CLIRunner{
		Binary:   "podman",
		LookPath: func(string) (string, error) { return "/usr/bin/podman", nil },
		Command: func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
			return exec.CommandContext(ctx, "sh", "-c", "echo boom >&2; exit 1")
		},
	}
	_, err := r.Logs(context.Background(), "cid-123", 10)
	if err == nil {
		t.Fatal("Logs: want error")
	}
	if got := err.Error(); !strings.Contains(got, "engine/podman: logs:") {
		t.Fatalf("err = %q, want wrapped engine/podman: logs: prefix", got)
	}
}

func TestRunArgsRemainingBranches(t *testing.T) {
	t.Parallel()
	var got []string
	r := captureRunner(&got)
	if _, err := r.Run(context.Background(), engine.RunSpec{
		Image:   "img:1",
		Network: "bridge",
		CapAdd:  []string{"NET_ADMIN"},
		Volumes: []engine.VolumeMount{
			{Source: "/host", Target: "/container", ReadOnly: true},
			{Source: "/onlysource"},
			{Target: "/onlytarget"},
		},
		Ports: []string{""},
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	hasSeq(t, got, []string{"--network", "bridge"})
	hasSeq(t, got, []string{"--cap-add", "NET_ADMIN"})
	hasSeq(t, got, []string{"-v", "/host:/container:ro"})
	if indexOfSeq(got, []string{"-v", "/onlysource"}) >= 0 {
		t.Errorf("volume with empty target should be skipped: %v", got)
	}
	if indexOfSeq(got, []string{"-v", "/onlytarget"}) >= 0 {
		t.Errorf("volume with empty source should be skipped: %v", got)
	}
	if indexOfSeq(got, []string{"-p", ""}) >= 0 {
		t.Errorf("empty port should be skipped: %v", got)
	}
	// No spec.Command: the argv should end at the image with no trailing args.
	if got[len(got)-1] != "img:1" {
		t.Fatalf("last arg = %q, want img:1 (empty Command)", got[len(got)-1])
	}
}

func TestRunCLIErrorNonEmptyStderr(t *testing.T) {
	t.Parallel()
	r := engine.CLIRunner{
		Binary:   "podman",
		LookPath: func(string) (string, error) { return "/usr/bin/podman", nil },
		Command: func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
			return exec.CommandContext(ctx, "sh", "-c", "echo something-broke >&2; exit 1")
		},
	}
	_, err := r.Run(context.Background(), engine.RunSpec{Image: "img:1"})
	if err == nil {
		t.Fatal("Run: want error")
	}
	if got := err.Error(); !strings.Contains(got, "something-broke") {
		t.Fatalf("err = %q, want it to contain stderr message", got)
	}
}

func TestRunCLIErrorEmptyStderrFallsBackToErr(t *testing.T) {
	t.Parallel()
	r := engine.CLIRunner{
		Binary:   "podman",
		LookPath: func(string) (string, error) { return "/usr/bin/podman", nil },
		Command: func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
			return exec.CommandContext(ctx, "false")
		},
	}
	_, err := r.Run(context.Background(), engine.RunSpec{Image: "img:1"})
	if err == nil {
		t.Fatal("Run: want error")
	}
	// exec.ExitError.Error() is something like "exit status 1"; ensure it made
	// it into the wrapped message as the stderr fallback.
	if got := err.Error(); !strings.Contains(got, "exit status 1") {
		t.Fatalf("err = %q, want it to contain fallback exit status message", got)
	}
}
