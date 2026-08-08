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

// Package fake provides an in-memory engine.Engine for tests.
package fake

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/scrothers/pmmcp/internal/engine"
)

// Compile-time checks: the fake satisfies the core Engine and every optional
// capability, so it can stand in for a real backend in any consumer test.
var (
	_ engine.Engine      = (*Engine)(nil)
	_ engine.Inspector   = (*Engine)(nil)
	_ engine.Waiter      = (*Engine)(nil)
	_ engine.Remover     = (*Engine)(nil)
	_ engine.ImagePuller = (*Engine)(nil)
	_ engine.Lister      = (*Engine)(nil)
	_ engine.Versioner   = (*Engine)(nil)
)

// Engine is an in-memory container engine.
type Engine struct {
	mu         sync.Mutex
	containers map[string]*container
	seq        atomic.Uint64

	// Runs records successful Run specs (for assertions).
	Runs []engine.RunSpec
	// Pulled records PullImage calls (for assertions).
	Pulled []string
	// AvailableFunc overrides Available when non-nil.
	AvailableFunc func(context.Context) bool
	// Per-operation error overrides (nil = success).
	RunErr     error
	StopErr    error
	InspectErr error
	WaitErr    error
	RemoveErr  error
	PullErr    error
	ListErr    error
	VersionErr error
	// VersionInfo is returned by Version when VersionErr is nil.
	VersionInfo engine.VersionInfo
}

type container struct {
	id       string
	spec     engine.RunSpec
	running  bool
	exitCode int
	oom      bool
	health   string
	logs     string
	done     chan struct{}
}

// New returns an empty fake Engine.
func New() *Engine {
	return &Engine{containers: make(map[string]*container)}
}

// Name implements engine.Engine.
func (e *Engine) Name() string { return "fake" }

// Available implements engine.Engine.
func (e *Engine) Available(ctx context.Context) bool {
	if e.AvailableFunc != nil {
		return e.AvailableFunc(ctx)
	}
	return ctx.Err() == nil
}

// Run implements engine.Engine.
func (e *Engine) Run(ctx context.Context, spec engine.RunSpec) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if e.RunErr != nil {
		return "", e.RunErr
	}
	if spec.Image == "" {
		return "", fmt.Errorf("%w: empty image", engine.ErrInvalidSpec)
	}
	id := "fake-" + strconv.FormatUint(e.seq.Add(1), 10)
	e.mu.Lock()
	defer e.mu.Unlock()
	e.Runs = append(e.Runs, spec)
	e.containers[id] = &container{
		id:      id,
		spec:    spec,
		running: true,
		logs:    "fake container " + id + " running\n",
		done:    make(chan struct{}),
	}
	return id, nil
}

// Stop implements engine.Engine (graceful exit with code 0).
func (e *Engine) Stop(ctx context.Context, containerID string, _ time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if e.StopErr != nil {
		return e.StopErr
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	c, ok := e.containers[containerID]
	if !ok {
		return fmt.Errorf("%w: %s", engine.ErrNotFound, containerID)
	}
	e.markExitedLocked(c, 0, false)
	return nil
}

// Logs implements engine.Engine.
func (e *Engine) Logs(ctx context.Context, containerID string, _ int) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	c, ok := e.containers[containerID]
	if !ok {
		return "", fmt.Errorf("%w: %s", engine.ErrNotFound, containerID)
	}
	return c.logs, nil
}

// Inspect implements engine.Inspector.
func (e *Engine) Inspect(ctx context.Context, containerID string) (engine.Status, error) {
	if err := ctx.Err(); err != nil {
		return engine.Status{}, err
	}
	if e.InspectErr != nil {
		return engine.Status{}, e.InspectErr
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	c, ok := e.containers[containerID]
	if !ok {
		return engine.Status{}, fmt.Errorf("%w: %s", engine.ErrNotFound, containerID)
	}
	st := engine.Status{
		ID:        c.id,
		Name:      c.spec.Name,
		Image:     c.spec.Image,
		State:     engine.StateExited,
		Running:   c.running,
		ExitCode:  c.exitCode,
		OOMKilled: c.oom,
		Health:    c.health,
		Labels:    c.spec.Labels,
	}
	if c.running {
		st.State = engine.StateRunning
	}
	return st, nil
}

// Wait implements engine.Waiter, blocking until the container exits.
func (e *Engine) Wait(ctx context.Context, containerID string) (int, error) {
	if e.WaitErr != nil {
		return 0, e.WaitErr
	}
	e.mu.Lock()
	c, ok := e.containers[containerID]
	if !ok {
		e.mu.Unlock()
		return 0, fmt.Errorf("%w: %s", engine.ErrNotFound, containerID)
	}
	if !c.running {
		code := c.exitCode
		e.mu.Unlock()
		return code, nil
	}
	done := c.done
	e.mu.Unlock()

	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	case <-done:
		e.mu.Lock()
		code := c.exitCode
		e.mu.Unlock()
		return code, nil
	}
}

// Remove implements engine.Remover.
func (e *Engine) Remove(ctx context.Context, containerID string, force bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if e.RemoveErr != nil {
		return e.RemoveErr
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	c, ok := e.containers[containerID]
	if !ok {
		return fmt.Errorf("%w: %s", engine.ErrNotFound, containerID)
	}
	if c.running && !force {
		return fmt.Errorf("engine/fake: cannot remove running container %s without force", containerID)
	}
	delete(e.containers, containerID)
	return nil
}

// PullImage implements engine.ImagePuller.
func (e *Engine) PullImage(ctx context.Context, image string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if e.PullErr != nil {
		return e.PullErr
	}
	if image == "" {
		return fmt.Errorf("%w: empty image", engine.ErrInvalidSpec)
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.Pulled = append(e.Pulled, image)
	return nil
}

// List implements engine.Lister, returning containers matching every label.
func (e *Engine) List(ctx context.Context, labels map[string]string) ([]engine.Container, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if e.ListErr != nil {
		return nil, e.ListErr
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	var out []engine.Container
	for _, c := range e.containers {
		if !labelsMatch(c.spec.Labels, labels) {
			continue
		}
		state := engine.StateExited
		if c.running {
			state = engine.StateRunning
		}
		out = append(out, engine.Container{ID: c.id, Name: c.spec.Name, Image: c.spec.Image, State: state, Labels: c.spec.Labels})
	}
	return out, nil
}

// Version implements engine.Versioner.
func (e *Engine) Version(ctx context.Context) (engine.VersionInfo, error) {
	if err := ctx.Err(); err != nil {
		return engine.VersionInfo{}, err
	}
	if e.VersionErr != nil {
		return engine.VersionInfo{}, e.VersionErr
	}
	return e.VersionInfo, nil
}

// Exit simulates a container exiting on its own with the given code (oomKilled
// marks it OOM-killed), so tests can exercise natural-exit detection.
func (e *Engine) Exit(containerID string, code int, oomKilled bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if c, ok := e.containers[containerID]; ok {
		e.markExitedLocked(c, code, oomKilled)
	}
}

// SetHealth sets a container's healthcheck status reported by Inspect.
func (e *Engine) SetHealth(containerID, health string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if c, ok := e.containers[containerID]; ok {
		c.health = health
	}
}

// markExitedLocked transitions a running container to exited exactly once.
func (e *Engine) markExitedLocked(c *container, code int, oom bool) {
	if !c.running {
		return
	}
	c.running = false
	c.exitCode = code
	c.oom = oom
	close(c.done)
}

// labelsMatch reports whether have contains every key/value pair in want.
func labelsMatch(have, want map[string]string) bool {
	for k, v := range want {
		if have[k] != v {
			return false
		}
	}
	return true
}
