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

//go:build linux

package linux

import (
	"fmt"
	"os"
	"path/filepath"
	"unsafe"

	"github.com/scrothers/pmmcp/internal/sandbox"
	"golang.org/x/sys/unix"
)

// landlockRulesetAttr is the kernel landlock_ruleset_attr layout (ABI ≤3 fields used).
type landlockRulesetAttr struct {
	HandledAccessFS  uint64
	HandledAccessNet uint64
	Scoped           uint64
}

// landlockPathBeneathAttr is landlock_path_beneath_attr.
type landlockPathBeneathAttr struct {
	AllowedAccess uint64
	ParentFd      int32
}

// accessFSRead is the read-oriented FS rights we grant under allowed trees.
const accessFSRead = unix.LANDLOCK_ACCESS_FS_EXECUTE |
	unix.LANDLOCK_ACCESS_FS_READ_FILE |
	unix.LANDLOCK_ACCESS_FS_READ_DIR

// accessFSWrite adds write/remove/make rights for writable roots.
const accessFSWrite = accessFSRead |
	unix.LANDLOCK_ACCESS_FS_WRITE_FILE |
	unix.LANDLOCK_ACCESS_FS_REMOVE_DIR |
	unix.LANDLOCK_ACCESS_FS_REMOVE_FILE |
	unix.LANDLOCK_ACCESS_FS_MAKE_CHAR |
	unix.LANDLOCK_ACCESS_FS_MAKE_DIR |
	unix.LANDLOCK_ACCESS_FS_MAKE_REG |
	unix.LANDLOCK_ACCESS_FS_MAKE_SOCK |
	unix.LANDLOCK_ACCESS_FS_MAKE_FIFO |
	unix.LANDLOCK_ACCESS_FS_MAKE_BLOCK |
	unix.LANDLOCK_ACCESS_FS_MAKE_SYM |
	unix.LANDLOCK_ACCESS_FS_TRUNCATE

// landlockSysRoots are the read-only system trees a Landlock-restricted thread
// keeps, so it can still execute binaries, resolve libraries, and read machine
// configuration. Entries that are absent on the host are skipped.
var landlockSysRoots = []string{"/usr", "/bin", "/lib", "/lib64", "/etc", "/proc", "/dev"}

// landlockABI returns the highest supported Landlock ABI, or 0 if unavailable.
func landlockABI() int {
	return landlockABIFlags(uintptr(unix.LANDLOCK_CREATE_RULESET_VERSION))
}

// landlockABIFlags issues the landlock_create_ruleset query selected by flags
// and reports the ABI version it returns, or 0 when the kernel rejects the
// query (no Landlock support, or a flag this kernel does not implement).
func landlockABIFlags(flags uintptr) int {
	r1, _, errno := unix.Syscall(
		uintptr(unix.SYS_LANDLOCK_CREATE_RULESET),
		0,
		0,
		flags,
	)
	if errno != 0 {
		return 0
	}
	return int(r1)
}

// LandlockAvailable reports whether the kernel supports Landlock.
func LandlockAvailable() bool {
	return landlockABI() > 0
}

// LandlockRestrictPaths builds a Landlock ruleset for pol and restricts the
// current thread (LandlockRestrictSelf). Intended for a child helper, not the
// long-lived daemon. WritableRoots get read+write; other paths are denied by
// the default-deny ruleset. Returns an error when Landlock is unavailable or
// ruleset setup fails.
//
// ReadDeny entries are not individually listed (Landlock is allowlist-based);
// callers keep path-policy checks as a second layer for those markers.
func LandlockRestrictPaths(pol sandbox.Policy) error {
	return landlockRestrictPaths(landlockABI(), landlockSysRoots, pol)
}

// landlockRestrictPaths is LandlockRestrictPaths with the kernel ABI and the
// read-only system roots supplied by the caller, so both can be varied without
// depending on the host kernel or filesystem layout.
func landlockRestrictPaths(abi int, sysRoots []string, pol sandbox.Policy) error {
	if abi <= 0 {
		return fmt.Errorf("sandbox/linux: landlock: not available")
	}

	rulesetFD, err := landlockCreateRuleset(abi)
	if err != nil {
		return err
	}
	defer func() { _ = unix.Close(rulesetFD) }()

	// Allow read on filesystem roots that managed processes typically need.
	// Strict: writable only under WritableRoots; still allow execute of system bins.
	for _, root := range sysRoots {
		if st, err := os.Stat(root); err != nil || !st.IsDir() {
			continue
		}
		if err := landlockAddPath(rulesetFD, root, accessFSRead); err != nil {
			// Best-effort: missing optional roots (e.g. /lib64) are fine.
			continue
		}
	}
	for _, root := range pol.WritableRoots {
		root = filepath.Clean(root)
		if root == "." {
			// filepath.Clean("") is "."; an empty root must never widen the
			// ruleset to the process working directory.
			continue
		}
		if err := landlockAddPath(rulesetFD, root, accessFSWrite); err != nil {
			return fmt.Errorf("sandbox/linux: landlock add writable %s: %w", root, err)
		}
	}

	// PR_SET_NO_NEW_PRIVS is required before landlock_restrict_self.
	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		return fmt.Errorf("sandbox/linux: landlock no_new_privs: %w", err)
	}
	_, _, errno := unix.Syscall(
		uintptr(unix.SYS_LANDLOCK_RESTRICT_SELF),
		uintptr(rulesetFD),
		0,
		0,
	)
	if errno != 0 {
		return fmt.Errorf("sandbox/linux: landlock restrict_self: %w", errno)
	}
	return nil
}

// landlockCreateRuleset creates a default-deny ruleset handling every
// filesystem access right the given ABI understands. The caller owns the
// returned descriptor and must close it.
func landlockCreateRuleset(abi int) (int, error) {
	// Handled access: all FS rights we know about up to truncate (ABI 3).
	handled := uint64(accessFSWrite)
	if abi >= 2 {
		handled |= unix.LANDLOCK_ACCESS_FS_REFER
	}
	if abi >= 3 {
		handled |= unix.LANDLOCK_ACCESS_FS_TRUNCATE
	}

	attr := landlockRulesetAttr{HandledAccessFS: handled}
	size := unsafe.Sizeof(attr)
	if abi < 4 {
		// Older ABIs only had HandledAccessFS.
		size = unsafe.Sizeof(attr.HandledAccessFS)
	}
	fd, _, errno := unix.Syscall(
		uintptr(unix.SYS_LANDLOCK_CREATE_RULESET),
		uintptr(unsafe.Pointer(&attr)),
		size,
		0,
	)
	if errno != 0 {
		return -1, fmt.Errorf("sandbox/linux: landlock create_ruleset: %w", errno)
	}
	return int(fd), nil
}

func landlockAddPath(rulesetFD int, path string, access uint64) error {
	pfd, err := unix.Open(path, unix.O_PATH|unix.O_CLOEXEC, 0)
	if err != nil {
		return err
	}
	defer func() { _ = unix.Close(pfd) }()

	rule := landlockPathBeneathAttr{
		AllowedAccess: access,
		ParentFd:      int32(pfd),
	}
	_, _, errno := unix.Syscall6(
		uintptr(unix.SYS_LANDLOCK_ADD_RULE),
		uintptr(rulesetFD),
		uintptr(unix.LANDLOCK_RULE_PATH_BENEATH),
		uintptr(unsafe.Pointer(&rule)),
		0,
		0,
		0,
	)
	if errno != 0 {
		return errno
	}
	return nil
}
