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

package darwin

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/scrothers/pmmcp/internal/sandbox"
)

func userHome() string {
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Clean(home)
	}
	return ""
}

// seatbeltSystemReadRoots are the read-only system paths a sandboxed process
// needs to locate and execute binaries, load frameworks, and resolve certs.
// Home is deliberately excluded so credential files there are not readable.
var seatbeltSystemReadRoots = []string{
	"/usr", "/bin", "/sbin", "/System", "/Library",
	"/private/etc", "/private/var/db/dyld", "/private/var/db/timezone",
	"/private/var/db/mds/system", // Spotlight/xpc metadata some toolchains stat
	"/dev/null", "/dev/random", "/dev/urandom", "/dev/zero", "/dev/dtracehelper",
}

// SeatbeltProfile builds a deny-by-default sandbox-exec (seatbelt) profile for
// pol. Unlike a permissive "(allow default)" policy, only the project/temp RW
// roots and a fixed set of read-only system paths are permitted, so home
// credential files (~/.ssh, ~/.netrc, ~/.aws, ~/.config, ~/.kube, ~/.docker,
// other projects' files) are unreadable (sandbox-strict "read-only limited
// system paths" allowlist).
//
// Network: strict denies egress but allows loopback; standard and looser
// profiles allow egress.
//
// NOTE: seatbelt semantics can only be validated on macOS; on other GOOS this
// builds the profile string but never enforces it. macOS CI must exercise the
// enforced behavior.
func SeatbeltProfile(projectRoot string, pol sandbox.Policy) string {
	projectRoot = filepath.Clean(projectRoot)

	var b strings.Builder
	b.WriteString("(version 1)\n")
	b.WriteString("(deny default)\n")
	// Silence the deny log so a sandboxed dev process doesn't spam the system log
	// with every denied probe (common with toolchains that stat speculatively).
	b.WriteString("(deny default (with no-log))\n")

	// Metadata reads (stat/lstat/access) are allowed filesystem-wide: a
	// deny-default profile that cannot stat "/" or the ancestors of the project
	// root cannot even resolve a path to the allowlisted subpaths, so most
	// programs fail to start. Metadata exposes existence/size/mode, never file
	// contents — content reads stay restricted to the allowlist below.
	b.WriteString("(allow file-read-metadata)\n")

	// Process/exec primitives needed for the child and its subprocesses.
	b.WriteString("(allow process-fork)\n")
	b.WriteString("(allow process-exec*)\n")
	b.WriteString("(allow process-info* (target self))\n")
	b.WriteString("(allow signal (target same-sandbox))\n")
	b.WriteString("(allow sysctl-read)\n")
	b.WriteString("(allow mach-lookup)\n")
	// POSIX shared memory + semaphores: needed by many runtimes (Node, Python
	// multiprocessing, databases) even when confined to their own project tree.
	b.WriteString("(allow ipc-posix-shm*)\n")
	b.WriteString("(allow ipc-posix-sem*)\n")

	// Read-only system paths (allowlist).
	b.WriteString("(allow file-read*\n")
	for _, p := range seatbeltSystemReadRoots {
		fmt.Fprintf(&b, "  (subpath %q)\n", p)
	}
	b.WriteString(")\n")

	// Standard extends read (but not write) to home minus the secret trees.
	if pol.Profile == sandbox.Standard {
		writeStandardHomeReads(&b, pol)
	}

	// Writable roots: project root and temp roots from the policy.
	fmt.Fprintf(&b, "(allow file-read* file-write* (subpath %q))\n", projectRoot)
	for _, root := range pol.WritableRoots {
		root = filepath.Clean(strings.TrimSpace(root))
		if root == "" || root == projectRoot {
			continue
		}
		fmt.Fprintf(&b, "(allow file-read* file-write* (subpath %q))\n", root)
	}
	// Standard error/output devices.
	b.WriteString("(allow file-write-data (literal \"/dev/null\") (literal \"/dev/stdout\") (literal \"/dev/stderr\"))\n")

	// Network dimension.
	if pol.Profile == sandbox.Strict {
		// Egress denied by default; loopback permitted.
		b.WriteString("(allow network-bind (local ip \"localhost:*\"))\n")
		b.WriteString("(allow network-outbound (remote ip \"localhost:*\") (remote unix-socket))\n")
	} else {
		b.WriteString("(allow network*)\n")
	}
	return b.String()
}

// writeStandardHomeReads allows reading the home tree for the standard profile
// while explicitly denying the secret subtrees (home-read-limited).
func writeStandardHomeReads(b *strings.Builder, pol sandbox.Policy) {
	home := userHome()
	if home == "" {
		return
	}
	fmt.Fprintf(b, "(allow file-read* (subpath %q))\n", home)
	for _, sub := range []string{".ssh", ".gnupg", ".aws", ".config/gcloud", ".netrc", ".kube", ".docker"} {
		fmt.Fprintf(b, "(deny file-read* file-write* (subpath %q))\n", filepath.Join(home, sub))
	}
	for _, d := range pol.ReadDeny {
		d = filepath.Clean(strings.TrimSpace(d))
		if d == "" || !filepath.IsAbs(d) {
			continue
		}
		fmt.Fprintf(b, "(deny file-read* file-write* (subpath %q))\n", d)
	}
}
