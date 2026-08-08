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

//go:build !windows

package local

import "os/exec"

// applySandboxToken is a no-op off Windows: FS isolation for restrictive
// profiles is provided by bubblewrap (Linux) and sandbox-exec (macOS), applied
// in wrapSandbox before the child is spawned. The returned cleanup is always
// safe to call.
func applySandboxToken(_ *exec.Cmd, _ string) func() { return func() {} }
