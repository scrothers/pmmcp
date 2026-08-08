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

package engine

import (
	"context"
	"errors"
	"time"
)

// Sentinel errors for container engines.
var (
	// ErrUnavailable is returned when the engine binary or daemon is not usable.
	ErrUnavailable = errors.New("engine: unavailable")
	// ErrInvalidSpec is returned when RunSpec fails validation.
	ErrInvalidSpec = errors.New("engine: invalid spec")
	// ErrNotFound is returned when a container ID is unknown.
	ErrNotFound = errors.New("engine: not found")
)

// Engine runs containers via a backend (Podman, Docker, fake, …).
//
// Parent package imports no concrete drivers.
type Engine interface {
	// Name returns the engine identifier (podman, docker, fake, …).
	Name() string
	// Available reports whether the engine can be used on this host.
	Available(ctx context.Context) bool
	// Run creates and starts a container, returning its ID.
	Run(ctx context.Context, spec RunSpec) (containerID string, err error)
	// Stop stops a running container, waiting up to timeout before force-kill.
	Stop(ctx context.Context, containerID string, timeout time.Duration) error
	// Logs returns recent container log output (tail lines; 0 means engine default).
	Logs(ctx context.Context, containerID string, tail int) (string, error)
}

// RunSpec describes a container to create and start.
type RunSpec struct {
	// Name is an optional human-readable container name.
	Name string
	// Image is the container image reference (required).
	Image string
	// Command is the argv override; empty means image default entrypoint/cmd.
	Command []string
	// Env is environment variables for the container.
	Env map[string]string
	// Ports are host:container port mappings (e.g. "5432:5432").
	// Unless PublishAllInterfaces is set, mappings without an explicit host IP
	// are bound to 127.0.0.1 (loopback-only) rather than all interfaces.
	Ports []string
	// Labels are container labels (e.g. io.pmmcp.proc_id) for reconcile.
	Labels map[string]string
	// User is the container user, e.g. "1000:1000"; empty uses the image default.
	User string
	// Network selects the container network mode; empty uses the engine default.
	// "host" host networking is never set by strict/standard callers.
	Network string
	// CapDrop lists Linux capabilities to drop (strict uses ["ALL"]).
	CapDrop []string
	// CapAdd lists Linux capabilities to add back after CapDrop.
	CapAdd []string
	// SecurityOpt lists --security-opt values (e.g. "no-new-privileges").
	SecurityOpt []string
	// ReadOnlyRootfs mounts the container root filesystem read-only when true.
	ReadOnlyRootfs bool
	// Privileged runs the container privileged; strict/standard keep this false.
	Privileged bool
	// Volumes are bind mounts applied to the container.
	Volumes []VolumeMount
	// PublishAllInterfaces, when true, publishes ports verbatim (all interfaces)
	// instead of defaulting host-IP-less mappings to loopback.
	PublishAllInterfaces bool
	// NoRemove, when true, keeps the container after exit (omits --rm) so exit
	// status and post-exit logs remain inspectable.
	NoRemove bool
}

// VolumeMount is a bind mount for a container.
type VolumeMount struct {
	// Source is the host path.
	Source string
	// Target is the in-container mount path.
	Target string
	// ReadOnly mounts the bind read-only when true.
	ReadOnly bool
}
