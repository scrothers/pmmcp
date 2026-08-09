// Copyright 2026 Steven Crothers
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build unix

package local_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/scrothers/pmmcp/internal/domain"
	"github.com/scrothers/pmmcp/internal/process"
	"github.com/scrothers/pmmcp/internal/process/local"
)

// canceledContext returns a context that is already canceled.
func canceledContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

// startSleeper starts a long-lived child and guarantees teardown.
func startSleeper(t *testing.T, m *local.Manager, id string) *process.Handle {
	t.Helper()
	h, err := m.Start(context.Background(), process.StartSpec{
		ID:      id,
		Command: []string{"/bin/sleep", "30"},
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = m.Stop(context.Background(), id, time.Millisecond) })
	return h
}

func TestStartRejectsCanceledContext(t *testing.T) {
	t.Parallel()
	m := local.New()
	_, err := m.Start(canceledContext(t), process.StartSpec{
		ID:      "proc-01CTXCANCELSTART000001",
		Command: []string{"true"},
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Start = %v, want context.Canceled", err)
	}
}

func TestStartRejectsEmptyID(t *testing.T) {
	t.Parallel()
	m := local.New()
	_, err := m.Start(context.Background(), process.StartSpec{Command: []string{"true"}})
	if !errors.Is(err, process.ErrInvalidSpec) {
		t.Fatalf("Start = %v, want ErrInvalidSpec", err)
	}
}

// TestStartLogDirErrors covers each of the three capture-setup failures. The
// triggers are permission-independent (they rely on file-type conflicts), so
// they behave identically for an unprivileged daemon and for root.
func TestStartLogDirErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		logDir  func(t *testing.T) string
		wantSub string
	}{
		{
			name: "logcap new fails when a path component is a file",
			logDir: func(t *testing.T) string {
				t.Helper()
				base := t.TempDir()
				file := filepath.Join(base, "notadir")
				if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
					t.Fatal(err)
				}
				return filepath.Join(file, "logs")
			},
			wantSub: "local: logcap:",
		},
		{
			name: "stdout open fails when stdout.log is a directory",
			logDir: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				if err := os.Mkdir(filepath.Join(dir, "stdout.log"), 0o700); err != nil {
					t.Fatal(err)
				}
				return dir
			},
			wantSub: "local: open stdout.log:",
		},
		{
			name: "stderr open fails when stderr.log is a directory",
			logDir: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				if err := os.Mkdir(filepath.Join(dir, "stderr.log"), 0o700); err != nil {
					t.Fatal(err)
				}
				return dir
			},
			wantSub: "local: open stderr.log:",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m := local.New()
			_, err := m.Start(context.Background(), process.StartSpec{
				ID:      "proc-01LOGDIRFAIL0000000001",
				Command: []string{"true"},
				LogDir:  tt.logDir(t),
			})
			if err == nil || !strings.Contains(err.Error(), tt.wantSub) {
				t.Fatalf("Start = %v, want an error containing %q", err, tt.wantSub)
			}
		})
	}
}

// TestStartExecFailureClosesCaptureFiles exercises the cmd.Start failure arm
// with capture writers already open, so both are closed before returning.
func TestStartExecFailureClosesCaptureFiles(t *testing.T) {
	t.Parallel()
	m := local.New()
	logDir := t.TempDir()
	_, err := m.Start(context.Background(), process.StartSpec{
		ID:      "proc-01EXECFAILCLOSE0000001",
		Command: []string{filepath.Join(t.TempDir(), "definitely-not-a-binary")},
		LogDir:  logDir,
	})
	if err == nil || !strings.Contains(err.Error(), "local: start:") {
		t.Fatalf("Start = %v, want a local: start: error", err)
	}
	// Both capture files must exist and be closed (re-openable and writable).
	for _, name := range []string{"stdout.log", "stderr.log"} {
		f, err := os.OpenFile(filepath.Join(logDir, name), os.O_WRONLY|os.O_APPEND, 0o600)
		if err != nil {
			t.Fatalf("reopen %s: %v", name, err)
		}
		_ = f.Close()
	}
}

// TestStartMemoryLimit exercises the rlimit path end to end: pre-start
// SysProcAttr shaping plus the post-start prlimit against a live child.
func TestStartMemoryLimit(t *testing.T) {
	t.Parallel()
	m := local.New()
	logDir := t.TempDir()
	h, err := m.Start(context.Background(), process.StartSpec{
		ID:          "proc-01MEMLIMIT00000000001",
		Command:     []string{"/bin/sh", "-c", "ulimit -v; sleep 0.2"},
		LogDir:      logDir,
		MemoryBytes: 256 << 20,
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	waited, err := m.Wait(ctx, h.ID)
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if waited.ExitCode == nil || *waited.ExitCode != 0 {
		t.Fatalf("ExitCode = %v, want 0", waited.ExitCode)
	}
	// ulimit -v reports RLIMIT_AS in KiB; the limit is applied post-start, so the
	// child may or may not observe it. Only assert it is not "unlimited" when a
	// number was reported.
	out, err := os.ReadFile(filepath.Join(logDir, "stdout.log"))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(out)); got != "" && got != "unlimited" {
		if kb, convErr := strconv.Atoi(got); convErr == nil && kb > 256*1024 {
			t.Fatalf("ulimit -v = %d KiB, want <= %d", kb, 256*1024)
		}
	}
}

// TestStartSandboxWithoutCwdUsesWorkingDirectory covers the project-root
// fallback: an empty Cwd resolves to the daemon's working directory.
func TestStartSandboxWithoutCwdUsesWorkingDirectory(t *testing.T) {
	t.Parallel()
	m := local.New()
	h, err := m.Start(context.Background(), process.StartSpec{
		ID:      "proc-01SANDBOXNOCWD00000001",
		Command: []string{"true"},
		Sandbox: "standard",
	})
	if err != nil {
		t.Fatalf("Start with empty Cwd under standard: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := m.Wait(ctx, h.ID); err != nil {
		t.Fatalf("Wait: %v", err)
	}
}

func TestReapRecordsNonZeroExitCode(t *testing.T) {
	t.Parallel()
	m := local.New()
	h, err := m.Start(context.Background(), process.StartSpec{
		ID:      "proc-01EXITCODE3000000000001",
		Command: []string{"/bin/sh", "-c", "exit 3"},
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	waited, err := m.Wait(context.Background(), h.ID)
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if waited.ExitCode == nil || *waited.ExitCode != 3 {
		t.Fatalf("ExitCode = %v, want 3", waited.ExitCode)
	}
	if waited.Status != domain.StatusExited {
		t.Fatalf("Status = %q, want exited", waited.Status)
	}
}

// TestReapRecordsCaptureFailureAsMinusOne covers the non-ExitError arm of reap.
// os/exec returns the stdout copier's error from Wait when the child itself
// exited 0, so a capture sink that fails mid-stream surfaces as exit code -1
// rather than a fabricated success.
//
// The failure is arranged without touching permissions: stdout.log is
// pre-seeded at exactly the rotation cap so the child's first line
// forces a rotation, and the oldest archive slot (stdout.log.5) is a non-empty
// directory, which rotation cannot remove.
func TestReapRecordsCaptureFailureAsMinusOne(t *testing.T) {
	t.Parallel()
	logDir := t.TempDir()
	const capBytes = 10 * 1024 * 1024
	f, err := os.OpenFile(filepath.Join(logDir, "stdout.log"), os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(capBytes); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	blocker := filepath.Join(logDir, "stdout.log.5")
	if err := os.Mkdir(blocker, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(blocker, "occupant"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	m := local.New()
	h, err := m.Start(context.Background(), process.StartSpec{
		ID:      "proc-01CAPTUREFAILURE000001",
		Command: []string{"/bin/sh", "-c", "echo x"},
		LogDir:  logDir,
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	waited, err := m.Wait(ctx, h.ID)
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if waited.ExitCode == nil || *waited.ExitCode != -1 {
		t.Fatalf("ExitCode = %v, want -1 for a non-ExitError wait failure", waited.ExitCode)
	}
}

func TestStopNotFound(t *testing.T) {
	t.Parallel()
	m := local.New()
	err := m.Stop(context.Background(), "missing", time.Second)
	if !errors.Is(err, process.ErrNotFound) {
		t.Fatalf("Stop = %v, want ErrNotFound", err)
	}
}

func TestStopRejectsCanceledContext(t *testing.T) {
	t.Parallel()
	m := local.New()
	err := m.Stop(canceledContext(t), "missing", time.Second)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Stop = %v, want context.Canceled", err)
	}
}

func TestStopAfterExitIsNoOp(t *testing.T) {
	t.Parallel()
	m := local.New()
	h, err := m.Start(context.Background(), process.StartSpec{
		ID:      "proc-01STOPAFTEREXIT0000001",
		Command: []string{"true"},
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if _, err := m.Wait(context.Background(), h.ID); err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if err := m.Stop(context.Background(), h.ID, time.Second); err != nil {
		t.Fatalf("Stop after exit = %v, want nil", err)
	}
}

// TestStopNonPositiveTimeoutUsesDefault covers the timeout normalization arm.
func TestStopNonPositiveTimeoutUsesDefault(t *testing.T) {
	t.Parallel()
	m := local.New()
	h := startSleeper(t, m, "proc-01STOPDEFAULTTIMEOUT01")
	start := time.Now()
	if err := m.Stop(context.Background(), h.ID, 0); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if elapsed := time.Since(start); elapsed >= local.DefaultStopTimeout {
		t.Fatalf("Stop took %v; SIGTERM should have landed well inside the default timeout", elapsed)
	}
}

// TestStopForcePathSkipsGraceful covers the immediate-kill arm (timeout <= 5ms).
func TestStopForcePathSkipsGraceful(t *testing.T) {
	t.Parallel()
	m := local.New()
	h := startSleeper(t, m, "proc-01STOPFORCEPATH0000001")
	if err := m.Stop(context.Background(), h.ID, time.Millisecond); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	waited, err := m.Wait(context.Background(), h.ID)
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if waited.Status != domain.StatusExited {
		t.Fatalf("Status = %q, want exited", waited.Status)
	}
}

// TestStopEscalatesToSIGKILL uses a child that ignores SIGTERM, so Stop must let
// the grace timer expire and then force-kill the group.
func TestStopEscalatesToSIGKILL(t *testing.T) {
	t.Parallel()
	m := local.New()
	h, err := m.Start(context.Background(), process.StartSpec{
		ID: "proc-01STOPESCALATE00000001",
		// The loop keeps the shell itself resident (a trailing `sleep 5` would be
		// exec-optimized away, and the replacement would not inherit the trap).
		// The child sleep is long-lived so waitForChildren reliably observes it
		// even under heavy parallel `-race` scheduling (a sub-100ms child was
		// racy to catch); tree-kill on Stop reaps it regardless.
		Command: []string{"/bin/sh", "-c", `trap "" TERM; while :; do sleep 5; done`},
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	// Wait until the shell has spawned its first `sleep`, which proves the trap
	// is installed; a SIGTERM landing before that would kill the shell outright.
	waitForChildren(t, h.PID, 1)

	const grace = 150 * time.Millisecond
	start := time.Now()
	if err := m.Stop(context.Background(), h.ID, grace); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if elapsed := time.Since(start); elapsed < grace {
		t.Fatalf("Stop returned after %v; the SIGTERM grace period was skipped", elapsed)
	}
	waited, err := m.Wait(context.Background(), h.ID)
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if waited.Status != domain.StatusExited {
		t.Fatalf("Status = %q, want exited", waited.Status)
	}
}

// TestStopKillsGrandchildren is the tree-kill guarantee: Stop signals the whole
// process group, so descendants the daemon never saw die with the direct child.
func TestStopKillsGrandchildren(t *testing.T) {
	t.Parallel()
	m := local.New()
	h, err := m.Start(context.Background(), process.StartSpec{
		ID:      "proc-01TREEKILL000000000001",
		Command: []string{"/bin/sh", "-c", "sleep 100 & sleep 100 & wait"},
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = m.Stop(context.Background(), h.ID, time.Millisecond) })

	grandchildren := waitForChildren(t, h.PID, 2)
	for _, pid := range grandchildren {
		if !pidAlive(pid) {
			t.Fatalf("grandchild %d not alive before Stop", pid)
		}
	}

	if err := m.Stop(context.Background(), h.ID, 2*time.Second); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if _, err := m.Wait(context.Background(), h.ID); err != nil {
		t.Fatalf("Wait: %v", err)
	}

	// The grandchildren are re-parented to init on the shell's death, so poll
	// until the kernel has torn them down.
	deadline := time.Now().Add(5 * time.Second)
	for _, pid := range grandchildren {
		for pidAlive(pid) {
			if time.Now().After(deadline) {
				t.Fatalf("grandchild %d survived Stop of its process group", pid)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
}

// waitForChildren polls /proc until ppid has at least n children, returning
// them. /proc enumeration is Linux-only, so callers are skipped elsewhere.
func waitForChildren(t *testing.T, ppid, n int) []int {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("process-tree enumeration via /proc is Linux-only")
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		kids := childrenOf(t, ppid)
		if len(kids) >= n {
			return kids
		}
		if time.Now().After(deadline) {
			t.Fatalf("process %d never had %d children (saw %v)", ppid, n, kids)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// childrenOf scans /proc for processes whose PPid is ppid.
func childrenOf(t *testing.T, ppid int) []int {
	t.Helper()
	entries, err := os.ReadDir("/proc")
	if err != nil {
		t.Fatalf("read /proc: %v", err)
	}
	want := "PPid:\t" + strconv.Itoa(ppid)
	var kids []int
	for _, e := range entries {
		pid, convErr := strconv.Atoi(e.Name())
		if convErr != nil {
			continue
		}
		status, readErr := os.ReadFile(filepath.Join("/proc", e.Name(), "status"))
		if readErr != nil {
			continue // exited between ReadDir and ReadFile
		}
		for line := range strings.SplitSeq(string(status), "\n") {
			if line == want {
				kids = append(kids, pid)
				break
			}
		}
	}
	return kids
}

// pidAlive reports whether pid still exists (zombies included; the reaper clears
// the direct child, and re-parented grandchildren are reaped by init).
func pidAlive(pid int) bool {
	return syscall.Kill(pid, 0) == nil
}

func TestWaitNotFound(t *testing.T) {
	t.Parallel()
	m := local.New()
	_, err := m.Wait(context.Background(), "missing")
	if !errors.Is(err, process.ErrNotFound) {
		t.Fatalf("Wait = %v, want ErrNotFound", err)
	}
}

func TestWaitRespectsCanceledContext(t *testing.T) {
	t.Parallel()
	m := local.New()
	h := startSleeper(t, m, "proc-01WAITCTXCANCEL0000001")
	_, err := m.Wait(canceledContext(t), h.ID)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Wait = %v, want context.Canceled", err)
	}
}

func TestInspectRejectsCanceledContext(t *testing.T) {
	t.Parallel()
	m := local.New()
	_, err := m.Inspect(canceledContext(t), "anything")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Inspect = %v, want context.Canceled", err)
	}
}

func TestSignalRejectsCanceledContext(t *testing.T) {
	t.Parallel()
	m := local.New()
	err := m.Signal(canceledContext(t), "anything", syscall.SIGTERM)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Signal = %v, want context.Canceled", err)
	}
}

func TestSignalTerminalProcess(t *testing.T) {
	t.Parallel()
	m := local.New()
	h, err := m.Start(context.Background(), process.StartSpec{
		ID:      "proc-01SIGNALTERMINAL000001",
		Command: []string{"true"},
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if _, err := m.Wait(context.Background(), h.ID); err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if err := m.Signal(context.Background(), h.ID, syscall.SIGTERM); !errors.Is(err, process.ErrNotRunning) {
		t.Fatalf("Signal = %v, want ErrNotRunning", err)
	}
}

func TestSignalDeliversToRunningProcess(t *testing.T) {
	t.Parallel()
	m := local.New()
	h := startSleeper(t, m, "proc-01SIGNALDELIVER0000001")
	// SIGCONT is inert for an already-running child, so this asserts delivery
	// without racing the reaper.
	if err := m.Signal(context.Background(), h.ID, syscall.SIGCONT); err != nil {
		t.Fatalf("Signal = %v, want nil", err)
	}
	insp, err := m.Inspect(context.Background(), h.ID)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if insp.Status != domain.StatusRunning {
		t.Fatalf("Status = %q, want running", insp.Status)
	}
}
