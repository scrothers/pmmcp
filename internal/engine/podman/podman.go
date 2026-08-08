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

// Package podman implements engine.Engine using the Podman CLI.
package podman

import (
	"context"
	"time"

	"github.com/scrothers/pmmcp/internal/engine"
)

// Compile-time checks: podman satisfies the core Engine plus every optional
// capability interface (it shares CLIRunner with docker).
var (
	_ engine.Engine      = (*Engine)(nil)
	_ engine.Inspector   = (*Engine)(nil)
	_ engine.Waiter      = (*Engine)(nil)
	_ engine.Remover     = (*Engine)(nil)
	_ engine.ImagePuller = (*Engine)(nil)
	_ engine.Lister      = (*Engine)(nil)
	_ engine.Versioner   = (*Engine)(nil)
)

// Engine runs containers via the podman CLI.
//
// Available checks that the podman binary is on PATH (exec.LookPath).
// Run/Stop/Logs shell out to the CLI; if podman is missing they return
// engine.ErrUnavailable with a clear message.
//
// Socket discovery: rootless Podman uses $XDG_RUNTIME_DIR/podman/podman.sock
// when the CLI is invoked; this package does not open the socket directly.
type Engine struct {
	cli engine.CLIRunner
}

// New returns a Podman-backed Engine.
func New() *Engine {
	return &Engine{cli: engine.CLIRunner{Binary: "podman"}}
}

// Name implements engine.Engine.
func (e *Engine) Name() string { return "podman" }

// Available implements engine.Engine.
func (e *Engine) Available(ctx context.Context) bool {
	return e.cli.Available(ctx)
}

// Run implements engine.Engine.
func (e *Engine) Run(ctx context.Context, spec engine.RunSpec) (string, error) {
	return e.cli.Run(ctx, spec)
}

// Stop implements engine.Engine.
func (e *Engine) Stop(ctx context.Context, containerID string, timeout time.Duration) error {
	return e.cli.Stop(ctx, containerID, timeout)
}

// Logs implements engine.Engine.
func (e *Engine) Logs(ctx context.Context, containerID string, tail int) (string, error) {
	return e.cli.Logs(ctx, containerID, tail)
}

// Inspect implements engine.Inspector.
func (e *Engine) Inspect(ctx context.Context, containerID string) (engine.Status, error) {
	return e.cli.Inspect(ctx, containerID)
}

// Wait implements engine.Waiter.
func (e *Engine) Wait(ctx context.Context, containerID string) (int, error) {
	return e.cli.Wait(ctx, containerID)
}

// Remove implements engine.Remover.
func (e *Engine) Remove(ctx context.Context, containerID string, force bool) error {
	return e.cli.Remove(ctx, containerID, force)
}

// PullImage implements engine.ImagePuller.
func (e *Engine) PullImage(ctx context.Context, image string) error {
	return e.cli.PullImage(ctx, image)
}

// List implements engine.Lister.
func (e *Engine) List(ctx context.Context, labels map[string]string) ([]engine.Container, error) {
	return e.cli.List(ctx, labels)
}

// Version implements engine.Versioner.
func (e *Engine) Version(ctx context.Context) (engine.VersionInfo, error) {
	return e.cli.Version(ctx)
}
