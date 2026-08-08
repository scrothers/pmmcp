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

package windows

import (
	"context"
	"fmt"

	"github.com/scrothers/pmmcp/internal/sandbox"
)

// Applied is the result of applying a sandbox policy on Windows.
type Applied struct {
	// Profile is the requested profile that was accepted.
	Profile sandbox.Profile
	// Mode is "policy", "off", or "container-substitute".
	Mode string
}

// Apply validates pol and records the effective enforcement mode.
//
// Windows has no filesystem-isolation mechanism in this MVP: Job Objects give
// tree-kill and resource limits, not FS confinement. Per, strict
// therefore FAILS CLOSED (ErrStrictUnsupported) rather than reporting an
// enforcement it does not provide — a container runtime is the escape hatch.
// Standard runs best-effort under a Job Object (Mode "job-object"). macOS/Linux
// provide real FS isolation; this behavior is Windows-specific and needs a
// Windows CI runner to validate the spawn path.
func Apply(ctx context.Context, pol sandbox.Policy) (Applied, error) {
	if err := ctx.Err(); err != nil {
		return Applied{}, fmt.Errorf("sandbox/windows: apply: %w", err)
	}
	if !sandbox.Valid(pol.Profile) {
		return Applied{}, fmt.Errorf("sandbox/windows: apply: %w: %q", sandbox.ErrUnknownProfile, pol.Profile)
	}

	switch pol.Profile {
	case sandbox.Off:
		return Applied{Profile: sandbox.Off, Mode: sandbox.ModeOff}, nil

	case sandbox.Strict:
		if !pol.HasProjectRoot() {
			return Applied{}, fmt.Errorf("sandbox/windows: apply: %w", sandbox.ErrProjectRootRequired)
		}
		// No FS isolation available: refuse rather than run unconfined.
		return Applied{}, fmt.Errorf("sandbox/windows: apply: %w", sandbox.ErrStrictUnsupported)

	case sandbox.Standard:
		if !pol.HasProjectRoot() {
			return Applied{}, fmt.Errorf("sandbox/windows: apply: %w", sandbox.ErrProjectRootRequired)
		}
		return Applied{Profile: sandbox.Standard, Mode: "job-object"}, nil

	case sandbox.Permissive:
		return Applied{Profile: sandbox.Permissive, Mode: sandbox.ModePolicy}, nil

	default:
		return Applied{}, fmt.Errorf("sandbox/windows: apply: %w: %q", sandbox.ErrUnknownProfile, pol.Profile)
	}
}
