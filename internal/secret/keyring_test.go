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

package secret_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/scrothers/pmmcp/internal/secret"
)

func TestFileBackend(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	b, err := secret.NewFileBackend(filepath.Join(dir, "kr"))
	if err != nil {
		t.Fatal(err)
	}
	path, err := b.Set("api_token", "s3cret")
	if err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm()&0o077 != 0 {
		t.Fatalf("perms too open: %v", st.Mode())
	}
	v, err := b.Get("api_token")
	if err != nil || v != "s3cret" {
		t.Fatalf("get %q %v", v, err)
	}
	names, err := b.ListNames()
	if err != nil || len(names) != 1 || names[0] != "api_token" {
		t.Fatalf("%v %v", names, err)
	}
}

func TestFileBackendRejectsTraversal(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	b, err := secret.NewFileBackend(filepath.Join(dir, "kr"))
	if err != nil {
		t.Fatal(err)
	}
	// Plant a file the traversal would target if validation were missing.
	secretOutside := filepath.Join(dir, "outside")
	if err := os.WriteFile(secretOutside, []byte("TOPSECRET"), 0o600); err != nil {
		t.Fatal(err)
	}
	bad := []string{"../outside", "../../etc/hostname", "a/b", "..", "", `..\outside`}
	for _, name := range bad {
		if _, err := b.Get(name); err == nil {
			t.Errorf("Get(%q) should be rejected", name)
		}
		if _, err := b.Set(name, "x"); err == nil {
			t.Errorf("Set(%q) should be rejected", name)
		}
	}
}

func TestNewFileBackendTightensDirMode(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "kr")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := secret.NewFileBackend(dir); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("dir mode not tightened: %v", info.Mode().Perm())
	}
}

// TestNewFileBackendMkdirAllError covers MkdirAll's error return: a path
// component that is a regular file (not a directory) makes any MkdirAll
// underneath it fail with ENOTDIR.
func TestNewFileBackendMkdirAllError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	notADir := filepath.Join(dir, "file")
	if err := os.WriteFile(notADir, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := secret.NewFileBackend(filepath.Join(notADir, "sub")); err == nil {
		t.Fatal("want error when a path component is a regular file")
	}
}

// TestNewFileBackendChmodError covers the dir-chmod error return. A symlink
// to a directory we do not own (/tmp, owned by root) already exists (so
// MkdirAll's fast path succeeds without modifying anything), but the
// subsequent Chmod fails with EPERM since only the owner or root may chmod.
func TestNewFileBackendChmodError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	link := filepath.Join(dir, "not-owned")
	if err := os.Symlink("/tmp", link); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	if _, err := secret.NewFileBackend(link); err == nil {
		t.Fatal("want error chmod-ing a directory we do not own")
	}
}

// TestFileBackendSetWriteError covers Set's WriteFile error return: a
// keyring directory with no write permission rejects file creation.
func TestFileBackendSetWriteError(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "kr")
	b, err := secret.NewFileBackend(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	if _, err := b.Set("readonly", "x"); err == nil {
		t.Fatal("want write error in a read-only keyring dir")
	}
}

// TestFileBackendSetChmodError covers Set's post-write Chmod error return.
// The name resolves to a symlink pointing at /dev/null: writing through it
// succeeds (world-writable), but chmod-ing /dev/null fails since it is
// owned by root.
func TestFileBackendSetChmodError(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "kr")
	b, err := secret.NewFileBackend(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/dev/null", filepath.Join(dir, "devnull")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	if _, err := b.Set("devnull", "test-secret-do-not-use"); err == nil {
		t.Fatal("want chmod error writing through a symlink to /dev/null")
	}
}

// TestFileBackendListNamesReadDirError covers ListNames' ReadDir error
// return: the keyring directory is removed out from under the backend.
func TestFileBackendListNamesReadDirError(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "kr")
	b, err := secret.NewFileBackend(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := b.ListNames(); err == nil {
		t.Fatal("want ReadDir error once the keyring dir is gone")
	}
}
