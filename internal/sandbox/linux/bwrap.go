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
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/scrothers/pmmcp/internal/sandbox"
)

// systemROPaths are the read-only system paths bound into a strict jail: enough
// to locate and execute binaries, resolve libraries, and read CA certs, but
// nothing under the user's home. Missing entries are skipped (--ro-bind-try).
var systemROPaths = []string{
	"/usr", "/bin", "/sbin", "/lib", "/lib64",
	"/etc/ssl", "/etc/pki", "/etc/ca-certificates",
	"/etc/resolv.conf", "/etc/hosts", "/etc/nsswitch.conf",
	"/etc/passwd", "/etc/group", "/etc/localtime",
	"/etc/ld.so.cache", "/etc/ld.so.conf", "/etc/ld.so.conf.d",
	"/etc/alternatives",
}

// engineSocketPaths are container-engine sockets masked in the standard jail so
// a sandboxed process cannot reach the host container engine (sandbox-strict
// "Container socket by default deny"). In the strict jail these paths are never
// bound in the first place, so they are absent regardless.
func engineSocketPaths() []string {
	socks := []string{"/var/run/docker.sock", "/run/docker.sock"}
	if xdg := os.Getenv("XDG_RUNTIME_DIR"); xdg != "" {
		socks = append(socks, filepath.Join(xdg, "podman", "podman.sock"))
	}
	return socks
}

// TryBwrap rewrites cmd into a bubblewrap argv that isolates the child when
// `bwrap` is on PATH. With no policy it uses the standard (denylist) jail.
// Returns ok=false when bwrap is missing or roots empty.
func TryBwrap(cmd []string, projectRoot string) (argv []string, ok bool) {
	return TryBwrapPolicy(cmd, projectRoot, nil)
}

// TryBwrapPolicy rewrites cmd into a bubblewrap argv sized to pol.Profile.
//
//   - strict: deny-by-default ALLOWLIST — only read-only system paths and the
//     project/temp RW roots are visible, so ~/.netrc, ~/.aws, ~/.config,
//     ~/.kube, other projects' files, and engine sockets are absent. The
//     network is unshared (fresh netns with loopback up), so external egress is
//     denied while loopback still works (sandbox-strict). PID/IPC/UTS are
//     unshared so host processes are not visible.
//   - standard (or nil policy): the host root is bound read-only with the
//     secret trees and engine sockets masked; the network stays shared for dev
//     servers needing outbound APIs.
//
// Returns ok=false when bwrap is missing or roots empty.
func TryBwrapPolicy(cmd []string, projectRoot string, pol *sandbox.Policy) (argv []string, ok bool) {
	if len(cmd) == 0 {
		return nil, false
	}
	bwrap, err := exec.LookPath("bwrap")
	if err != nil {
		return nil, false
	}
	projectRoot = filepath.Clean(projectRoot)
	if projectRoot == "" || projectRoot == "." {
		return nil, false
	}
	if st, err := os.Stat(projectRoot); err != nil || !st.IsDir() {
		return nil, false
	}
	temp := filepath.Clean(os.TempDir())

	strict := pol != nil && pol.Profile == sandbox.Strict
	if strict {
		argv = strictBwrapArgv(bwrap, projectRoot, temp)
	} else {
		argv = standardBwrapArgv(bwrap, projectRoot, temp, pol)
	}
	argv = append(argv, "--chdir", projectRoot, "--")
	argv = append(argv, cmd...)
	return argv, true
}

// strictBwrapArgv builds the deny-by-default allowlist jail.
func strictBwrapArgv(bwrap, projectRoot, temp string) []string {
	argv := []string{
		bwrap,
		"--die-with-parent",
		// Deny external egress (loopback stays up) and hide host processes/IPC.
		"--unshare-net",
		"--unshare-pid",
		"--unshare-ipc",
		"--unshare-uts",
		"--proc", "/proc",
		"--dev", "/dev",
	}
	// Read-only system paths (best-effort: skip any that do not exist).
	for _, p := range systemROPaths {
		if _, err := os.Lstat(p); err == nil {
			argv = append(argv, "--ro-bind-try", p, p)
		}
	}
	// A real but empty HOME so tools that stat $HOME work without exposing it.
	// Skipped when the project root is the home dir itself (the later project
	// bind would otherwise re-expose the full home over the tmpfs).
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		home = filepath.Clean(home)
		if home != "/" && home != projectRoot {
			argv = append(argv, "--tmpfs", home)
		}
	}
	// Scratch /tmp when the temp root is elsewhere.
	if temp != "/tmp" {
		argv = append(argv, "--tmpfs", "/tmp")
	}
	// Writable roots: project + temp (temp bind comes after the home tmpfs so a
	// temp under home is re-established; project bind is added last so a project
	// under home wins over the home tmpfs).
	if temp != "" && temp != projectRoot {
		argv = append(argv, "--bind", temp, temp)
	}
	argv = append(argv, "--bind", projectRoot, projectRoot)
	return argv
}

// standardBwrapArgv builds the denylist jail: host root read-only, secret trees
// and engine sockets masked, network shared.
func standardBwrapArgv(bwrap, projectRoot, temp string, pol *sandbox.Policy) []string {
	argv := []string{
		bwrap,
		"--die-with-parent",
		"--ro-bind", "/", "/",
		"--dev", "/dev",
		"--proc", "/proc",
		"--bind", projectRoot, projectRoot,
	}
	if temp != "" && temp != projectRoot {
		argv = append(argv, "--bind", temp, temp)
	}
	// Cover secret path classes so ro-bind of / cannot leak keys (e.g. ~/.ssh).
	for _, p := range denyMountPaths(pol) {
		if p == "" || p == projectRoot {
			continue
		}
		if isUnderPath(p, projectRoot) {
			continue
		}
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			argv = append(argv, "--tmpfs", p)
		} else if parent := filepath.Dir(p); parent != "" && parent != "/" {
			base := filepath.Base(p)
			if strings.HasPrefix(base, ".") || strings.Contains(p, ".ssh") {
				if st, err := os.Stat(parent); err == nil && st.IsDir() {
					argv = append(argv, "--tmpfs", parent)
				}
			}
		}
	}
	// Mask engine sockets by shadowing them with /dev/null (a read-only bind of
	// a socket still permits connect()).
	for _, s := range engineSocketPaths() {
		if isUnderPath(s, projectRoot) {
			continue
		}
		if st, err := os.Stat(s); err == nil && st.Mode()&os.ModeSocket != 0 {
			argv = append(argv, "--bind", os.DevNull, s)
		}
	}
	return argv
}

// BwrapAvailable reports whether bubblewrap is on PATH.
func BwrapAvailable() bool {
	_, err := exec.LookPath("bwrap")
	return err == nil
}

// IsolationAvailable reports whether Linux can enforce FS isolation for children.
func IsolationAvailable() bool {
	return BwrapAvailable() || LandlockAvailable()
}

func denyMountPaths(pol *sandbox.Policy) []string {
	seen := map[string]struct{}{}
	var out []string
	add := func(p string) {
		p = filepath.Clean(strings.TrimSpace(p))
		if p == "" || p == "." || p == "/" {
			return
		}
		// Only absolute host paths can be tmpfs-overlaid.
		if !filepath.IsAbs(p) {
			return
		}
		if _, ok := seen[p]; ok {
			return
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		add(filepath.Join(home, ".ssh"))
		add(filepath.Join(home, ".gnupg"))
		add(filepath.Join(home, ".aws"))
		add(filepath.Join(home, ".config", "gcloud"))
	}
	if pol != nil {
		for _, d := range pol.ReadDeny {
			if filepath.IsAbs(d) {
				add(d)
			}
		}
	}
	return out
}

func isUnderPath(path, root string) bool {
	path = filepath.Clean(path)
	root = filepath.Clean(root)
	if path == root {
		return true
	}
	sep := string(os.PathSeparator)
	return strings.HasPrefix(path, root+sep)
}
