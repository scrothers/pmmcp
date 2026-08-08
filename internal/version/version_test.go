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

package version_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/scrothers/pmmcp/internal/version"
)

func TestStringDefault(t *testing.T) {
	t.Parallel()
	// With a plain `go test` (no -ldflags), the package defaults hold.
	want := "0.0.0-dev (commit=unknown date=unknown)"
	if got := version.String(); got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
}

// TestLdflagsInjection pins the fully-qualified -X variable paths used by
// release builds. A rename of the package, of any of the three vars, or of the
// String() format would break stamping silently in production; this builds a
// tiny binary with the exact -ldflags the README/Makefile use and asserts the
// injected values round-trip through String().
func TestLdflagsInjection(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skipf("go toolchain unavailable: %v", err)
	}

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	moduleRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")

	tmp := t.TempDir()
	mainSrc := `package main

import (
	"fmt"

	"github.com/scrothers/pmmcp/internal/version"
)

func main() { fmt.Print(version.String()) }
`
	if err := os.WriteFile(filepath.Join(tmp, "main.go"), []byte(mainSrc), 0o600); err != nil {
		t.Fatal(err)
	}
	// Name the throwaway module under the pmmcp path prefix so importing the
	// internal/version package is permitted (Go's internal-import rule keys on
	// the import path prefix, not the module identity).
	goMod := "module github.com/scrothers/pmmcp/ldflagcheck\n\ngo 1.25.0\n\nrequire github.com/scrothers/pmmcp v0.0.0\n\nreplace github.com/scrothers/pmmcp => " + moduleRoot + "\n"
	if err := os.WriteFile(filepath.Join(tmp, "go.mod"), []byte(goMod), 0o600); err != nil {
		t.Fatal(err)
	}

	const pkg = "github.com/scrothers/pmmcp/internal/version"
	ldflags := strings.Join([]string{
		"-X " + pkg + ".Version=9.9.9",
		"-X " + pkg + ".Commit=deadbeef",
		"-X " + pkg + ".BuildDate=2026-01-02",
	}, " ")
	bin := filepath.Join(tmp, "ldflagcheck")

	build := exec.Command("go", "build", "-ldflags", ldflags, "-o", bin, ".")
	build.Dir = tmp
	build.Env = append(os.Environ(), "GOFLAGS=-mod=mod")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build failed: %v\n%s", err, out)
	}

	out, err := exec.Command(bin).CombinedOutput()
	if err != nil {
		t.Fatalf("run failed: %v\n%s", err, out)
	}
	want := "9.9.9 (commit=deadbeef date=2026-01-02)"
	if got := string(out); got != want {
		t.Fatalf("stamped String() = %q, want %q", got, want)
	}
}
