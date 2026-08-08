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

package process

import "github.com/scrothers/pmmcp/internal/domain"

// StartSpec describes a process to start under a Manager.
type StartSpec struct {
	// ID is the process identity (proc-prefixed ULID).
	ID string
	// Name is a human-readable label (optional at this layer).
	Name string
	// Command is the argv list; required and non-empty (no shell wrapping).
	// For container runtime, may be empty to use the image default entrypoint.
	Command []string
	// Cwd is the working directory; empty means inherit.
	Cwd string
	// Env is KEY=VAL overlays applied over the child's base environment.
	Env []string
	// InheritEnv selects how much of the daemon environment the child inherits
	// before Env overlays: "minimal" (default; PATH/HOME/LANG/TMPDIR only, so
	// ambient secrets are not leaked), "full" (the daemon's whole environment),
	// or "none" (only Env). Empty means "minimal".
	InheritEnv string
	// LogDir, when set, is where stdout.log and stderr.log are written.
	LogDir string
	// Sandbox is an optional sandbox profile name (applied by higher layers).
	Sandbox string
	// Runtime selects backend: "local" (default), "container", "container:podman", "container:docker".
	Runtime string
	// Image is the container image (required for the container Manager).
	Image string
	// Ports are host:container mappings for the container Manager.
	Ports []string
	// MemoryBytes is a best-effort soft memory limit (rlimit on Unix); 0 = none.
	MemoryBytes uint64
}

// Handle is a point-in-time snapshot of a managed process.
type Handle struct {
	ID string
	// PID is the OS process id for local runtime; 0 for container-backed handles.
	PID int
	// ContainerID is set when the process is backed by a container engine.
	ContainerID string
	Status      domain.Status
	ExitCode    *int
}
