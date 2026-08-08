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

//go:build windows

package local

import (
	"fmt"
	"os/exec"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// lowIntegritySID is the well-known Low mandatory integrity level SID
// (S-1-16-4096). A process running at Low integrity cannot write to objects
// labeled Medium or above — which is almost the entire user profile, Program
// Files, and the registry — so it is meaningfully write-confined.
const lowIntegritySID = "S-1-16-4096"

// applySandboxToken launches the child under a low-integrity primary token for
// the standard profile, giving write confinement on top of the Job Object.
//
// It is deliberately NOT used for strict: Low integrity restricts writes, not
// reads (reads are not integrity-gated on Windows), so a low-integrity process
// could still read ~/.ssh-equivalent files. Strict therefore continues to fail
// closed in wrapSandbox (a container runtime is the escape hatch); only standard
// — which is best-effort by contract — opts into this hardening.
//
// Best-effort: if the token cannot be built, the child still launches under the
// Job Object (the pre-existing standard behavior), so this never regresses a
// host that can't lower integrity. The returned cleanup closes the duplicated
// token after cmd.Start has consumed it; it is always safe to call.
//
// NOTE: this path only runs on Windows and cannot be exercised on the Linux/CI
// hosts used for unit tests — a Windows CI runner must validate the enforced
// behavior (that a standard child cannot write outside low-integrity locations).
func applySandboxToken(cmd *exec.Cmd, profile string) func() {
	if !strings.EqualFold(strings.TrimSpace(profile), "standard") {
		return func() {}
	}
	tok, err := lowIntegrityToken()
	if err != nil {
		// Fall back to Job-Object-only isolation (standard is best-effort).
		return func() {}
	}
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Token = syscall.Token(tok)
	return func() { _ = tok.Close() }
}

// lowIntegrityToken duplicates the current process token as a primary token and
// lowers its integrity level to Low. The caller owns the returned token and must
// Close it once CreateProcess has consumed it.
func lowIntegrityToken() (windows.Token, error) {
	var procTok windows.Token
	access := uint32(windows.TOKEN_DUPLICATE | windows.TOKEN_QUERY | windows.TOKEN_ADJUST_DEFAULT | windows.TOKEN_ASSIGN_PRIMARY)
	if err := windows.OpenProcessToken(windows.CurrentProcess(), access, &procTok); err != nil {
		return 0, fmt.Errorf("local: open process token: %w", err)
	}
	defer procTok.Close()

	var dup windows.Token
	if err := windows.DuplicateTokenEx(procTok, access, nil, windows.SecurityImpersonation, windows.TokenPrimary, &dup); err != nil {
		return 0, fmt.Errorf("local: duplicate token: %w", err)
	}

	sid, err := windows.StringToSid(lowIntegritySID)
	if err != nil {
		_ = dup.Close()
		return 0, fmt.Errorf("local: low-integrity sid: %w", err)
	}
	tml := windows.Tokenmandatorylabel{}
	tml.Label.Attributes = windows.SE_GROUP_INTEGRITY
	tml.Label.Sid = sid
	if err := windows.SetTokenInformation(dup, windows.TokenIntegrityLevel, (*byte)(unsafe.Pointer(&tml)), tml.Size()); err != nil {
		_ = dup.Close()
		return 0, fmt.Errorf("local: set integrity level: %w", err)
	}
	return dup, nil
}
