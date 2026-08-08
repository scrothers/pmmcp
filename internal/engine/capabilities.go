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

package engine

import (
	"context"
	"time"
)

// The interfaces below are OPTIONAL engine capabilities. The core Engine
// interface stays minimal (Name/Available/Run/Stop/Logs); richer backends
// (docker, podman via CLIRunner, fake) additionally implement these, and
// consumers discover them with a type assertion:
//
//	if insp, ok := eng.(engine.Inspector); ok { st, _ := insp.Inspect(ctx, id) }
//
// This keeps a backend that only knows the basics valid, while letting the
// container driver report real state, reap exited containers, and reconcile by
// label when the backend supports it.

// Inspector reports a container's current state (docker/podman `inspect`).
type Inspector interface {
	Inspect(ctx context.Context, containerID string) (Status, error)
}

// Waiter blocks until a container exits and returns its exit code
// (docker/podman `wait`).
type Waiter interface {
	Wait(ctx context.Context, containerID string) (exitCode int, err error)
}

// Remover deletes a container (docker/podman `rm`); force removes a running one.
type Remover interface {
	Remove(ctx context.Context, containerID string, force bool) error
}

// ImagePuller pre-pulls an image (docker/podman `pull`).
type ImagePuller interface {
	PullImage(ctx context.Context, image string) error
}

// Lister returns containers matching every label in labels (docker/podman
// `ps --filter label=k=v`). An empty map lists all pmmcp-managed containers the
// caller filters itself.
type Lister interface {
	List(ctx context.Context, labels map[string]string) ([]Container, error)
}

// Versioner returns client/server version info (docker/podman `version`). For a
// client/daemon engine (docker) a populated Server confirms the daemon is up.
type Versioner interface {
	Version(ctx context.Context) (VersionInfo, error)
}

// State is a container lifecycle state, mirroring the docker/podman
// `State.Status` field.
type State string

// Container states. StateUnknown is used when the backend reports a value this
// package does not model.
const (
	StateCreated    State = "created"
	StateRunning    State = "running"
	StatePaused     State = "paused"
	StateRestarting State = "restarting"
	StateRemoving   State = "removing"
	StateExited     State = "exited"
	StateDead       State = "dead"
	StateUnknown    State = "unknown"
)

// Status is the inspected state of a single container.
type Status struct {
	// ID is the full container id.
	ID string
	// Name is the container name (without the docker leading slash).
	Name string
	// Image is the image reference the container was created from.
	Image string
	// State is the lifecycle state.
	State State
	// Running is true while the container's main process is alive.
	Running bool
	// ExitCode is the main process exit code once the container has exited.
	ExitCode int
	// OOMKilled is true when the container was killed for exceeding memory.
	OOMKilled bool
	// Health is the healthcheck status ("", "starting", "healthy",
	// "unhealthy") when the image defines one.
	Health string
	// StartedAt / FinishedAt are the run boundaries (zero if not set).
	StartedAt  time.Time
	FinishedAt time.Time
	// Labels are the container labels (includes the io.pmmcp.* reconcile keys).
	Labels map[string]string
}

// Container is a summary row returned by Lister.List.
type Container struct {
	ID     string
	Name   string
	Image  string
	State  State
	Labels map[string]string
}

// VersionInfo is the client and (when reachable) server version of an engine.
type VersionInfo struct {
	Client string
	Server string
}
