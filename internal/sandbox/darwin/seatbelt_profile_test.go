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

package darwin_test

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/scrothers/pmmcp/internal/sandbox"
	"github.com/scrothers/pmmcp/internal/sandbox/darwin"
)

func TestSeatbeltProfileStrictIsDenyDefault(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	pol, err := sandbox.DefaultPolicy(sandbox.Strict, root)
	if err != nil {
		t.Fatal(err)
	}
	p := darwin.SeatbeltProfile(root, pol)

	if !strings.Contains(p, "(deny default)") {
		t.Errorf("strict profile is not deny-default:\n%s", p)
	}
	if strings.Contains(p, "(allow default)") {
		t.Errorf("strict profile must not allow default:\n%s", p)
	}
	if !strings.Contains(p, fmt.Sprintf("(subpath %q)", root)) {
		t.Errorf("strict profile must allow the project root:\n%s", p)
	}
	// Strict denies egress by default: no blanket network allow.
	if strings.Contains(p, "(allow network*)") {
		t.Errorf("strict profile must not allow all network:\n%s", p)
	}
	// Home must not be allowlisted at all (deny-default hides it).
	if home, err := os.UserHomeDir(); err == nil && home != "" && home != "/" {
		if strings.Contains(p, fmt.Sprintf("%q", home)) {
			t.Errorf("strict profile must not reference home %q:\n%s", home, p)
		}
	}
}

// TestSeatbeltProfileAllowsMetadataRead locks in the correctness fix: a
// deny-default profile must still allow filesystem-wide metadata reads, or the
// child cannot stat the ancestors of the project root and fails to start.
// Metadata exposes existence/size/mode, never file contents.
func TestSeatbeltProfileAllowsMetadataRead(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	for _, profile := range []sandbox.Profile{sandbox.Strict, sandbox.Standard} {
		pol, err := sandbox.DefaultPolicy(profile, root)
		if err != nil {
			t.Fatal(err)
		}
		p := darwin.SeatbeltProfile(root, pol)
		if !strings.Contains(p, "(allow file-read-metadata)") {
			t.Errorf("%s profile must allow file-read-metadata for path traversal:\n%s", profile, p)
		}
	}
}

// TestSeatbeltProfileStandardHomeUnset covers userHome's error branch and
// writeStandardHomeReads' home=="" early return: on Unix, os.UserHomeDir
// consults $HOME, so an empty $HOME makes it fail.
// Mutates $HOME via t.Setenv, so it must not run in parallel.
func TestSeatbeltProfileStandardHomeUnset(t *testing.T) {
	t.Setenv("HOME", "")

	root := t.TempDir()
	pol, err := sandbox.DefaultPolicy(sandbox.Standard, root)
	if err != nil {
		t.Fatal(err)
	}
	p := darwin.SeatbeltProfile(root, pol)
	if strings.Contains(p, "(allow file-read* (subpath") {
		t.Errorf("profile should not add a home read allow when $HOME is empty:\n%s", p)
	}
	if strings.Contains(p, "(deny file-read* file-write*") {
		t.Errorf("profile should not add home secret-tree denies when $HOME is empty:\n%s", p)
	}
}

// TestSeatbeltProfileStandardReadDenyEdgeCases exercises
// writeStandardHomeReads' pol.ReadDeny loop: skip-empty, skip-relative, and a
// normal absolute custom entry. Requires a resolvable $HOME to reach the loop
// at all (writeStandardHomeReads returns early otherwise), so it runs with
// whatever $HOME the test process already has.
func TestSeatbeltProfileStandardReadDenyEdgeCases(t *testing.T) {
	t.Parallel()
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skip("no home dir")
	}

	root := t.TempDir()
	pol := sandbox.Policy{
		Profile:       sandbox.Standard,
		WritableRoots: []string{root},
		ReadDeny:      []string{"", "   ", "relative/deny", "/abs/custom-deny"},
	}
	p := darwin.SeatbeltProfile(root, pol)

	if strings.Contains(p, "relative/deny") {
		t.Errorf("relative ReadDeny entries must be skipped:\n%s", p)
	}
	if !strings.Contains(p, "/abs/custom-deny") {
		t.Errorf("absolute ReadDeny entries must produce a deny line:\n%s", p)
	}
}

func TestSeatbeltProfileStandardAllowsEgress(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	pol, err := sandbox.DefaultPolicy(sandbox.Standard, root)
	if err != nil {
		t.Fatal(err)
	}
	p := darwin.SeatbeltProfile(root, pol)
	if !strings.Contains(p, "(deny default)") {
		t.Errorf("standard profile is not deny-default:\n%s", p)
	}
	if !strings.Contains(p, "(allow network*)") {
		t.Errorf("standard profile should allow egress:\n%s", p)
	}
	// Standard reads home but denies the secret subtrees.
	if home, err := os.UserHomeDir(); err == nil && home != "" && home != "/" {
		if !strings.Contains(p, "/.ssh") {
			t.Errorf("standard profile should explicitly deny ~/.ssh:\n%s", p)
		}
	}
}
