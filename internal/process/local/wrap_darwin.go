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

//go:build darwin

package local

import (
	"fmt"

	"github.com/scrothers/pmmcp/internal/process"
	"github.com/scrothers/pmmcp/internal/sandbox"
	sandboxdarwin "github.com/scrothers/pmmcp/internal/sandbox/darwin"
)

// wrapSandbox applies macOS sandbox-exec seatbelt for strict/standard.
func wrapSandbox(cmd []string, projectRoot, profile string) ([]string, error) {
	pol, err := sandbox.DefaultPolicy(sandbox.Profile(profile), projectRoot)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", process.ErrSandboxFailed, err)
	}
	if wrapped, ok := sandboxdarwin.TrySandboxExec(cmd, projectRoot, pol); ok {
		return wrapped, nil
	}
	return nil, fmt.Errorf("%w: %s requires sandbox-exec for FS isolation", process.ErrSandboxFailed, profile)
}
