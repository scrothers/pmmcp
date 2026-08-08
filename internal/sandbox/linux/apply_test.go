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

package linux_test

import (
	"context"
	"errors"
	"testing"

	"github.com/scrothers/pmmcp/internal/sandbox"
	"github.com/scrothers/pmmcp/internal/sandbox/linux"
)

func TestApplyStrict(t *testing.T) {
	t.Parallel()
	pol, err := sandbox.DefaultPolicy(sandbox.Strict, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	got, err := linux.Apply(context.Background(), pol)
	if err != nil {
		t.Fatal(err)
	}
	if got.Profile != sandbox.Strict {
		t.Fatalf("profile = %q", got.Profile)
	}
	// Landlock when kernel supports it; otherwise path policy.
	if got.Mode != sandbox.ModePolicy && got.Mode != sandbox.ModeBwrap {
		t.Fatalf("mode = %q, want policy or landlock", got.Mode)
	}
}

func TestApplyStrictEmptyProjectRoot(t *testing.T) {
	t.Parallel()
	pol, err := sandbox.DefaultPolicy(sandbox.Strict, "")
	if err != nil {
		t.Fatal(err)
	}
	_, err = linux.Apply(context.Background(), pol)
	if !errors.Is(err, sandbox.ErrProjectRootRequired) {
		t.Fatalf("err = %v, want ErrProjectRootRequired", err)
	}
}

func TestApplyOff(t *testing.T) {
	t.Parallel()
	pol, err := sandbox.DefaultPolicy(sandbox.Off, "")
	if err != nil {
		t.Fatal(err)
	}
	got, err := linux.Apply(context.Background(), pol)
	if err != nil {
		t.Fatal(err)
	}
	if got.Mode != sandbox.ModeOff {
		t.Fatalf("mode = %q, want off", got.Mode)
	}
}

func TestApplyUnknown(t *testing.T) {
	t.Parallel()
	_, err := linux.Apply(context.Background(), sandbox.Policy{Profile: "nope"})
	if !errors.Is(err, sandbox.ErrUnknownProfile) {
		t.Fatalf("err = %v, want ErrUnknownProfile", err)
	}
}

func TestApplyCanceled(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	pol, err := sandbox.DefaultPolicy(sandbox.Strict, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_, err = linux.Apply(ctx, pol)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestApplyStandardAndPermissive(t *testing.T) {
	t.Parallel()
	std, err := sandbox.DefaultPolicy(sandbox.Standard, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	got, err := linux.Apply(context.Background(), std)
	if err != nil {
		t.Fatal(err)
	}
	if got.Mode != sandbox.ModePolicy && got.Mode != sandbox.ModeBwrap {
		t.Fatalf("standard mode = %q", got.Mode)
	}

	perm, err := sandbox.DefaultPolicy(sandbox.Permissive, "")
	if err != nil {
		t.Fatal(err)
	}
	got, err = linux.Apply(context.Background(), perm)
	if err != nil {
		t.Fatal(err)
	}
	if got.Mode != sandbox.ModePolicy {
		t.Fatalf("permissive mode = %q", got.Mode)
	}
}

func TestTryBwrap(t *testing.T) {
	t.Parallel()
	if !linux.BwrapAvailable() {
		t.Skip("bwrap not on PATH")
	}
	root := t.TempDir()
	argv, ok := linux.TryBwrap([]string{"/bin/echo", "hi"}, root)
	if !ok {
		t.Fatal("TryBwrap should succeed when bwrap is available")
	}
	if len(argv) < 3 || argv[len(argv)-2] != "/bin/echo" {
		t.Fatalf("argv = %v", argv)
	}
	_, ok = linux.TryBwrap(nil, root)
	if ok {
		t.Fatal("empty cmd should fail")
	}
}

func TestApplyStandardEmptyProjectRoot(t *testing.T) {
	t.Parallel()
	// DefaultPolicy(standard, "") yields a temp-only writable root, which
	// HasProjectRoot rejects.
	pol, err := sandbox.DefaultPolicy(sandbox.Standard, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := linux.Apply(context.Background(), pol); !errors.Is(err, sandbox.ErrProjectRootRequired) {
		t.Fatalf("err = %v, want ErrProjectRootRequired", err)
	}
}

// TestApplyModeWithoutBwrap pins the fallback half of effectiveMode: with no
// `bwrap` reachable on PATH, restrictive profiles degrade to the path-policy
// mode rather than claiming kernel-backed confinement.
//
// It mutates PATH for the whole process, so it must not call t.Parallel.
func TestApplyModeWithoutBwrap(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if linux.BwrapAvailable() {
		t.Fatal("bwrap still resolvable after emptying PATH")
	}
	// IsolationAvailable still reports Landlock when the kernel has it.
	if got, want := linux.IsolationAvailable(), linux.LandlockAvailable(); got != want {
		t.Errorf("IsolationAvailable() = %v, want %v (bwrap hidden)", got, want)
	}

	for _, profile := range []sandbox.Profile{sandbox.Strict, sandbox.Standard} {
		pol, err := sandbox.DefaultPolicy(profile, t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		got, err := linux.Apply(context.Background(), pol)
		if err != nil {
			t.Fatalf("Apply(%s): %v", profile, err)
		}
		if got.Mode != sandbox.ModePolicy {
			t.Errorf("Apply(%s).Mode = %q, want %q", profile, got.Mode, sandbox.ModePolicy)
		}
		if got.Profile != profile {
			t.Errorf("Apply(%s).Profile = %q", profile, got.Profile)
		}
	}
}

// TestApplyModeWithBwrap is the other half: bubblewrap present ⇒ mode "bwrap".
func TestApplyModeWithBwrap(t *testing.T) {
	t.Parallel()
	if !linux.BwrapAvailable() {
		t.Skip("bwrap not on PATH")
	}
	if !linux.IsolationAvailable() {
		t.Error("IsolationAvailable() = false with bwrap on PATH")
	}
	for _, profile := range []sandbox.Profile{sandbox.Strict, sandbox.Standard} {
		pol, err := sandbox.DefaultPolicy(profile, t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		got, err := linux.Apply(context.Background(), pol)
		if err != nil {
			t.Fatalf("Apply(%s): %v", profile, err)
		}
		if got.Mode != sandbox.ModeBwrap {
			t.Errorf("Apply(%s).Mode = %q, want %q", profile, got.Mode, sandbox.ModeBwrap)
		}
	}
}

func TestApplyStrictDeniesSSHViaPolicy(t *testing.T) {
	t.Parallel()
	// Policy layer used after Apply for path checks (secret-path deny class).
	pol, err := sandbox.DefaultPolicy(sandbox.Strict, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := linux.Apply(context.Background(), pol); err != nil {
		t.Fatal(err)
	}
	if pol.AllowsRead("/home/dev/.ssh/id_rsa") {
		t.Fatal("strict policy must deny .ssh")
	}
}
