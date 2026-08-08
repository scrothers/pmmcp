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

package linux

import (
	"context"
	"fmt"

	"github.com/scrothers/pmmcp/internal/sandbox"
)

// Applied is the result of applying a sandbox policy on Linux.
type Applied struct {
	// Profile is the requested profile that was accepted.
	Profile sandbox.Profile
	// Mode is "landlock", "bwrap", "policy", "off", or "container-substitute".
	// "landlock" means the kernel ABI is available and a ruleset for the policy
	// was successfully prepared (child isolation is completed via bwrap/local
	// when configured; the daemon process is not Landlock-restricted).
	Mode string
}

// Apply validates pol and records the effective enforcement mode.
//
// For restrictive profiles:
//  1. Validates the profile (unknown → error).
//  2. Strict/standard require a project root.
//  3. Reports Mode "bwrap" when bubblewrap is available (the mechanism that
//     actually confines the child in the local driver), else Mode "policy".
//
// Mode never reports "landlock": although a Landlock availability probe exists,
// no child is confined via LandlockRestrictSelf in this MVP (the local driver
// wraps children with bubblewrap), so reporting "landlock" would overstate the
// enforcement. Landlock is reserved for a future re-exec child helper.
func Apply(ctx context.Context, pol sandbox.Policy) (Applied, error) {
	if err := ctx.Err(); err != nil {
		return Applied{}, fmt.Errorf("sandbox/linux: apply: %w", err)
	}
	if !sandbox.Valid(pol.Profile) {
		return Applied{}, fmt.Errorf("sandbox/linux: apply: %w: %q", sandbox.ErrUnknownProfile, pol.Profile)
	}

	switch pol.Profile {
	case sandbox.Off:
		return Applied{Profile: sandbox.Off, Mode: sandbox.ModeOff}, nil

	case sandbox.Strict:
		if !pol.HasProjectRoot() {
			return Applied{}, fmt.Errorf("sandbox/linux: apply: %w", sandbox.ErrProjectRootRequired)
		}
		return Applied{Profile: sandbox.Strict, Mode: effectiveMode(pol)}, nil

	case sandbox.Standard:
		if !pol.HasProjectRoot() {
			return Applied{}, fmt.Errorf("sandbox/linux: apply: %w", sandbox.ErrProjectRootRequired)
		}
		return Applied{Profile: sandbox.Standard, Mode: effectiveMode(pol)}, nil

	case sandbox.Permissive:
		return Applied{Profile: sandbox.Permissive, Mode: sandbox.ModePolicy}, nil

	default:
		return Applied{}, fmt.Errorf("sandbox/linux: apply: %w: %q", sandbox.ErrUnknownProfile, pol.Profile)
	}
}

func effectiveMode(_ sandbox.Policy) string {
	if BwrapAvailable() {
		return sandbox.ModeBwrap
	}
	return sandbox.ModePolicy
}
