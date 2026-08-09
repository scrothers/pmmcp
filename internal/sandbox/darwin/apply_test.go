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
	"context"
	"errors"
	"runtime"
	"testing"

	"github.com/scrothers/pmmcp/internal/sandbox"
	"github.com/scrothers/pmmcp/internal/sandbox/darwin"
)

func TestApplyContextCanceled(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := darwin.Apply(ctx, sandbox.Policy{Profile: sandbox.Off})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestApplyStandard(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	pol, err := sandbox.DefaultPolicy(sandbox.Standard, root)
	if err != nil {
		t.Fatal(err)
	}
	got, err := darwin.Apply(context.Background(), pol)
	if err != nil {
		t.Fatal(err)
	}
	// On a real macOS runner, IsolationAvailable() finds the genuine
	// sandbox-exec binary on PATH and Apply upgrades to seatbelt mode; every
	// other GOOS uses the non-darwin stub, which is always unavailable.
	wantMode := sandbox.ModePolicy
	if runtime.GOOS == "darwin" {
		wantMode = "seatbelt"
	}
	if got.Profile != sandbox.Standard || got.Mode != wantMode {
		t.Fatalf("got %+v, want mode %q", got, wantMode)
	}
}

func TestApplyStandardEmptyProjectRoot(t *testing.T) {
	t.Parallel()
	pol, err := sandbox.DefaultPolicy(sandbox.Standard, "")
	if err != nil {
		t.Fatal(err)
	}
	_, err = darwin.Apply(context.Background(), pol)
	if !errors.Is(err, sandbox.ErrProjectRootRequired) {
		t.Fatalf("err = %v, want ErrProjectRootRequired", err)
	}
}

func TestApplyPermissive(t *testing.T) {
	t.Parallel()
	pol, err := sandbox.DefaultPolicy(sandbox.Permissive, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	got, err := darwin.Apply(context.Background(), pol)
	if err != nil {
		t.Fatal(err)
	}
	if got.Profile != sandbox.Permissive || got.Mode != sandbox.ModePolicy {
		t.Fatalf("got %+v", got)
	}
}

// TestApplySeatbeltMode exercises the mode="seatbelt" branch for both Strict
// and Standard via the IsolationAvailable test seam (see seatbelt_stub.go).
// It mutates a package-level var, so it cannot run in parallel with other
// darwin tests; the override is restored before this test returns, which is
// always before any t.Parallel() sibling in this package actually executes.
func TestApplySeatbeltMode(t *testing.T) {
	orig := darwin.IsolationAvailable
	darwin.IsolationAvailable = func() bool { return true }
	t.Cleanup(func() { darwin.IsolationAvailable = orig })

	root := t.TempDir()
	for _, profile := range []sandbox.Profile{sandbox.Strict, sandbox.Standard} {
		pol, err := sandbox.DefaultPolicy(profile, root)
		if err != nil {
			t.Fatal(err)
		}
		got, err := darwin.Apply(context.Background(), pol)
		if err != nil {
			t.Fatal(err)
		}
		if got.Mode != "seatbelt" {
			t.Fatalf("profile %q: mode = %q, want seatbelt", profile, got.Mode)
		}
	}
}

func TestApplyStrict(t *testing.T) {
	t.Parallel()
	pol, err := sandbox.DefaultPolicy(sandbox.Strict, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	got, err := darwin.Apply(context.Background(), pol)
	if err != nil {
		t.Fatal(err)
	}
	// On a real macOS runner, IsolationAvailable() finds the genuine
	// sandbox-exec binary on PATH and Apply upgrades to seatbelt mode; every
	// other GOOS uses the non-darwin stub, which is always unavailable.
	wantMode := sandbox.ModePolicy
	if runtime.GOOS == "darwin" {
		wantMode = "seatbelt"
	}
	if got.Profile != sandbox.Strict || got.Mode != wantMode {
		t.Fatalf("got %+v, want mode %q", got, wantMode)
	}
}

func TestApplyStrictEmptyProjectRoot(t *testing.T) {
	t.Parallel()
	pol, err := sandbox.DefaultPolicy(sandbox.Strict, "")
	if err != nil {
		t.Fatal(err)
	}
	_, err = darwin.Apply(context.Background(), pol)
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
	got, err := darwin.Apply(context.Background(), pol)
	if err != nil {
		t.Fatal(err)
	}
	if got.Mode != sandbox.ModeOff {
		t.Fatalf("mode = %q", got.Mode)
	}
}

func TestApplyUnknown(t *testing.T) {
	t.Parallel()
	_, err := darwin.Apply(context.Background(), sandbox.Policy{Profile: "nope"})
	if !errors.Is(err, sandbox.ErrUnknownProfile) {
		t.Fatalf("err = %v", err)
	}
}

func TestAllowsReadWriteStrict(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	pol, err := sandbox.DefaultPolicy(sandbox.Strict, root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := darwin.Apply(context.Background(), pol); err != nil {
		t.Fatal(err)
	}
	if pol.AllowsRead("/Users/dev/.ssh/id_rsa") {
		t.Fatal("strict must deny .ssh")
	}
	if !pol.AllowsWrite(root + "/out") {
		t.Fatal("project write should allow")
	}
}
