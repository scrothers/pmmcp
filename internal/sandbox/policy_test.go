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

package sandbox_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/scrothers/pmmcp/internal/sandbox"
)

func TestValidProfiles(t *testing.T) {
	t.Parallel()
	for _, p := range []sandbox.Profile{
		sandbox.Strict, sandbox.Standard, sandbox.Permissive, sandbox.Off,
	} {
		if !sandbox.Valid(p) {
			t.Fatalf("Valid(%q) = false", p)
		}
	}
	if sandbox.Valid("nope") {
		t.Fatal("unknown profile should be invalid")
	}
	if sandbox.Valid("") {
		t.Fatal("empty profile should be invalid")
	}
}

func TestDefaultPolicyUnknown(t *testing.T) {
	t.Parallel()
	_, err := sandbox.DefaultPolicy("bogus", "/proj")
	if !errors.Is(err, sandbox.ErrUnknownProfile) {
		t.Fatalf("err = %v, want ErrUnknownProfile", err)
	}
}

func TestDefaultPolicyStrict(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "proj")
	pol, err := sandbox.DefaultPolicy(sandbox.Strict, root)
	if err != nil {
		t.Fatal(err)
	}
	if pol.Profile != sandbox.Strict {
		t.Fatalf("profile = %q", pol.Profile)
	}
	if !pol.HasProjectRoot() {
		t.Fatal("expected project root")
	}
	if !pol.AllowsWrite(filepath.Join(root, "out.txt")) {
		t.Fatal("write under project should allow")
	}
	// Path outside project and host temp must deny.
	if pol.AllowsWrite("/var/empty-not-a-root/x") {
		t.Fatal("write outside roots should deny")
	}
	if !pol.AllowsRead(filepath.Join(root, "README.md")) {
		t.Fatal("read project file should allow")
	}
}

func TestDefaultPolicyOff(t *testing.T) {
	t.Parallel()
	pol, err := sandbox.DefaultPolicy(sandbox.Off, "")
	if err != nil {
		t.Fatal(err)
	}
	if !pol.AllowsRead("/etc/passwd") {
		t.Fatal("off should allow read")
	}
	if !pol.AllowsWrite("/tmp/whatever") {
		t.Fatal("off should allow write")
	}
	if pol.HasProjectRoot() {
		t.Fatal("off has no project root")
	}
}

func TestAllowsReadDeniesSSH(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	pol, err := sandbox.DefaultPolicy(sandbox.Strict, root)
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		path string
		want bool
	}{
		{"slash_ssh_id", "/home/dev/.ssh/id_rsa", false},
		{"slash_ssh_ed", "/home/dev/.ssh/id_ed25519", false},
		{"contains_ssh_seg", "/Users/a/.ssh/config", false},
		{"ssh_dir", "/home/dev/.ssh", false},
		{"ssh_dir_trailing", "/home/dev/.ssh/", false},
		{"project_file", filepath.Join(root, "main.go"), true},
		{"relative_ssh", ".ssh/id_rsa", false},
		{"not_ssh_suffix", filepath.Join(root, "myssh", "file"), true},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := pol.AllowsRead(tc.path)
			if got != tc.want {
				t.Fatalf("AllowsRead(%q) = %v, want %v", tc.path, got, tc.want)
			}
		})
	}
}

func TestAllowsReadStandardDeniesSSH(t *testing.T) {
	t.Parallel()
	pol, err := sandbox.DefaultPolicy(sandbox.Standard, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if pol.AllowsRead("/home/u/.ssh/id_rsa") {
		t.Fatal("standard should deny .ssh")
	}
}

func TestAllowsReadPermissiveAllowsSSH(t *testing.T) {
	t.Parallel()
	pol, err := sandbox.DefaultPolicy(sandbox.Permissive, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	// Permissive is home-scoped escape hatch; path policy does not block .ssh.
	if !pol.AllowsRead("/home/u/.ssh/id_rsa") {
		t.Fatal("permissive should allow .ssh read at policy layer")
	}
}

func TestAllowsWriteStrict(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	pol, err := sandbox.DefaultPolicy(sandbox.Strict, root)
	if err != nil {
		t.Fatal(err)
	}
	if !pol.AllowsWrite(filepath.Join(root, "a", "b")) {
		t.Fatal("project write denied")
	}
	if !pol.AllowsWrite(filepath.Join(os.TempDir(), "pmmcp-test-write")) {
		t.Fatal("temp write denied")
	}
	if pol.AllowsWrite("/home/dev/.ssh/authorized_keys") {
		t.Fatal("ssh write should deny")
	}
	if pol.AllowsWrite("/etc/shadow") {
		t.Fatal("etc write should deny")
	}
}

func TestAllowsReadWriteUnknownProfile(t *testing.T) {
	t.Parallel()
	pol := sandbox.Policy{Profile: "nope", WritableRoots: []string{"/"}}
	if pol.AllowsRead("/x") {
		t.Fatal("unknown should deny read")
	}
	if pol.AllowsWrite("/x") {
		t.Fatal("unknown should deny write")
	}
}

func TestHasProjectRootEmpty(t *testing.T) {
	t.Parallel()
	pol, err := sandbox.DefaultPolicy(sandbox.Strict, "")
	if err != nil {
		t.Fatal(err)
	}
	if pol.HasProjectRoot() {
		t.Fatal("empty projectRoot should not set project writable root")
	}
}

func TestDefaultPolicyPermissiveIncludesHome(t *testing.T) {
	t.Parallel()
	pol, err := sandbox.DefaultPolicy(sandbox.Permissive, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skip("no home dir")
	}
	if !pol.AllowsWrite(filepath.Join(home, "notes.txt")) {
		t.Fatalf("permissive should allow write under home %q", home)
	}
}

// TestDefaultPolicyPermissiveHomeAlreadyRoot exercises the containsPath
// dedup branch in DefaultPolicy's permissive case: when projectRoot is the
// home directory itself, home must not be appended a second time.
func TestDefaultPolicyPermissiveHomeAlreadyRoot(t *testing.T) {
	// Mutates $HOME; must not run in parallel with other HOME-dependent tests.
	home := t.TempDir()
	t.Setenv("HOME", home)

	pol, err := sandbox.DefaultPolicy(sandbox.Permissive, home)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, r := range pol.WritableRoots {
		if filepath.Clean(r) == filepath.Clean(home) {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("home appeared %d times in WritableRoots, want 1: %v", count, pol.WritableRoots)
	}
}

func TestAllowsReadWriteEmptyPath(t *testing.T) {
	t.Parallel()
	pol, err := sandbox.DefaultPolicy(sandbox.Strict, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"", "   "} {
		if pol.AllowsRead(p) {
			t.Errorf("AllowsRead(%q) = true, want false", p)
		}
		if pol.AllowsWrite(p) {
			t.Errorf("AllowsWrite(%q) = true, want false", p)
		}
	}
}

// TestAllowsReadGenericDenyMarker exercises the ReadDeny loop's true branch
// (and pathMatchesDeny's marker-style match) via a non-ssh default deny entry.
func TestAllowsReadGenericDenyMarker(t *testing.T) {
	t.Parallel()
	pol, err := sandbox.DefaultPolicy(sandbox.Standard, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if pol.AllowsRead("/home/dev/.aws/credentials") {
		t.Fatal("standard should deny .aws credentials")
	}
}

// TestPathMatchesDenyAbsolutePrefix exercises pathMatchesDeny's absolute
// prefix branch (no dot-marker, no .sock suffix) via AllowsRead, including
// the requirement that a sibling directory sharing only a name prefix must
// not match (e.g. ".../secretdir-backup" vs deny ".../secretdir").
func TestPathMatchesDenyAbsolutePrefix(t *testing.T) {
	t.Parallel()
	deny := filepath.Join(string(filepath.Separator), "home", "user", "secretdir")
	pol := sandbox.Policy{
		Profile:  sandbox.Standard,
		ReadDeny: []string{"", deny},
	}

	cases := []struct {
		name string
		path string
		want bool
	}{
		{"exact_match", deny, false},
		{"subpath_match", filepath.Join(deny, "file"), false},
		{"sibling_prefix_not_matched", deny + "-backup", true},
		{"sibling_subpath_not_matched", filepath.Join(deny+"-backup", "file"), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := pol.AllowsRead(tc.path)
			if got != tc.want {
				t.Fatalf("AllowsRead(%q) = %v, want %v", tc.path, got, tc.want)
			}
		})
	}
}

// TestAllowsWriteRootEdgeCases exercises AllowsWrite's own root=="" skip
// alongside isUnder's root=="" fast path (via a whitespace-only root that
// normalizes to empty) and isUnder's path==root exact match.
func TestAllowsWriteRootEdgeCases(t *testing.T) {
	t.Parallel()
	valid := t.TempDir()
	pol := sandbox.Policy{
		Profile:       sandbox.Standard,
		WritableRoots: []string{"", "   ", valid},
	}
	if !pol.AllowsWrite(filepath.Join(valid, "out.txt")) {
		t.Fatal("write under valid root should allow despite blank entries")
	}
	// path == root exact match (isUnder's fast path).
	if !pol.AllowsWrite(valid) {
		t.Fatal("write to the root itself should allow")
	}
}

// TestHasProjectRootSkipsBlankEntries exercises the r=="" continue branch.
func TestHasProjectRootSkipsBlankEntries(t *testing.T) {
	t.Parallel()
	pol := sandbox.Policy{WritableRoots: []string{"  ", filepath.Join(t.TempDir(), "proj")}}
	if !pol.HasProjectRoot() {
		t.Fatal("non-temp root after a blank entry should count as a project root")
	}
}
