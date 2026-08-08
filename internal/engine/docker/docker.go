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

// Package docker implements engine.Engine using the Docker CLI against a Docker
// daemon.
package docker

import (
	"context"
	"time"

	"github.com/scrothers/pmmcp/internal/engine"
)

// Compile-time checks: docker satisfies the core Engine plus every optional
// capability interface.
var (
	_ engine.Engine      = (*Engine)(nil)
	_ engine.Inspector   = (*Engine)(nil)
	_ engine.Waiter      = (*Engine)(nil)
	_ engine.Remover     = (*Engine)(nil)
	_ engine.ImagePuller = (*Engine)(nil)
	_ engine.Lister      = (*Engine)(nil)
	_ engine.Versioner   = (*Engine)(nil)
)

// Engine runs containers via the docker CLI.
//
// Unlike daemonless Podman, Docker is a client/daemon architecture: the docker
// binary can be present while the daemon is down. Available therefore probes the
// daemon, not just the binary. Run/Stop/Logs/Inspect/Wait/Remove/PullImage/List
// shell out to the CLI; when docker is missing they return engine.ErrUnavailable
// and an unknown container id returns engine.ErrNotFound.
type Engine struct {
	cli engine.CLIRunner
}

// Option configures a docker Engine.
type Option func(*Engine)

// WithHost points the docker CLI at a specific daemon by exporting DOCKER_HOST
// for every command (e.g. "unix:///var/run/docker.sock" or "tcp://host:2375").
// An empty host is ignored so the CLI's own DOCKER_HOST/context resolution
// applies.
func WithHost(host string) Option {
	return func(e *Engine) {
		if host != "" {
			e.cli.Env = append(e.cli.Env, "DOCKER_HOST="+host)
		}
	}
}

// New returns a Docker-backed Engine.
func New(opts ...Option) *Engine {
	e := &Engine{cli: engine.CLIRunner{Binary: "docker"}}
	for _, o := range opts {
		o(e)
	}
	return e
}

// Name implements engine.Engine.
func (e *Engine) Name() string { return "docker" }

// Available reports whether docker is usable: the CLI is on PATH and the daemon
// answers a version query. A binary-only check would wrongly select docker when
// the daemon is stopped.
func (e *Engine) Available(ctx context.Context) bool {
	if !e.cli.Available(ctx) {
		return false
	}
	v, err := e.cli.Version(ctx)
	return err == nil && v.Server != ""
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
