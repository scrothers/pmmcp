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

package local_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/scrothers/pmmcp/internal/domain"
	"github.com/scrothers/pmmcp/internal/process"
	"github.com/scrothers/pmmcp/internal/process/local"
)

func TestStartEchoLogDir(t *testing.T) {
	t.Parallel()
	m := local.New()
	ctx := context.Background()
	logDir := t.TempDir()

	h, err := m.Start(ctx, process.StartSpec{
		ID:      "proc-01ARZ3NDEKTSV4RRFFQ69G5FAV",
		Name:    "echo",
		Command: []string{"echo", "hi"},
		LogDir:  logDir,
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if h.PID <= 0 {
		t.Fatalf("PID = %d, want > 0", h.PID)
	}
	if h.Status != domain.StatusRunning && h.Status != domain.StatusExited {
		// May already have exited on a fast machine before we inspect.
		t.Fatalf("Status = %q, want running or exited", h.Status)
	}

	waited, err := m.Wait(ctx, h.ID)
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if waited.Status != domain.StatusExited {
		t.Fatalf("Status = %q, want %q", waited.Status, domain.StatusExited)
	}
	if waited.ExitCode == nil || *waited.ExitCode != 0 {
		t.Fatalf("ExitCode = %v, want 0", waited.ExitCode)
	}

	// Ensure log files flushed.
	time.Sleep(20 * time.Millisecond)
	out, err := os.ReadFile(filepath.Join(logDir, "stdout.log"))
	if err != nil {
		t.Fatalf("read stdout.log: %v", err)
	}
	if string(out) != "hi\n" && string(out) != "hi\r\n" {
		t.Fatalf("stdout = %q, want %q", out, "hi\\n")
	}
}

func TestStopSleep(t *testing.T) {
	t.Parallel()
	m := local.New()
	ctx := context.Background()

	h, err := m.Start(ctx, process.StartSpec{
		ID:      "proc-01BX5ZZKBKACTAV9WEVGEMMVRZ",
		Command: []string{"sleep", "30"},
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Confirm still running.
	insp, err := m.Inspect(ctx, h.ID)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if insp.Status != domain.StatusRunning {
		t.Fatalf("Status = %q, want running", insp.Status)
	}

	start := time.Now()
	if err := m.Stop(ctx, h.ID, 2*time.Second); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("Stop took %v, want well under sleep duration", elapsed)
	}

	waited, err := m.Wait(ctx, h.ID)
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if waited.Status != domain.StatusExited {
		t.Fatalf("Status = %q, want exited", waited.Status)
	}
	if waited.ExitCode == nil {
		t.Fatal("ExitCode is nil")
	}
}

func TestWaitNaturalExit(t *testing.T) {
	t.Parallel()
	m := local.New()
	ctx := context.Background()

	h, err := m.Start(ctx, process.StartSpec{
		ID:      "proc-01HABCDEFGHJKMNPQRSTVWXYZ0",
		Command: []string{"true"},
	})
	if err != nil {
		// true may not exist on Windows; use a portable alternative.
		if runtime.GOOS == "windows" {
			h, err = m.Start(ctx, process.StartSpec{
				ID:      "proc-01HABCDEFGHJKMNPQRSTVWXYZ0",
				Command: []string{"cmd", "/c", "exit", "0"},
			})
		}
		if err != nil {
			t.Fatalf("Start: %v", err)
		}
	}

	waited, err := m.Wait(ctx, h.ID)
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if waited.ExitCode == nil || *waited.ExitCode != 0 {
		t.Fatalf("ExitCode = %v, want 0", waited.ExitCode)
	}
}

func TestStartInvalidAndDuplicate(t *testing.T) {
	t.Parallel()
	m := local.New()
	ctx := context.Background()

	if _, err := m.Start(ctx, process.StartSpec{ID: "x", Command: nil}); !errors.Is(err, process.ErrInvalidSpec) {
		t.Fatalf("empty command: %v", err)
	}

	spec := process.StartSpec{
		ID:      "proc-01ARZ3NDEKTSV4RRFFQ69G5FAV",
		Command: []string{"sleep", "5"},
	}
	if _, err := m.Start(ctx, spec); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = m.Stop(ctx, spec.ID, time.Second) })

	if _, err := m.Start(ctx, spec); !errors.Is(err, process.ErrAlreadyExists) {
		t.Fatalf("duplicate: %v", err)
	}
}

func TestInspectNotFound(t *testing.T) {
	t.Parallel()
	m := local.New()
	_, err := m.Inspect(context.Background(), "nope")
	if !errors.Is(err, process.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestNoShellWrapping(t *testing.T) {
	t.Parallel()
	// A single-token shell expression must not be interpreted as a shell command.
	m := local.New()
	_, err := m.Start(context.Background(), process.StartSpec{
		ID:      "proc-01ARZ3NDEKTSV4RRFFQ69G5FAV",
		Command: []string{"echo hi"},
	})
	if err == nil {
		t.Fatal("expected start error for non-existent argv0 'echo hi'")
	}
}

func TestCwdAndEnv(t *testing.T) {
	t.Parallel()
	m := local.New()
	ctx := context.Background()
	dir := t.TempDir()
	logDir := t.TempDir()

	// Print PWD (or cd) via a small argv that writes env.
	// Use /bin/sh only as explicit argv0 (user-requested interpreter), not shell wrapping of a string command.
	h, err := m.Start(ctx, process.StartSpec{
		ID:      "proc-01BX5ZZKBKACTAV9WEVGEMMVRZ",
		Command: []string{"/bin/sh", "-c", "printf '%s\\n' \"$PMMCP_TEST\"; pwd"},
		Cwd:     dir,
		Env:     []string{"PMMCP_TEST=from-overlay"},
		LogDir:  logDir,
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if _, err := m.Wait(ctx, h.ID); err != nil {
		t.Fatalf("Wait: %v", err)
	}
	out, err := os.ReadFile(filepath.Join(logDir, "stdout.log"))
	if err != nil {
		t.Fatal(err)
	}
	wantPrefix := "from-overlay\n"
	if runtime.GOOS == "windows" {
		t.Skip("cwd/env shell helper is unix-oriented")
	}
	if len(out) < len(wantPrefix) || string(out[:len(wantPrefix)]) != wantPrefix {
		t.Fatalf("stdout = %q, want prefix %q", out, wantPrefix)
	}
	// Second line should be the temp dir (may be symlink-resolved).
	rest := string(out[len(wantPrefix):])
	if rest != dir+"\n" {
		// Accept resolved path if different from dir.
		resolved, _ := filepath.EvalSymlinks(dir)
		if rest != resolved+"\n" {
			t.Fatalf("pwd line = %q, want %q or %q", rest, dir, resolved)
		}
	}
}

// TestImmediateExitThenRestartRace exercises the auto-restart crash-loop path:
// a child that exits immediately, restarted repeatedly under the same ID, with
// concurrent Inspect calls. Under -race this guards the fixed data race where
// Start read the handle without the lock while the reaper wrote status/exitCode.
func TestImmediateExitThenRestartRace(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses unix 'true'")
	}
	m := local.New()
	ctx := context.Background()
	const id = "proc-01RESTARTRACE000000000001"

	for i := range 40 {
		h, err := m.Start(ctx, process.StartSpec{ID: id, Command: []string{"true"}})
		if err != nil {
			t.Fatalf("Start %d: %v", i, err)
		}
		// Read fields of the returned handle while the reaper runs concurrently.
		_ = h.PID
		_ = h.Status

		done := make(chan struct{})
		go func() {
			defer close(done)
			for range 20 {
				_, _ = m.Inspect(ctx, id)
			}
		}()

		waited, err := m.Wait(ctx, id)
		if err != nil {
			t.Fatalf("Wait %d: %v", i, err)
		}
		if waited.Status != domain.StatusExited {
			t.Fatalf("iter %d status = %q, want exited", i, waited.Status)
		}
		<-done
	}
}

func TestMinimalEnvDoesNotLeakDaemonSecret(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix env helper")
	}
	// A secret in the daemon environment must not reach a default (minimal) child.
	t.Setenv("PMMCP_FAKE_SECRET", "leaked-token-value")
	m := local.New()
	ctx := context.Background()
	logDir := t.TempDir()
	h, err := m.Start(ctx, process.StartSpec{
		ID:      "proc-01ENVLEAK0000000000000001",
		Command: []string{"/bin/sh", "-c", "printf '%s\\n' \"${PMMCP_FAKE_SECRET:-absent}\""},
		LogDir:  logDir,
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if _, err := m.Wait(ctx, h.ID); err != nil {
		t.Fatalf("Wait: %v", err)
	}
	out, err := os.ReadFile(filepath.Join(logDir, "stdout.log"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(out)) != "absent" {
		t.Fatalf("child saw daemon secret: stdout = %q", out)
	}
}

func TestSignalMissing(t *testing.T) {
	t.Parallel()
	m := local.New()
	err := m.Signal(context.Background(), "missing", os.Interrupt)
	if !errors.Is(err, process.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}
