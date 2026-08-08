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

//go:build windows

package local

import (
	"fmt"
	"strings"

	"github.com/scrothers/pmmcp/internal/process"
)

// wrapSandbox enforces the fail-closed rule on Windows. There is no filesystem
// isolation mechanism in this MVP (Job Objects provide tree-kill and resource
// limits, not FS confinement), so strict refuses to start rather than running
// with secrets readable; a container runtime is the escape hatch.
// Standard proceeds best-effort — the Job Object is still assigned after spawn.
func wrapSandbox(cmd []string, projectRoot, profile string) ([]string, error) {
	_ = projectRoot
	if strings.EqualFold(strings.TrimSpace(profile), "strict") {
		return nil, fmt.Errorf("%w: strict has no FS isolation on Windows; use a container runtime or a looser profile", process.ErrSandboxFailed)
	}
	return cmd, nil
}
