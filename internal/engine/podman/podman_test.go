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

package podman_test

import (
	"context"
	"errors"
	"os/exec"
	"testing"
	"time"

	"github.com/scrothers/pmmcp/internal/engine"
	"github.com/scrothers/pmmcp/internal/engine/podman"
)

// Unit tests are hermetic: the CLI runner is injected via NewWithCLI so
// nothing here depends on a real podman binary or daemon (podman may or may
// not be installed on the host running these tests).

func TestName(t *testing.T) {
	t.Parallel()
	e := podman.New()
	if e.Name() != "podman" {
		t.Fatalf("Name = %q, want podman", e.Name())
	}
}

func TestEmptyImage(t *testing.T) {
	t.Parallel()
	e := podman.New()
	_, err := e.Run(context.Background(), engine.RunSpec{})
	if !errors.Is(err, engine.ErrInvalidSpec) {
		t.Fatalf("err = %v, want ErrInvalidSpec", err)
	}
}

func TestAvailable(t *testing.T) {
	t.Parallel()
	t.Run("true", func(t *testing.T) {
		t.Parallel()
		e := podman.NewWithCLI(engine.CLIRunner{
			Binary:   "podman",
			LookPath: func(string) (string, error) { return "/usr/bin/podman", nil },
		})
		if !e.Available(context.Background()) {
			t.Fatal("Available = false, want true")
		}
	})
	t.Run("false", func(t *testing.T) {
		t.Parallel()
		e := podman.NewWithCLI(engine.CLIRunner{
			Binary:   "podman",
			LookPath: func(string) (string, error) { return "", exec.ErrNotFound },
		})
		if e.Available(context.Background()) {
			t.Fatal("Available = true, want false")
		}
	})
}

func TestStop(t *testing.T) {
	t.Parallel()
	t.Run("success", func(t *testing.T) {
		t.Parallel()
		e := podman.NewWithCLI(engine.CLIRunner{
			Binary:   "podman",
			LookPath: func(string) (string, error) { return "/usr/bin/podman", nil },
			Command: func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
				return exec.CommandContext(ctx, "true")
			},
		})
		if err := e.Stop(context.Background(), "cid-123", time.Second); err != nil {
			t.Fatalf("Stop: %v", err)
		}
	})
	t.Run("cli error", func(t *testing.T) {
		t.Parallel()
		e := podman.NewWithCLI(engine.CLIRunner{
			Binary:   "podman",
			LookPath: func(string) (string, error) { return "/usr/bin/podman", nil },
			Command: func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
				return exec.CommandContext(ctx, "false")
			},
		})
		if err := e.Stop(context.Background(), "cid-123", time.Second); err == nil {
			t.Fatal("Stop: want error")
		}
	})
	t.Run("unavailable", func(t *testing.T) {
		t.Parallel()
		e := podman.NewWithCLI(engine.CLIRunner{
			Binary:   "podman",
			LookPath: func(string) (string, error) { return "", exec.ErrNotFound },
		})
		if err := e.Stop(context.Background(), "cid-123", time.Second); !errors.Is(err, engine.ErrUnavailable) {
			t.Fatalf("Stop err = %v, want ErrUnavailable", err)
		}
	})
}

func TestLogs(t *testing.T) {
	t.Parallel()
	t.Run("success", func(t *testing.T) {
		t.Parallel()
		e := podman.NewWithCLI(engine.CLIRunner{
			Binary:   "podman",
			LookPath: func(string) (string, error) { return "/usr/bin/podman", nil },
			Command: func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
				return exec.CommandContext(ctx, "echo", "log-line")
			},
		})
		out, err := e.Logs(context.Background(), "cid-123", 10)
		if err != nil {
			t.Fatalf("Logs: %v", err)
		}
		if out != "log-line\n" {
			t.Fatalf("out = %q, want %q", out, "log-line\n")
		}
	})
	t.Run("cli error", func(t *testing.T) {
		t.Parallel()
		e := podman.NewWithCLI(engine.CLIRunner{
			Binary:   "podman",
			LookPath: func(string) (string, error) { return "/usr/bin/podman", nil },
			Command: func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
				return exec.CommandContext(ctx, "false")
			},
		})
		if _, err := e.Logs(context.Background(), "cid-123", 10); err == nil {
			t.Fatal("Logs: want error")
		}
	})
	t.Run("unavailable", func(t *testing.T) {
		t.Parallel()
		e := podman.NewWithCLI(engine.CLIRunner{
			Binary:   "podman",
			LookPath: func(string) (string, error) { return "", exec.ErrNotFound },
		})
		if _, err := e.Logs(context.Background(), "cid-123", 10); !errors.Is(err, engine.ErrUnavailable) {
			t.Fatalf("Logs err = %v, want ErrUnavailable", err)
		}
	})
}
