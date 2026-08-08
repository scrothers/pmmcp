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

// Package container implements process.Manager for container-backed workloads.
package container

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/scrothers/pmmcp/internal/domain"
	"github.com/scrothers/pmmcp/internal/engine"
	"github.com/scrothers/pmmcp/internal/process"
)

// Compile-time interface check.
var _ process.Manager = (*Manager)(nil)

// DefaultStopTimeout is used when Stop is called with a non-positive timeout.
const DefaultStopTimeout = 10 * time.Second

// Manager runs processes as containers via an engine.Engine.
//
// Handles use PID=0 and track the engine container id in Handle.ContainerID
// and an internal map.
type Manager struct {
	eng engine.Engine

	mu    sync.Mutex
	procs map[string]*entry
}

type entry struct {
	id          string
	containerID string
	status      domain.Status
	exitCode    *int
	done        chan struct{}
}

// New returns a container-backed process Manager using eng.
func New(eng engine.Engine) *Manager {
	if eng == nil {
		panic("process/container: nil engine")
	}
	return &Manager{
		eng:   eng,
		procs: make(map[string]*entry),
	}
}

// Start launches a container for the given spec.
// Image is required. Command may be empty to use the image default.
func (m *Manager) Start(ctx context.Context, spec process.StartSpec) (*process.Handle, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if spec.ID == "" {
		return nil, fmt.Errorf("%w: empty id", process.ErrInvalidSpec)
	}
	if spec.Image == "" {
		return nil, fmt.Errorf("%w: empty image", process.ErrInvalidSpec)
	}
	// Allow empty Command for image default entrypoint; when present, validate.
	if len(spec.Command) > 0 {
		if err := domain.ValidateCommand(spec.Command); err != nil {
			return nil, fmt.Errorf("%w: %w", process.ErrInvalidSpec, err)
		}
	}

	m.mu.Lock()
	if _, ok := m.procs[spec.ID]; ok {
		m.mu.Unlock()
		return nil, fmt.Errorf("%w: %s", process.ErrAlreadyExists, spec.ID)
	}
	m.mu.Unlock()

	run, err := buildRunSpec(spec)
	if err != nil {
		return nil, err
	}

	cid, err := m.eng.Run(ctx, run)
	if err != nil {
		return nil, fmt.Errorf("process/container: run: %w", err)
	}

	e := &entry{
		id:          spec.ID,
		containerID: cid,
		status:      domain.StatusRunning,
		done:        make(chan struct{}),
	}

	m.mu.Lock()
	if _, ok := m.procs[spec.ID]; ok {
		m.mu.Unlock()
		_ = m.eng.Stop(ctx, cid, DefaultStopTimeout)
		return nil, fmt.Errorf("%w: %s", process.ErrAlreadyExists, spec.ID)
	}
	m.procs[spec.ID] = e
	m.mu.Unlock()

	return e.handle(), nil
}

// Stop stops the container for id.
func (m *Manager) Stop(ctx context.Context, id string, timeout time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if timeout <= 0 {
		timeout = DefaultStopTimeout
	}

	m.mu.Lock()
	e, ok := m.procs[id]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("%w: %s", process.ErrNotFound, id)
	}
	if isTerminal(e.status) {
		m.mu.Unlock()
		return nil
	}
	e.status = domain.StatusStopping
	cid := e.containerID
	done := e.done
	m.mu.Unlock()

	if err := m.eng.Stop(ctx, cid, timeout); err != nil {
		return fmt.Errorf("process/container: stop: %w", err)
	}

	m.mu.Lock()
	code := 0
	e.exitCode = &code
	e.status = domain.StatusExited
	select {
	case <-done:
	default:
		close(done)
	}
	m.mu.Unlock()
	return nil
}

// Wait blocks until the process is stopped or ctx is canceled.
func (m *Manager) Wait(ctx context.Context, id string) (*process.Handle, error) {
	m.mu.Lock()
	e, ok := m.procs[id]
	if !ok {
		m.mu.Unlock()
		return nil, fmt.Errorf("%w: %s", process.ErrNotFound, id)
	}
	done := e.done
	if isTerminal(e.status) {
		h := e.handle()
		m.mu.Unlock()
		return h, nil
	}
	m.mu.Unlock()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-done:
		m.mu.Lock()
		h := e.handle()
		m.mu.Unlock()
		return h, nil
	}
}

// Inspect returns the current handle. When the engine supports introspection,
// it refreshes state from the engine so a container that exited on its own — or
// turned unhealthy — is reflected, rather than being pinned to "running" until
// Stop is called.
func (m *Manager) Inspect(ctx context.Context, id string) (*process.Handle, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.Lock()
	e, ok := m.procs[id]
	if !ok {
		m.mu.Unlock()
		return nil, fmt.Errorf("%w: %s", process.ErrNotFound, id)
	}
	insp, canInspect := m.eng.(engine.Inspector)
	if isTerminal(e.status) || !canInspect {
		h := e.handle()
		m.mu.Unlock()
		return h, nil
	}
	cid := e.containerID
	m.mu.Unlock()

	st, err := insp.Inspect(ctx, cid)
	if err != nil {
		// Cannot refresh (transient engine error, or container already gone):
		// return the last-known handle rather than failing Inspect.
		m.mu.Lock()
		h := e.handle()
		m.mu.Unlock()
		return h, nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	// Re-check under lock: a concurrent Stop may have finalized the entry while
	// the engine call was in flight.
	if e.status == domain.StatusRunning || e.status == domain.StatusUnhealthy {
		switch {
		case !st.Running:
			status, code := exitStatus(st)
			e.status = status
			e.exitCode = &code
			select {
			case <-e.done:
			default:
				close(e.done)
			}
		case st.Health == "unhealthy":
			e.status = domain.StatusUnhealthy
		default:
			e.status = domain.StatusRunning
		}
	}
	return e.handle(), nil
}

// exitStatus maps an exited engine.Status to a domain status and exit code: a
// clean exit (code 0) is Exited; a non-zero exit or an OOM kill is Crashed.
func exitStatus(st engine.Status) (domain.Status, int) {
	if st.OOMKilled || st.ExitCode != 0 {
		return domain.StatusCrashed, st.ExitCode
	}
	return domain.StatusExited, st.ExitCode
}

// Signal is not supported for container-backed processes in this MVP.
func (m *Manager) Signal(ctx context.Context, id string, _ os.Signal) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.procs[id]
	if !ok {
		return fmt.Errorf("%w: %s", process.ErrNotFound, id)
	}
	if isTerminal(e.status) {
		return fmt.Errorf("%w: %s", process.ErrNotRunning, id)
	}
	// Container engines do not map arbitrary OS signals in this layer.
	return fmt.Errorf("process/container: signal not supported")
}

// Logs returns engine logs for a managed process (helper for higher layers).
func (m *Manager) Logs(ctx context.Context, id string, tail int) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	m.mu.Lock()
	e, ok := m.procs[id]
	if !ok {
		m.mu.Unlock()
		return "", fmt.Errorf("%w: %s", process.ErrNotFound, id)
	}
	cid := e.containerID
	m.mu.Unlock()
	return m.eng.Logs(ctx, cid, tail)
}

func (e *entry) handle() *process.Handle {
	h := &process.Handle{
		ID:          e.id,
		PID:         0,
		ContainerID: e.containerID,
		Status:      e.status,
	}
	if e.exitCode != nil {
		c := *e.exitCode
		h.ExitCode = &c
	}
	return h
}

// buildRunSpec translates a StartSpec into an engine.RunSpec, applying strict
// container hardening (runtime-container.md "Strict defaults") for the
// strict/standard profiles: cap_drop ALL, read-only rootfs,
// no-new-privileges, non-privileged, and loopback-only port publishing. It
// also stamps the io.pmmcp reconcile labels so running containers can be
// re-attached by label after a daemon restart, and rejects the docker/podman
// socket env footgun in strict/standard (sandbox-strict AC).
func buildRunSpec(spec process.StartSpec) (engine.RunSpec, error) {
	run := engine.RunSpec{
		Name:    spec.Name,
		Image:   spec.Image,
		Command: spec.Command,
		Env:     envSliceToMap(spec.Env),
		Ports:   append([]string(nil), spec.Ports...),
		Labels:  reconcileLabels(spec),
	}

	strict := strings.EqualFold(spec.Sandbox, "strict") || strings.EqualFold(spec.Sandbox, "standard")
	if strict {
		// Reject the common docker/podman socket footgun before spawning.
		for _, e := range spec.Env {
			if strings.Contains(e, "docker.sock") || strings.Contains(e, "podman.sock") {
				return engine.RunSpec{}, fmt.Errorf("%w: strict sandbox forbids docker/podman socket mounts", process.ErrSandboxFailed)
			}
		}
		run.CapDrop = []string{"ALL"}
		run.ReadOnlyRootfs = true
		run.SecurityOpt = []string{"no-new-privileges"}
		run.Privileged = false
		// Network left at engine default (bridge/rootless); host networking is
		// never requested. Ports default to loopback (PublishAllInterfaces=false).
	} else {
		// permissive/off: publish on all interfaces as requested.
		run.PublishAllInterfaces = true
	}
	return run, nil
}

// reconcileLabels stamps the io.pmmcp label convention used to re-attach
// running containers after a daemon restart (runtime-container.md).
func reconcileLabels(spec process.StartSpec) map[string]string {
	labels := map[string]string{"io.pmmcp.proc_id": spec.ID}
	if spec.Name != "" {
		labels["io.pmmcp.name"] = spec.Name
	}
	return labels
}

func envSliceToMap(env []string) map[string]string {
	if len(env) == 0 {
		return nil
	}
	out := make(map[string]string, len(env))
	for _, kv := range env {
		k, v, ok := strings.Cut(kv, "=")
		if !ok || k == "" {
			continue
		}
		out[k] = v
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
