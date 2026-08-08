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

import (
	"context"
	"errors"
	"os"
	"time"
)

// Sentinel errors for process management.
var (
	// ErrNotFound is returned when a process ID is unknown to the Manager.
	ErrNotFound = errors.New("process: not found")
	// ErrAlreadyExists is returned when Start is called with a duplicate ID.
	ErrAlreadyExists = errors.New("process: already exists")
	// ErrInvalidSpec is returned when StartSpec fails validation.
	ErrInvalidSpec = errors.New("process: invalid spec")
	// ErrNotRunning is returned when a signal/stop targets a non-running process.
	ErrNotRunning = errors.New("process: not running")
	// ErrSandboxFailed is returned when a restrictive sandbox cannot be applied (fail closed).
	ErrSandboxFailed = errors.New("process: sandbox failed")
)

// Manager controls process lifecycle for a backend (local OS, container, …).
//
// Implementations must not wrap commands in a shell; Command is argv only.
type Manager interface {
	// Start launches a process described by spec and returns its handle.
	Start(ctx context.Context, spec StartSpec) (*Handle, error)
	// Stop requests graceful termination, escalating after timeout (tree kill on local).
	Stop(ctx context.Context, id string, timeout time.Duration) error
	// Wait blocks until the process exits or ctx is canceled.
	Wait(ctx context.Context, id string) (*Handle, error)
	// Inspect returns the current handle snapshot without blocking on exit.
	Inspect(ctx context.Context, id string) (*Handle, error)
	// Signal delivers sig to the managed process (or process group where applicable).
	Signal(ctx context.Context, id string, sig os.Signal) error
}
