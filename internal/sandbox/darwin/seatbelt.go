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

package darwin

import (
	"os"
	"os/exec"
	"path/filepath"

	"github.com/scrothers/pmmcp/internal/sandbox"
)

// TrySandboxExec rewrites cmd into sandbox-exec argv that enforces the
// deny-by-default profile from SeatbeltProfile under strict/standard. The
// generated profile is written to a mode-0600 temp file and passed with -f, so
// the full policy is not exposed in the process list (unlike inline -p).
//
// The temp file (argv[2]) holds only path policy, not secrets; sandbox-exec
// reads it at child startup. It is left in TMPDIR for the OS temp reaper — a
// future refinement should remove it once the child has started (needs a start
// hook the current wrap signature does not expose). macOS CI must validate the
// enforced behavior; this path never runs on Linux.
// Returns ok=false when sandbox-exec is missing.
func TrySandboxExec(cmd []string, projectRoot string, pol sandbox.Policy) (argv []string, ok bool) {
	if len(cmd) == 0 {
		return nil, false
	}
	bin, err := exec.LookPath("sandbox-exec")
	if err != nil {
		return nil, false
	}
	projectRoot = filepath.Clean(projectRoot)
	if projectRoot == "" || projectRoot == "." {
		return nil, false
	}
	profile := SeatbeltProfile(projectRoot, pol)
	f, err := os.CreateTemp("", "pmmcp-seatbelt-*.sb")
	if err != nil {
		return nil, false
	}
	name := f.Name()
	if err := f.Chmod(0o600); err != nil {
		_ = f.Close()
		_ = os.Remove(name)
		return nil, false
	}
	if _, err := f.WriteString(profile); err != nil {
		_ = f.Close()
		_ = os.Remove(name)
		return nil, false
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(name)
		return nil, false
	}
	argv = append([]string{bin, "-f", name}, cmd...)
	return argv, true
}

// SandboxExecAvailable reports whether sandbox-exec is on PATH.
func SandboxExecAvailable() bool {
	_, err := exec.LookPath("sandbox-exec")
	return err == nil
}

// IsolationAvailable reports whether macOS can enforce FS isolation for
// children. It is a var (not a plain func) so it can be overridden as a test
// seam, matching the non-darwin stub in seatbelt_stub.go — without this, Apply's
// mode-selection branch could not be exercised on macOS itself (the assignment
// would not compile), so a macOS CI runner could not test the seatbelt path.
// Restore the original via t.Cleanup after overriding.
var IsolationAvailable = func() bool { return SandboxExecAvailable() }
