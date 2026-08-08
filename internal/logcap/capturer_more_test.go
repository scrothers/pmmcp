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

package logcap_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scrothers/pmmcp/internal/logcap"
)

func TestNewMkdirAllFails(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	blocker := filepath.Join(base, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	// blocker is a regular file, so MkdirAll can't descend through it to
	// create "sub" underneath.
	dir := filepath.Join(blocker, "sub")
	if _, err := logcap.New(dir, 1, 1); err == nil {
		t.Fatal("expected mkdir error when a path component is a file")
	}
}

func TestOpenStdoutMissingParentDir(t *testing.T) {
	t.Parallel()
	// A Capturer built directly (bypassing New, so the dir is never created)
	// makes the underlying os.OpenFile fail: the parent directory is absent.
	c := &logcap.Capturer{Dir: filepath.Join(t.TempDir(), "missing", "nested")}
	if _, err := c.OpenStdout(); err == nil {
		t.Fatal("expected OpenStdout error for missing parent dir")
	}
	if _, err := c.OpenStderr(); err == nil {
		t.Fatal("expected OpenStderr error for missing parent dir")
	}
}

func TestRotateIfNeededStatPermissionDenied(t *testing.T) {
	t.Parallel()
	if os.Getuid() == 0 {
		t.Skip("running as root: directory permission bits don't block root")
	}
	dir := t.TempDir()
	c, err := logcap.New(dir, 1, 2)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := os.WriteFile(c.StdoutPath(), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Remove execute (search) permission: os.Stat on an entry inside dir now
	// fails with EACCES rather than ENOENT.
	if err := os.Chmod(dir, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	err = c.RotateIfNeeded()
	if err == nil {
		t.Fatal("expected RotateIfNeeded error")
	}
	if !strings.Contains(err.Error(), "stat") {
		t.Fatalf("err = %v, want a stat error", err)
	}
}
