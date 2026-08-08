// Copyright 2026 Steven Crothers
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package windows_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/scrothers/pmmcp/internal/sandbox"
	"github.com/scrothers/pmmcp/internal/sandbox/windows"
)

func TestApplyContextCanceled(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := windows.Apply(ctx, sandbox.Policy{Profile: sandbox.Off})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestApplyStandardEmptyProjectRoot(t *testing.T) {
	t.Parallel()
	pol, err := sandbox.DefaultPolicy(sandbox.Standard, "")
	if err != nil {
		t.Fatal(err)
	}
	_, err = windows.Apply(context.Background(), pol)
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
	got, err := windows.Apply(context.Background(), pol)
	if err != nil {
		t.Fatal(err)
	}
	if got.Profile != sandbox.Permissive || got.Mode != sandbox.ModePolicy {
		t.Fatalf("got %+v", got)
	}
}

func TestApplyStrictFailsClosed(t *testing.T) {
	t.Parallel()
	// Windows has no FS isolation in MVP: strict must fail closed.
	pol, err := sandbox.DefaultPolicy(sandbox.Strict, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := windows.Apply(context.Background(), pol); !errors.Is(err, sandbox.ErrStrictUnsupported) {
		t.Fatalf("err = %v, want ErrStrictUnsupported", err)
	}
}

func TestApplyStandard(t *testing.T) {
	t.Parallel()
	pol, err := sandbox.DefaultPolicy(sandbox.Standard, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	got, err := windows.Apply(context.Background(), pol)
	if err != nil {
		t.Fatal(err)
	}
	if got.Profile != sandbox.Standard || got.Mode != "job-object" {
		t.Fatalf("got %+v", got)
	}
}

func TestApplyStrictEmptyProjectRoot(t *testing.T) {
	t.Parallel()
	pol, err := sandbox.DefaultPolicy(sandbox.Strict, "")
	if err != nil {
		t.Fatal(err)
	}
	_, err = windows.Apply(context.Background(), pol)
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
	got, err := windows.Apply(context.Background(), pol)
	if err != nil {
		t.Fatal(err)
	}
	if got.Mode != sandbox.ModeOff {
		t.Fatalf("mode = %q", got.Mode)
	}
}

func TestApplyUnknown(t *testing.T) {
	t.Parallel()
	_, err := windows.Apply(context.Background(), sandbox.Policy{Profile: "nope"})
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
	// Strict Apply itself fails closed on Windows; the policy path-checks below
	// are the portable layer that still applies regardless.
	// Unix-style path still denied by policy (portable.ssh check).
	if pol.AllowsRead(`/Users/dev/.ssh/id_rsa`) {
		t.Fatal("strict must deny .ssh")
	}
	if pol.AllowsRead(`C:\Users\dev\.ssh\id_rsa`) {
		t.Fatal("strict must deny Windows .ssh path")
	}
	if !pol.AllowsWrite(filepath.Join(root, "out.txt")) {
		t.Fatal("project write should allow")
	}
}
