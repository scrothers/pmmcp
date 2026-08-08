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

//go:build unix

package local

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/scrothers/pmmcp/internal/domain"
	"github.com/scrothers/pmmcp/internal/process"
)

// deadPID returns a PID that is guaranteed to have been reaped, so signalling
// its process group yields ESRCH. The child is its own group leader, so the
// group disappears with it.
func deadPID(t *testing.T) int {
	t.Helper()
	cmd := exec.Command("/bin/true")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start throwaway child: %v", err)
	}
	pid := cmd.Process.Pid
	_ = cmd.Wait()
	return pid
}

// injectProc registers a synthetic proc whose done channel the test owns, so
// Stop's timeout arms can be driven without an unkillable real child. pid is
// deliberately a dead or invalid PID: every signal the code under test sends
// must be a no-op for the host.
func injectProc(t *testing.T, m *Manager, id string, pid int) *proc {
	t.Helper()
	p := &proc{
		id:     id,
		pid:    pid,
		status: domain.StatusRunning,
		done:   make(chan struct{}),
	}
	m.mu.Lock()
	m.procs[id] = p
	m.mu.Unlock()
	return p
}

func TestBuildChildEnv(t *testing.T) {
	t.Parallel()
	full := os.Environ()
	minimal := minimalEnv()

	tests := []struct {
		name    string
		inherit string
		env     []string
		want    []string
	}{
		{name: "full base only", inherit: "full", want: full},
		{name: "full with overlay", inherit: "FULL", env: []string{"A=1"}, want: append(slices.Clone(full), "A=1")},
		{name: "none base only", inherit: "none", want: []string{}},
		{name: "none with overlay", inherit: "  none  ", env: []string{"A=1", "B=2"}, want: []string{"A=1", "B=2"}},
		{name: "empty means minimal", inherit: "", want: minimal},
		{name: "explicit minimal", inherit: "minimal", want: minimal},
		{name: "unknown falls back to minimal", inherit: "bogus", env: []string{"A=1"}, want: append(slices.Clone(minimal), "A=1")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := buildChildEnv(process.StartSpec{InheritEnv: tt.inherit, Env: tt.env})
			if got == nil {
				t.Fatal("buildChildEnv returned nil; os/exec would inherit the daemon environment")
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("buildChildEnv() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsNoProcessError(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "process done", err: os.ErrProcessDone, want: true},
		{name: "wrapped process done", err: fmt.Errorf("kill: %w", os.ErrProcessDone), want: true},
		{name: "esrch", err: syscall.ESRCH, want: true},
		{name: "wrapped esrch", err: fmt.Errorf("kill: %w", syscall.ESRCH), want: true},
		{name: "other errno", err: syscall.EPERM, want: false},
		{name: "unrelated", err: errors.New("boom"), want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := isNoProcessError(tt.err); got != tt.want {
				t.Errorf("isNoProcessError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

// fakeSignal is an os.Signal that is not a syscall.Signal, forcing killTree onto
// its os.FindProcess fallback.
type fakeSignal struct{}

func (fakeSignal) String() string { return "fake" }
func (fakeSignal) Signal()        {}

func TestKillTree(t *testing.T) {
	t.Parallel()
	dead := deadPID(t)

	t.Run("invalid pid", func(t *testing.T) {
		t.Parallel()
		err := killTree(0, syscall.SIGTERM)
		if err == nil || !strings.Contains(err.Error(), "invalid pid") {
			t.Fatalf("killTree(0) = %v, want invalid pid error", err)
		}
	})

	t.Run("non-syscall signal falls back to single process", func(t *testing.T) {
		t.Parallel()
		// os.Process.Signal rejects a non-syscall.Signal, which is exactly the
		// fallback arm we want to observe; the value of the error is incidental.
		if err := killTree(dead, fakeSignal{}); err == nil {
			t.Fatal("killTree with a non-syscall signal returned nil, want an error")
		}
	})

	t.Run("dead process group reports esrch", func(t *testing.T) {
		t.Parallel()
		err := killTree(dead, syscall.SIGTERM)
		if err == nil {
			t.Fatalf("killTree(%d) on a reaped group returned nil, want ESRCH", dead)
		}
		if !isNoProcessError(err) {
			t.Fatalf("killTree(%d) = %v, want an ESRCH-class error", dead, err)
		}
	})

	t.Run("live process group succeeds", func(t *testing.T) {
		t.Parallel()
		cmd := exec.Command("/bin/sleep", "30")
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		if err := cmd.Start(); err != nil {
			t.Fatalf("start: %v", err)
		}
		t.Cleanup(func() { _ = cmd.Wait() })
		if err := killTree(cmd.Process.Pid, syscall.SIGKILL); err != nil {
			t.Fatalf("killTree on live group: %v", err)
		}
	})
}

func TestApplyMemoryLimit(t *testing.T) {
	t.Parallel()
	t.Run("creates SysProcAttr", func(t *testing.T) {
		t.Parallel()
		cmd := exec.Command("/bin/true")
		applyMemoryLimit(cmd, 1<<20)
		if cmd.SysProcAttr == nil || !cmd.SysProcAttr.Setpgid {
			t.Fatalf("SysProcAttr = %+v, want Setpgid true", cmd.SysProcAttr)
		}
	})
	t.Run("preserves existing SysProcAttr", func(t *testing.T) {
		t.Parallel()
		cmd := exec.Command("/bin/true")
		setSysProcAttr(cmd)
		existing := cmd.SysProcAttr
		applyMemoryLimit(cmd, 1<<20)
		if cmd.SysProcAttr != existing || !cmd.SysProcAttr.Setpgid {
			t.Fatalf("SysProcAttr replaced or Setpgid cleared: %+v", cmd.SysProcAttr)
		}
	})
}

func TestApplyMemoryLimitPIDNoop(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		pid   int
		bytes uint64
	}{
		{name: "zero pid", pid: 0, bytes: 1 << 20},
		{name: "negative pid", pid: -1, bytes: 1 << 20},
		{name: "zero bytes", pid: os.Getpid(), bytes: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if err := applyMemoryLimitPID(tt.pid, tt.bytes); err != nil {
				t.Fatalf("applyMemoryLimitPID(%d, %d) = %v, want nil", tt.pid, tt.bytes, err)
			}
		})
	}
}

// TestStopForcePathContextCanceled drives the force arm (timeout <= 5ms) onto
// its ctx.Done branch: the synthetic proc's done channel never closes.
func TestStopForcePathContextCanceled(t *testing.T) {
	t.Parallel()
	m := New()
	injectProc(t, m, "p", deadPID(t))
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()
	err := m.Stop(ctx, "p", time.Millisecond)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Stop = %v, want context.Canceled", err)
	}
}

// TestStopForcePathHardTimeout drives the force arm onto the bounded wait that
// reports an unkillable child.
func TestStopForcePathHardTimeout(t *testing.T) {
	t.Parallel()
	m := New()
	m.forceKillTimeout = 10 * time.Millisecond
	injectProc(t, m, "p", deadPID(t))
	err := m.Stop(context.Background(), "p", time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "did not exit after force kill") {
		t.Fatalf("Stop = %v, want force-kill timeout error", err)
	}
}

// TestStopForcePathWithJob covers forceKill's Job Object arm: a non-nil handle
// short-circuits the pgid kill entirely.
func TestStopForcePathWithJob(t *testing.T) {
	t.Parallel()
	m := New()
	p := injectProc(t, m, "p", deadPID(t))
	p.job = &jobHandle{}
	close(p.done)
	if err := m.Stop(context.Background(), "p", time.Millisecond); err != nil {
		t.Fatalf("Stop = %v, want nil", err)
	}
}

// TestStopForcePathSkipsKillWhenReaped proves forceKill declines to signal a
// process group whose leader has already been reaped, since the PGID may have
// been recycled onto an unrelated process.
func TestStopForcePathSkipsKillWhenReaped(t *testing.T) {
	t.Parallel()
	m := New()
	m.forceKillTimeout = 10 * time.Millisecond
	// pid 0 would make killTree fail loudly if it were ever reached.
	p := injectProc(t, m, "p", 0)
	p.reaped.Store(true)
	err := m.Stop(context.Background(), "p", time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "did not exit after force kill") {
		t.Fatalf("Stop = %v, want force-kill timeout error", err)
	}
}

// TestStopSkipsSigtermWhenReaped proves a already-reaped child is never signalled
// on the graceful path, so a recycled PGID cannot be hit.
func TestStopSkipsSigtermWhenReaped(t *testing.T) {
	t.Parallel()
	m := New()
	// pid 0 would make killTree fail loudly if it were ever reached.
	p := injectProc(t, m, "p", 0)
	p.reaped.Store(true)
	close(p.done)
	if err := m.Stop(context.Background(), "p", time.Second); err != nil {
		t.Fatalf("Stop = %v, want nil", err)
	}
}

// TestStopGracefulContextCanceled cancels during the grace period, before the
// escalation timer fires.
func TestStopGracefulContextCanceled(t *testing.T) {
	t.Parallel()
	m := New()
	injectProc(t, m, "p", deadPID(t))
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()
	err := m.Stop(ctx, "p", 10*time.Second)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Stop = %v, want context.Canceled", err)
	}
}

// TestStopEscalatedContextCanceled lets the grace timer fire, then cancels while
// waiting on the reaper after the force kill.
func TestStopEscalatedContextCanceled(t *testing.T) {
	t.Parallel()
	m := New()
	injectProc(t, m, "p", deadPID(t))
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(60 * time.Millisecond)
		cancel()
	}()
	err := m.Stop(ctx, "p", 10*time.Millisecond)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Stop = %v, want context.Canceled", err)
	}
}

// TestStopEscalatedHardTimeout lets the grace timer fire and then exhausts the
// post-force-kill wait.
func TestStopEscalatedHardTimeout(t *testing.T) {
	t.Parallel()
	m := New()
	m.forceKillTimeout = 10 * time.Millisecond
	injectProc(t, m, "p", deadPID(t))
	err := m.Stop(context.Background(), "p", 10*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "did not exit after force kill") {
		t.Fatalf("Stop = %v, want force-kill timeout error", err)
	}
}

// TestStopEscalatedDoneAfterForce lets the grace timer fire and then observes the
// reaper closing done.
func TestStopEscalatedDoneAfterForce(t *testing.T) {
	t.Parallel()
	m := New()
	p := injectProc(t, m, "p", deadPID(t))
	go func() {
		time.Sleep(40 * time.Millisecond)
		close(p.done)
	}()
	if err := m.Stop(context.Background(), "p", 10*time.Millisecond); err != nil {
		t.Fatalf("Stop = %v, want nil", err)
	}
}

func TestSignalReapedReportsNotRunning(t *testing.T) {
	t.Parallel()
	m := New()
	p := injectProc(t, m, "p", 0)
	p.reaped.Store(true)
	err := m.Signal(context.Background(), "p", syscall.SIGTERM)
	if !errors.Is(err, process.ErrNotRunning) {
		t.Fatalf("Signal = %v, want ErrNotRunning", err)
	}
}

func TestSignalDeadProcessGroupReportsNotRunning(t *testing.T) {
	t.Parallel()
	m := New()
	injectProc(t, m, "p", deadPID(t))
	err := m.Signal(context.Background(), "p", syscall.SIGTERM)
	if !errors.Is(err, process.ErrNotRunning) {
		t.Fatalf("Signal = %v, want ErrNotRunning (ESRCH mapping)", err)
	}
}

func TestSignalOtherErrorIsWrapped(t *testing.T) {
	t.Parallel()
	m := New()
	// pid 0 makes killTree fail with a non-ESRCH error, taking the generic arm.
	injectProc(t, m, "p", 0)
	err := m.Signal(context.Background(), "p", syscall.SIGTERM)
	if err == nil || errors.Is(err, process.ErrNotRunning) {
		t.Fatalf("Signal = %v, want a wrapped non-ESRCH error", err)
	}
	if !strings.Contains(err.Error(), "local: signal:") {
		t.Fatalf("Signal = %v, want a local: signal: wrapper", err)
	}
}

// TestStartJobAssignmentFailurePermissive covers the arm where job assignment
// fails but the profile does not require isolation, so Start continues.
func TestStartJobAssignmentFailurePermissive(t *testing.T) {
	t.Parallel()
	m := New()
	m.assignJobFn = func(int, string) (*jobHandle, error) { return nil, errors.New("no job object") }
	h, err := m.Start(context.Background(), process.StartSpec{
		ID:      "proc-01JOBFAILPERMISSIVE00001",
		Command: []string{"/bin/sleep", "30"},
		Sandbox: "permissive",
	})
	if err != nil {
		t.Fatalf("Start = %v, want nil (permissive continues without a job)", err)
	}
	if h.PID <= 0 {
		t.Fatalf("PID = %d, want > 0", h.PID)
	}
	// The child was force-killed by the failure arm; the reaper still finalizes.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := m.Wait(ctx, h.ID); err != nil {
		t.Fatalf("Wait: %v", err)
	}
}

// TestStartJobAssignmentFailureStrict covers the fail-closed arm.
func TestStartJobAssignmentFailureStrict(t *testing.T) {
	t.Parallel()
	m := New()
	m.assignJobFn = func(int, string) (*jobHandle, error) { return nil, errors.New("no job object") }
	_, err := m.Start(context.Background(), process.StartSpec{
		ID:      "proc-01JOBFAILSTRICT00000001",
		Command: []string{"/bin/sleep", "30"},
		Cwd:     t.TempDir(),
		Sandbox: "strict",
	})
	if !errors.Is(err, process.ErrSandboxFailed) {
		t.Fatalf("Start = %v, want ErrSandboxFailed", err)
	}
}

// TestStartConcurrentDuplicateLosesRace covers the re-check under m.mu: a
// competing Start won the ID between this call's first check and its insert, so
// this call must tear its own child (and job) down and report ErrAlreadyExists.
func TestStartConcurrentDuplicateLosesRace(t *testing.T) {
	t.Parallel()
	const id = "proc-01STARTRECHECKDUP00001"
	m := New()
	// The job-assignment seam is the last hook before the re-check, so it is a
	// deterministic stand-in for the competing Start winning the map slot.
	m.assignJobFn = func(int, string) (*jobHandle, error) {
		injectProc(t, m, id, 0)
		return &jobHandle{}, nil
	}
	_, err := m.Start(context.Background(), process.StartSpec{
		ID:      id,
		Command: []string{"/bin/sleep", "30"},
		LogDir:  t.TempDir(),
	})
	if !errors.Is(err, process.ErrAlreadyExists) {
		t.Fatalf("Start = %v, want ErrAlreadyExists", err)
	}
}

// TestReapClosesJobHandle drives reap's job-teardown arm with a non-nil handle.
func TestReapClosesJobHandle(t *testing.T) {
	t.Parallel()
	m := New()
	m.assignJobFn = func(int, string) (*jobHandle, error) { return &jobHandle{}, nil }
	h, err := m.Start(context.Background(), process.StartSpec{
		ID:      "proc-01REAPJOBCLOSE000000001",
		Command: []string{"/bin/true"},
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	waited, err := m.Wait(context.Background(), h.ID)
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if waited.ExitCode == nil || *waited.ExitCode != 0 {
		t.Fatalf("ExitCode = %v, want 0", waited.ExitCode)
	}
	m.mu.Lock()
	job := m.procs[h.ID].job
	m.mu.Unlock()
	if job != nil {
		t.Fatal("reap left the job handle attached")
	}
}

// TestJobStubIsInert pins the non-Windows job shims: no handle, no error, no panic.
func TestJobStubIsInert(t *testing.T) {
	t.Parallel()
	j, err := assignJob(123, "off")
	if j != nil || err != nil {
		t.Fatalf("assignJob = (%v, %v), want (nil, nil)", j, err)
	}
	var nilHandle *jobHandle
	nilHandle.close()
	if err := nilHandle.terminate(); err != nil {
		t.Fatalf("terminate = %v, want nil", err)
	}
}
