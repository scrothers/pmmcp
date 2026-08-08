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

//go:build !darwin

package darwin

import "github.com/scrothers/pmmcp/internal/sandbox"

// TrySandboxExec is unavailable off macOS.
func TrySandboxExec(_ []string, _ string, _ sandbox.Policy) (argv []string, ok bool) {
	return nil, false
}

// SandboxExecAvailable is always false off macOS.
func SandboxExecAvailable() bool { return false }

// IsolationAvailable reports whether macOS can enforce FS isolation for
// children. Off macOS it is always false in production.
//
// It is a var (not a plain func) purely as a test seam: non-darwin unit
// tests override it to exercise Apply's "seatbelt" mode branch without a
// real sandbox-exec binary, then restore it via t.Cleanup. Real macOS builds
// use the darwin-tagged implementation in seatbelt.go instead, which this
// seam never touches.
var IsolationAvailable = func() bool { return false }
