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

// Package local implements process.Manager for OS processes.
package local

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/scrothers/pmmcp/internal/domain"
	"github.com/scrothers/pmmcp/internal/logcap"
	"github.com/scrothers/pmmcp/internal/process"
)

// Compile-time interface check.
var _ process.Manager = (*Manager)(nil)

// DefaultStopTimeout is used when Stop is called with a non-positive timeout.
const DefaultStopTimeout = 10 * time.Second

// defaultForceKillTimeout bounds the wait for the reaper after a force kill.
// A SIGKILL (or Job Object terminate) that has not been observed by the reaper
// within this window means the child is unkillable (uninterruptible sleep), and
// Stop reports failure rather than blocking its caller forever.
const defaultForceKillTimeout = 5 * time.Second

// Manager runs and tracks OS processes without shell wrapping.
type Manager struct {
	mu    sync.Mutex
	procs map[string]*proc

	// assignJobFn creates the platform job object for a freshly started child.
	// It is a field, not a direct call, purely so tests can drive the
	// assignment-failure paths on platforms where assignJob is a no-op.
	// New always installs assignJob.
	assignJobFn func(pid int, profile string) (*jobHandle, error)
	// forceKillTimeout is defaultForceKillTimeout; a field so tests can shrink it.
	forceKillTimeout time.Duration
}

type proc struct {
	id       string
	cmd      *exec.Cmd
	pid      int
	status   domain.Status
	exitCode *int
	done     chan struct{}
	// stdout/stderr are size-aware, redacting capture sinks (nil when no LogDir).
	// They rotate live at the cap and must be Closed on process exit to
	// flush the final partial line and release the fd.
	stdout *logcap.RotatingWriter
	stderr *logcap.RotatingWriter
	// job is a Windows Job Object (nil on Unix).
	job *jobHandle
	// reaped is set the instant cmd.Wait returns, i.e. once the kernel has
	// reaped the child and freed its PID/PGID. Signalers consult it before
	// delivering to the process group so a recycled PGID is never targeted.
	reaped atomic.Bool
}

// New returns a local process Manager.
func New() *Manager {
	return &Manager{
		procs:            make(map[string]*proc),
		assignJobFn:      assignJob,
		forceKillTimeout: defaultForceKillTimeout,
	}
}

// Start launches the process described by spec.
// The Start context bounds setup only; the child outlives it until Stop or natural exit.
func (m *Manager) Start(ctx context.Context, spec process.StartSpec) (*process.Handle, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if spec.ID == "" {
		return nil, fmt.Errorf("%w: empty id", process.ErrInvalidSpec)
	}
	if err := domain.ValidateCommand(spec.Command); err != nil {
		return nil, fmt.Errorf("%w: %w", process.ErrInvalidSpec, err)
	}

	m.mu.Lock()
	if old, ok := m.procs[spec.ID]; ok {
		if !isTerminal(old.status) {
			m.mu.Unlock()
			return nil, fmt.Errorf("%w: %s", process.ErrAlreadyExists, spec.ID)
		}
		// Allow restart of exited processes under the same ID (auto_restart / restartByID).
		delete(m.procs, spec.ID)
	}
	m.mu.Unlock()

	argv := spec.Command
	// Restrictive sandbox: platform FS isolation (Linux bwrap, Darwin sandbox-exec).
	// Fail closed when strict/standard cannot isolate.
	sb := strings.ToLower(strings.TrimSpace(spec.Sandbox))
	if sb == "strict" || sb == "standard" {
		root := spec.Cwd
		if root == "" {
			root, _ = os.Getwd()
		}
		wrapped, err := wrapSandbox(spec.Command, root, sb)
		if err != nil {
			return nil, err
		}
		argv = wrapped
	}
	cmd := exec.Command(argv[0], argv[1:]...) //nolint:gosec // argv is intentional; no shell
	cmd.Dir = spec.Cwd
	// Always set an explicit environment; never leave cmd.Env nil, which would
	// make os/exec hand the child the daemon's entire environment (secrets and
	// all). Default is a minimal allowlist (runtime-local.md).
	cmd.Env = buildChildEnv(spec)
	setSysProcAttr(cmd)
	// Windows-only: for the standard profile, launch the child under a
	// low-integrity primary token so it cannot write outside low-integrity
	// locations (write confinement on top of the Job Object). No-op on other
	// platforms and profiles. Best-effort: a host that can't build the token
	// still runs under the Job Object. The returned cleanup closes the token
	// after cmd.Start has consumed it.
	defer applySandboxToken(cmd, sb)()
	if spec.MemoryBytes > 0 {
		applyMemoryLimit(cmd, spec.MemoryBytes)
	}

	var stdout, stderr *logcap.RotatingWriter
	if spec.LogDir != "" {
		//: dirs 0700, files 0600 (via logcap capturer). The rotating
		// writers enforce the size cap continuously and redact each line via the
		// secret package's shared default (secrets registered by the daemon at
		// process start), so rotation fires live and secrets never hit disk.
		lc, err := logcap.New(spec.LogDir, 10, 5)
		if err != nil {
			return nil, fmt.Errorf("local: logcap: %w", err)
		}
		stdout, err = lc.OpenStdoutWriter()
		if err != nil {
			return nil, fmt.Errorf("local: open stdout.log: %w", err)
		}
		stderr, err = lc.OpenStderrWriter()
		if err != nil {
			_ = stdout.Close()
			return nil, fmt.Errorf("local: open stderr.log: %w", err)
		}
		cmd.Stdout = stdout
		cmd.Stderr = stderr
	}

	if err := cmd.Start(); err != nil {
		if stdout != nil {
			_ = stdout.Close()
		}
		if stderr != nil {
			_ = stderr.Close()
		}
		return nil, fmt.Errorf("local: start: %w", err)
	}

	p := &proc{
		id:     spec.ID,
		cmd:    cmd,
		pid:    cmd.Process.Pid,
		status: domain.StatusRunning,
		done:   make(chan struct{}),
		stdout: stdout,
		stderr: stderr,
	}

	// Assign the platform job object for tree-kill / kill-on-close.
	// Windows returns a real handle; Unix's assignJob is a no-op returning a nil
	// handle and no error, so the failure arm below is Windows-only in practice.
	// Fail closed for strict/standard when job assignment fails.
	if j, err := m.assignJobFn(p.pid, sb); err != nil {
		_ = killTree(p.pid, syscall.SIGKILL)
		_, _ = cmd.Process.Wait()
		p.closeLogs()
		if sb == "strict" || sb == "standard" {
			return nil, fmt.Errorf("%w: job object: %w", process.ErrSandboxFailed, err)
		}
		// permissive/off: continue without job
	} else {
		p.job = j
	}

	m.mu.Lock()
	// Re-check under lock in case of concurrent Start with same ID.
	if old, ok := m.procs[spec.ID]; ok && !isTerminal(old.status) {
		m.mu.Unlock()
		if p.job != nil {
			_ = p.job.terminate()
			p.job.close()
		}
		_ = killTree(p.pid, syscall.SIGKILL)
		_, _ = cmd.Process.Wait()
		p.closeLogs()
		return nil, fmt.Errorf("%w: %s", process.ErrAlreadyExists, spec.ID)
	}
	m.procs[spec.ID] = p
	// Build the returned handle while still holding m.mu and before reap is
	// spawned, so the snapshot read can never race the reaper's status/exitCode
	// writes (the reaper does not exist yet).
	h := p.handle()
	m.mu.Unlock()

	if spec.MemoryBytes > 0 {
		_ = applyMemoryLimitPID(p.pid, spec.MemoryBytes)
	}

	go m.reap(p)

	return h, nil
}

func (m *Manager) reap(p *proc) {
	err := p.cmd.Wait()
	// The child is now reaped and its PID/PGID may be recycled by the kernel at
	// any moment; flag it before finalizing so a concurrent Stop/Signal that
	// observed a still-running status does not signal a reused process group.
	p.reaped.Store(true)
	code := 0
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			code = ee.ExitCode()
		} else {
			code = -1
		}
	}

	m.mu.Lock()
	p.exitCode = &code
	// Preserve StatusStopping transition only while waiting; then terminal.
	if p.status == domain.StatusStopping || p.status == domain.StatusRunning || p.status == domain.StatusStarting {
		p.status = domain.StatusExited
	}
	if p.job != nil {
		p.job.close()
		p.job = nil
	}
	p.closeLogs()
	close(p.done)
	m.mu.Unlock()
}

// Stop gracefully terminates the process tree, then force-kills after timeout.
func (m *Manager) Stop(ctx context.Context, id string, timeout time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if timeout <= 0 {
		timeout = DefaultStopTimeout
	}

	m.mu.Lock()
	p, ok := m.procs[id]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("%w: %s", process.ErrNotFound, id)
	}
	if isTerminal(p.status) {
		m.mu.Unlock()
		return nil
	}
	p.status = domain.StatusStopping
	pid := p.pid
	done := p.done
	job := p.job
	m.mu.Unlock()

	// Force path (very short timeout): skip graceful and kill immediately.
	if timeout <= 5*time.Millisecond {
		forceKill(p, pid, job)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-done:
			return nil
		case <-time.After(m.forceKillTimeout):
			return fmt.Errorf("local: stop: process %s did not exit after force kill", id)
		}
	}

	// Graceful: SIGTERM to process group (Unix) or process (Windows).
	// Ignore errors (already exited / race); wait or escalate below.
	// Skip if already reaped so we never signal a recycled PGID.
	if !p.reaped.Load() {
		_ = killTree(pid, syscall.SIGTERM)
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		forceKill(p, pid, job)
		return ctx.Err()
	case <-done:
		return nil
	case <-timer.C:
		// Escalate: job terminate (Windows) or SIGKILL tree (Unix).
		forceKill(p, pid, job)
		// Wait for reaper (bounded by caller ctx).
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-done:
			return nil
		case <-time.After(m.forceKillTimeout):
			return fmt.Errorf("local: stop: process %s did not exit after force kill", id)
		}
	}
}

// forceKill terminates the process tree. The Windows Job Object path targets a
// kernel object (safe regardless of PID reuse); the Unix pgid path is skipped
// once the child has been reaped so a recycled process group is never signaled.
func forceKill(p *proc, pid int, job *jobHandle) {
	if job != nil {
		_ = job.terminate()
		return
	}
	if p.reaped.Load() {
		return
	}
	_ = killTree(pid, syscall.SIGKILL)
}

// Wait blocks until the process exits or ctx is canceled.
func (m *Manager) Wait(ctx context.Context, id string) (*process.Handle, error) {
	m.mu.Lock()
	p, ok := m.procs[id]
	if !ok {
		m.mu.Unlock()
		return nil, fmt.Errorf("%w: %s", process.ErrNotFound, id)
	}
	done := p.done
	if isTerminal(p.status) {
		h := p.handle()
		m.mu.Unlock()
		return h, nil
	}
	m.mu.Unlock()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-done:
		m.mu.Lock()
		h := p.handle()
		m.mu.Unlock()
		return h, nil
	}
}

// Inspect returns the current handle without waiting for exit.
func (m *Manager) Inspect(ctx context.Context, id string) (*process.Handle, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.procs[id]
	if !ok {
		return nil, fmt.Errorf("%w: %s", process.ErrNotFound, id)
	}
	return p.handle(), nil
}

// Signal delivers sig to the managed process tree.
func (m *Manager) Signal(ctx context.Context, id string, sig os.Signal) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	p, ok := m.procs[id]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("%w: %s", process.ErrNotFound, id)
	}
	if isTerminal(p.status) {
		m.mu.Unlock()
		return fmt.Errorf("%w: %s", process.ErrNotRunning, id)
	}
	pid := p.pid
	m.mu.Unlock()

	// If the child was reaped between the status check and here, its PGID may be
	// recycled; report not-running rather than signal an unrelated group.
	if p.reaped.Load() {
		return fmt.Errorf("%w: %s", process.ErrNotRunning, id)
	}

	if err := killTree(pid, sig); err != nil {
		if isNoProcessError(err) {
			return fmt.Errorf("%w: %s", process.ErrNotRunning, id)
		}
		return fmt.Errorf("local: signal: %w", err)
	}
	return nil
}

// handle returns a point-in-time snapshot of p. Callers must hold m.mu because
// it reads p.status and p.exitCode, which the reaper mutates under the lock.
func (p *proc) handle() *process.Handle {
	h := &process.Handle{
		ID:     p.id,
		PID:    p.pid,
		Status: p.status,
	}
	if p.exitCode != nil {
		c := *p.exitCode
		h.ExitCode = &c
	}
	return h
}

func (p *proc) closeLogs() {
	if p.stdout != nil {
		_ = p.stdout.Close()
		p.stdout = nil
	}
	if p.stderr != nil {
		_ = p.stderr.Close()
		p.stderr = nil
	}
}

// minimalEnvKeys is the allowlist inherited by children under the default
// "minimal" inherit mode: enough to locate binaries, a home, locale, and temp,
// but nothing that carries ambient secrets (runtime-local.md).
var minimalEnvKeys = []string{
	"PATH", "HOME", "USERPROFILE", "LANG", "LC_ALL", "LC_CTYPE",
	"TMPDIR", "TMP", "TEMP", "TZ", "TERM",
}

// buildChildEnv assembles the child's environment: a base determined by
// InheritEnv, then the spec's KEY=VAL overlays. The result is always non-nil so
// os/exec never falls back to inheriting the daemon's full environment.
func buildChildEnv(spec process.StartSpec) []string {
	var base []string
	switch strings.ToLower(strings.TrimSpace(spec.InheritEnv)) {
	case "full":
		base = os.Environ()
	case "none":
		base = []string{}
	default: // "minimal" / ""
		base = minimalEnv()
	}
	if len(spec.Env) == 0 {
		return base
	}
	return append(base, spec.Env...)
}

func minimalEnv() []string {
	out := make([]string, 0, len(minimalEnvKeys))
	for _, k := range minimalEnvKeys {
		if v, ok := os.LookupEnv(k); ok {
			out = append(out, k+"="+v)
		}
	}
	return out
}

func isTerminal(s domain.Status) bool {
	switch s {
	case domain.StatusExited, domain.StatusFailed, domain.StatusCrashed:
		return true
	default:
		return false
	}
}

func isNoProcessError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, os.ErrProcessDone) {
		return true
	}
	// ESRCH: no such process
	var errno syscall.Errno
	if errors.As(err, &errno) && errno == syscall.ESRCH {
		return true
	}
	return false
}
