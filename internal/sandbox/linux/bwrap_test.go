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

package linux_test

import (
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scrothers/pmmcp/internal/sandbox"
	"github.com/scrothers/pmmcp/internal/sandbox/linux"
)

const secretMarker = "SECRET_MARKER_DO_NOT_LEAK"

func containsArg(argv []string, want ...string) bool {
	for i := 0; i+len(want) <= len(argv); i++ {
		ok := true
		for j := range want {
			if argv[i+j] != want[j] {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}

// TestStrictBwrapIsAllowlist proves the strict jail is deny-by-default: secret
// files beyond the hardcoded .ssh/.gnupg/.aws set (~/.netrc, ~/.config/*, and
// another project's .env) are NOT readable, while the project root is, and the
// network is unshared (egress denied).
func TestStrictBwrapIsAllowlist(t *testing.T) {
	if !linux.BwrapAvailable() {
		if os.Getenv("PMMCP_REQUIRE_SANDBOX") != "" {
			t.Fatal("bwrap required (PMMCP_REQUIRE_SANDBOX set)")
		}
		t.Skip("bwrap required")
	}
	// Lay out home, tmp, project, and another project as siblings under a base
	// that is itself NOT a writable root, so the strict temp bind (base/tmp)
	// cannot re-expose home or the other project. This mirrors production, where
	// HOME (/home/user) is not under the temp root (/tmp).
	base := t.TempDir()
	home := filepath.Join(base, "home")
	tmp := filepath.Join(base, "tmp")
	project := filepath.Join(base, "project")
	other := filepath.Join(base, "other")
	for _, d := range []string{home, tmp, project, other} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("HOME", home)
	t.Setenv("TMPDIR", tmp)

	for _, rel := range []string{".netrc", ".aws/credentials", ".config/anthropic/config", ".kube/config", ".docker/config.json"} {
		p := filepath.Join(home, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(secretMarker+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(other, ".env"), []byte(secretMarker+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "app.txt"), []byte("hello\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	pol, err := sandbox.DefaultPolicy(sandbox.Strict, project)
	if err != nil {
		t.Fatal(err)
	}
	script := "for f in ~/.netrc ~/.aws/credentials ~/.config/anthropic/config ~/.kube/config ~/.docker/config.json " +
		other + "/.env; do cat \"$f\" 2>/dev/null; done; printf 'PROJECT='; cat " + filepath.Join(project, "app.txt")
	argv, ok := linux.TryBwrapPolicy([]string{"/bin/sh", "-c", script}, project, &pol)
	if !ok {
		t.Fatal("TryBwrapPolicy(strict) returned ok=false")
	}
	if !containsArg(argv, "--unshare-net") {
		t.Errorf("strict argv missing --unshare-net (egress deny): %v", argv)
	}
	if containsArg(argv, "--ro-bind", "/", "/") {
		t.Errorf("strict must not read-bind the whole host root: %v", argv)
	}

	out, _ := exec.Command(argv[0], argv[1:]...).CombinedOutput()
	got := string(out)
	if strings.Contains(got, secretMarker) {
		t.Fatalf("strict jail leaked a secret; output=%q", got)
	}
	if !strings.Contains(got, "PROJECT=hello") {
		t.Fatalf("strict jail could not read project file; output=%q", got)
	}
}

// TestStandardBwrapBindsRootReadOnly documents that standard keeps the denylist
// model (host root read-only, network shared) so dev servers keep working.
func TestStandardBwrapBindsRootReadOnly(t *testing.T) {
	if !linux.BwrapAvailable() {
		t.Skip("bwrap required")
	}
	project := t.TempDir()
	pol, err := sandbox.DefaultPolicy(sandbox.Standard, project)
	if err != nil {
		t.Fatal(err)
	}
	argv, ok := linux.TryBwrapPolicy([]string{"/bin/true"}, project, &pol)
	if !ok {
		t.Fatal("ok=false")
	}
	if !containsArg(argv, "--ro-bind", "/", "/") {
		t.Errorf("standard should ro-bind host root: %v", argv)
	}
	if containsArg(argv, "--unshare-net") {
		t.Errorf("standard should keep network shared: %v", argv)
	}
}

// TestTryBwrapPolicyRejects covers every early bail-out of TryBwrapPolicy that
// does not depend on bwrap being missing.
func TestTryBwrapPolicyRejects(t *testing.T) {
	t.Parallel()
	if !linux.BwrapAvailable() {
		t.Skip("bwrap required")
	}
	dir := t.TempDir()
	file := filepath.Join(dir, "not-a-dir")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		cmd  []string
		root string
	}{
		{name: "empty cmd", cmd: nil, root: dir},
		{name: "empty root", cmd: []string{"/bin/true"}, root: ""},
		{name: "dot root", cmd: []string{"/bin/true"}, root: "."},
		{name: "relative dot root", cmd: []string{"/bin/true"}, root: "./"},
		{name: "missing root", cmd: []string{"/bin/true"}, root: filepath.Join(dir, "nope")},
		{name: "file root", cmd: []string{"/bin/true"}, root: file},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if argv, ok := linux.TryBwrapPolicy(tt.cmd, tt.root, nil); ok {
				t.Errorf("ok = true, want false (argv=%v)", argv)
			}
		})
	}
}

// TestTryBwrapPolicyWithoutBwrap covers the LookPath failure branch. It mutates
// PATH process-wide, so it must not call t.Parallel.
func TestTryBwrapPolicyWithoutBwrap(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if linux.BwrapAvailable() {
		t.Fatal("bwrap still resolvable after emptying PATH")
	}
	if argv, ok := linux.TryBwrapPolicy([]string{"/bin/true"}, t.TempDir(), nil); ok {
		t.Errorf("ok = true without bwrap on PATH (argv=%v)", argv)
	}
}

// TestStandardBwrapArgvMasksSecretParents covers the non-directory arm of the
// standard jail's secret masking: a secret path that is a plain file (or is
// absent) has its dotfile parent overlaid instead, and a live container-engine
// socket is shadowed with /dev/null rather than bound through.
//
// It rewrites HOME and XDG_RUNTIME_DIR, so it must not call t.Parallel.
func TestStandardBwrapArgvMasksSecretParents(t *testing.T) {
	if !linux.BwrapAvailable() {
		t.Skip("bwrap required")
	}
	base := t.TempDir()
	home := filepath.Join(base, "home")
	xdg := filepath.Join(base, "xdg")
	project := filepath.Join(base, "project")
	for _, d := range []string{home, xdg, project, filepath.Join(home, ".aws"), filepath.Join(home, ".config")} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	// ~/.ssh as a regular file: not a directory, so the parent gets the tmpfs.
	if err := os.WriteFile(filepath.Join(home, ".ssh"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("XDG_RUNTIME_DIR", xdg)

	sock := filepath.Join(xdg, "podman", "podman.sock")
	if err := os.MkdirAll(filepath.Dir(sock), 0o700); err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen on %s: %v", sock, err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	pol := sandbox.Policy{
		Profile:  sandbox.Standard,
		ReadDeny: []string{filepath.Join(base, "extra-secrets"), "relative/ignored"},
	}
	argv, ok := linux.TryBwrapPolicy([]string{"/bin/true"}, project, &pol)
	if !ok {
		t.Fatal("ok = false")
	}
	if !containsArg(argv, "--tmpfs", filepath.Join(home, ".aws")) {
		t.Errorf("missing tmpfs over ~/.aws: %v", argv)
	}
	if !containsArg(argv, "--tmpfs", home) {
		t.Errorf("missing tmpfs over the parent of the ~/.ssh file: %v", argv)
	}
	if !containsArg(argv, "--bind", os.DevNull, sock) {
		t.Errorf("engine socket not shadowed with %s: %v", os.DevNull, argv)
	}
	if containsArg(argv, "--tmpfs", filepath.Join(home, ".config")) {
		t.Errorf("~/.config must not be masked wholesale (only .config/gcloud): %v", argv)
	}
}

// TestStandardBwrapArgvKeepsProjectVisible covers the skip arms: deny paths that
// are the project root, or live under it, must not be overlaid (that would hide
// the project), and neither must an engine socket inside the project root.
//
// It rewrites HOME and XDG_RUNTIME_DIR, so it must not call t.Parallel.
func TestStandardBwrapArgvKeepsProjectVisible(t *testing.T) {
	if !linux.BwrapAvailable() {
		t.Skip("bwrap required")
	}
	base := t.TempDir()
	// The project root is HOME itself, so every home secret path is under it.
	project := filepath.Join(base, "home")
	xdg := filepath.Join(project, "run")
	for _, d := range []string{project, xdg, filepath.Join(project, ".ssh")} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("HOME", project)
	t.Setenv("XDG_RUNTIME_DIR", xdg)

	sock := filepath.Join(xdg, "podman", "podman.sock")
	if err := os.MkdirAll(filepath.Dir(sock), 0o700); err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen on %s: %v", sock, err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	pol := sandbox.Policy{
		Profile:  sandbox.Standard,
		ReadDeny: []string{project},
	}
	argv, ok := linux.TryBwrapPolicy([]string{"/bin/true"}, project, &pol)
	if !ok {
		t.Fatal("ok = false")
	}
	if containsArg(argv, "--tmpfs", project) {
		t.Errorf("project root must not be overlaid: %v", argv)
	}
	if containsArg(argv, "--tmpfs", filepath.Join(project, ".ssh")) {
		t.Errorf("deny path under the project root must not be overlaid: %v", argv)
	}
	if containsArg(argv, "--bind", os.DevNull, sock) {
		t.Errorf("socket under the project root must not be shadowed: %v", argv)
	}
}

// TestDenyMountPaths pins the normalization rules of the tmpfs overlay list.
// It rewrites HOME, so it must not call t.Parallel.
func TestDenyMountPaths(t *testing.T) {
	home := t.TempDir()

	tests := []struct {
		name    string
		home    string
		pol     *sandbox.Policy
		want    []string
		absent  []string
		wantLen int
	}{
		{
			name:    "home only, nil policy",
			home:    home,
			pol:     nil,
			want:    []string{filepath.Join(home, ".ssh"), filepath.Join(home, ".gnupg"), filepath.Join(home, ".aws"), filepath.Join(home, ".config", "gcloud")},
			wantLen: 4,
		},
		{
			name:    "relative home yields no absolute overlay",
			home:    "relative-home",
			pol:     nil,
			wantLen: 0,
		},
		{
			name: "root, duplicate and relative denies are dropped",
			home: "relative-home",
			pol: &sandbox.Policy{ReadDeny: []string{
				"/",
				"/opt/secrets",
				"/opt/secrets/",
				"/opt/secrets",
				"relative/deny",
				"",
			}},
			want:    []string{"/opt/secrets"},
			absent:  []string{"/", "relative/deny"},
			wantLen: 1,
		},
		{
			name:    "home under policy is deduplicated",
			home:    home,
			pol:     &sandbox.Policy{ReadDeny: []string{filepath.Join(home, ".ssh"), filepath.Join(home, ".aws")}},
			want:    []string{filepath.Join(home, ".ssh")},
			wantLen: 4,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HOME", tt.home)
			got := linux.DenyMountPaths(tt.pol)
			if len(got) != tt.wantLen {
				t.Fatalf("len = %d (%v), want %d", len(got), got, tt.wantLen)
			}
			for _, w := range tt.want {
				if !containsArg(got, w) {
					t.Errorf("missing %q in %v", w, got)
				}
			}
			for _, a := range tt.absent {
				if containsArg(got, a) {
					t.Errorf("unexpected %q in %v", a, got)
				}
			}
		})
	}
}

func TestIsUnderPath(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		path string
		root string
		want bool
	}{
		{name: "exact match", path: "/foo", root: "/foo", want: true},
		{name: "trailing slash on path", path: "/foo/", root: "/foo", want: true},
		{name: "trailing slash on root", path: "/foo", root: "/foo/", want: true},
		{name: "descendant", path: "/foo/bar/baz", root: "/foo", want: true},
		{name: "unclean descendant", path: "/foo/./bar", root: "/foo", want: true},
		{name: "sibling prefix is not containment", path: "/foobar", root: "/foo", want: false},
		{name: "parent is not under child", path: "/foo", root: "/foo/bar", want: false},
		{name: "relative path vs absolute root", path: "foo/bar", root: "/foo", want: false},
		// The filesystem root is deliberately not treated as a container: with a
		// project root of "/", secret deny paths must still be masked in the
		// standard jail rather than skipped as "inside the project".
		{name: "filesystem root is not a container", path: "/foo", root: "/", want: false},
		{name: "filesystem root matches itself", path: "/", root: "/", want: true},
		{name: "disjoint", path: "/srv/a", root: "/opt/b", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := linux.IsUnderPath(tt.path, tt.root); got != tt.want {
				t.Errorf("IsUnderPath(%q, %q) = %v, want %v", tt.path, tt.root, got, tt.want)
			}
		})
	}
}
