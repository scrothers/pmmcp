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

// Package fake provides an in-memory process.Manager for tests.
package fake

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/scrothers/pmmcp/internal/domain"
	"github.com/scrothers/pmmcp/internal/process"
)

// Compile-time interface check.
var _ process.Manager = (*Manager)(nil)

// Manager is an in-memory process.Manager.
// Start records specs; Stop marks processes exited without spawning OS processes.
type Manager struct {
	mu      sync.Mutex
	procs   map[string]*entry
	nextPID int

	// Starts is the ordered list of StartSpec values passed to Start (for assertions).
	Starts []process.StartSpec
}

type entry struct {
	handle process.Handle
	done   chan struct{}
}

// New returns an empty fake Manager.
func New() *Manager {
	return &Manager{
		procs:   make(map[string]*entry),
		nextPID: 1000,
	}
}

// Start records the spec and marks the process running with a synthetic PID.
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
	defer m.mu.Unlock()

	if _, ok := m.procs[spec.ID]; ok {
		return nil, fmt.Errorf("%w: %s", process.ErrAlreadyExists, spec.ID)
	}

	m.Starts = append(m.Starts, spec)
	m.nextPID++
	pid := m.nextPID
	h := process.Handle{
		ID:     spec.ID,
		PID:    pid,
		Status: domain.StatusRunning,
	}
	m.procs[spec.ID] = &entry{
		handle: h,
		done:   make(chan struct{}),
	}
	return cloneHandle(&h), nil
}

// Stop marks the process exited with exit code 0.
func (m *Manager) Stop(ctx context.Context, id string, _ time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	e, ok := m.procs[id]
	if !ok {
		return fmt.Errorf("%w: %s", process.ErrNotFound, id)
	}
	if e.handle.Status == domain.StatusExited || e.handle.Status == domain.StatusFailed ||
		e.handle.Status == domain.StatusCrashed {
		return nil
	}
	code := 0
	e.handle.Status = domain.StatusExited
	e.handle.ExitCode = &code
	select {
	case <-e.done:
	default:
		close(e.done)
	}
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
	// Already terminal?
	if isTerminal(e.handle.Status) {
		h := cloneHandle(&e.handle)
		m.mu.Unlock()
		return h, nil
	}
	m.mu.Unlock()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-done:
		m.mu.Lock()
		h := cloneHandle(&e.handle)
		m.mu.Unlock()
		return h, nil
	}
}

// Inspect returns the current handle snapshot.
func (m *Manager) Inspect(ctx context.Context, id string) (*process.Handle, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.procs[id]
	if !ok {
		return nil, fmt.Errorf("%w: %s", process.ErrNotFound, id)
	}
	return cloneHandle(&e.handle), nil
}

// Signal is a no-op for running processes; fails if missing or not running.
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
	if isTerminal(e.handle.Status) {
		return fmt.Errorf("%w: %s", process.ErrNotRunning, id)
	}
	return nil
}

func isTerminal(s domain.Status) bool {
	switch s {
	case domain.StatusExited, domain.StatusFailed, domain.StatusCrashed:
		return true
	default:
		return false
	}
}

func cloneHandle(h *process.Handle) *process.Handle {
	out := *h
	if h.ExitCode != nil {
		c := *h.ExitCode
		out.ExitCode = &c
	}
	return &out
}
