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

//go:build unix

package logcap

import (
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// withLowFileSizeLimit lowers RLIMIT_FSIZE for the duration of fn and ignores
// SIGXFSZ so an over-limit write returns EFBIG instead of killing the
// process. gzipFile writes through concrete *os.File values (not injectable
// io.Writers), so this is the only portable, non-root way to force its
// output writes to fail deterministically.
//
// RLIMIT_FSIZE is process-wide, so this helper is only used by non-parallel
// tests: they run to completion (restoring the limit before returning)
// before the package's t.Parallel() tests are ever unpaused to run
// concurrently, per Go's test scheduling model.
func withLowFileSizeLimit(t *testing.T, limit uint64, fn func()) {
	t.Helper()
	var orig syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_FSIZE, &orig); err != nil {
		t.Fatalf("getrlimit: %v", err)
	}
	signal.Ignore(syscall.SIGXFSZ)
	lowered := syscall.Rlimit{Cur: limit, Max: orig.Max}
	if err := syscall.Setrlimit(syscall.RLIMIT_FSIZE, &lowered); err != nil {
		t.Fatalf("setrlimit: %v", err)
	}
	defer func() {
		if err := syscall.Setrlimit(syscall.RLIMIT_FSIZE, &orig); err != nil {
			t.Fatalf("restore rlimit: %v", err)
		}
	}()
	fn()
}

func TestGzipFileOpenMissing(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	err := gzipFile(filepath.Join(dir, "does-not-exist"))
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "gzip open") {
		t.Fatalf("err = %v, want gzip open", err)
	}
}

func TestGzipFileCreateFails(t *testing.T) {
	t.Parallel()
	if os.Getuid() == 0 {
		t.Skip("running as root: directory permission bits don't block root")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "stdout.log.1")
	if err := os.WriteFile(path, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	err := gzipFile(path)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "gzip create") {
		t.Fatalf("err = %v, want gzip create", err)
	}
}

func TestGzipFileCopyFailsOnDirectory(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// Opening a directory for read succeeds, but reading from it (as io.Copy
	// does) fails with EISDIR — a deterministic way to force the io.Copy
	// branch without any faulty io.Reader plumbing.
	sub := filepath.Join(dir, "adir")
	if err := os.Mkdir(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	err := gzipFile(sub)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "gzip write") {
		t.Fatalf("err = %v, want gzip write", err)
	}
}

func TestGzipFileCloseFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "stdout.log.1")
	if err := os.WriteFile(path, []byte("small content to compress"), 0o600); err != nil {
		t.Fatal(err)
	}
	// gzip.Writer's header includes the (null-terminated) Name set by
	// gzipFile, so the exact byte count flushed during the initial Write
	// (called from io.Copy) depends on the input path's basename length. For
	// "stdout.log.1", a RLIMIT_FSIZE of 30 bytes comfortably lets that
	// header-plus-compressed-data write through, but leaves no room for the
	// final deflate block plus 8-byte CRC32/size trailer that Close emits.
	withLowFileSizeLimit(t, 30, func() {
		err := gzipFile(path)
		if err == nil {
			t.Fatal("expected gzip close error")
		}
		if !strings.Contains(err.Error(), "gzip close") {
			t.Fatalf("err = %v, want gzip close", err)
		}
	})
}

func TestGzipFileRemovePlainFails(t *testing.T) {
	t.Parallel()
	if os.Getuid() == 0 {
		t.Skip("running as root: directory permission bits don't block root")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "stdout.log.1")
	if err := os.WriteFile(path, []byte("content"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Pre-create the output .gz file: opening it with O_CREATE|O_TRUNC then
	// only needs write permission on the file itself, not on the directory,
	// so the compression steps all succeed even once dir is read-only.
	// Removing the original plain file afterward, however, needs to unlink a
	// directory entry, which does require directory write permission.
	if err := os.WriteFile(path+".gz", []byte("stale"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	err := gzipFile(path)
	if err == nil {
		t.Fatal("expected remove error")
	}
	if !strings.Contains(err.Error(), "gzip remove plain") {
		t.Fatalf("err = %v, want gzip remove plain", err)
	}
}

func TestRotateNowRemoveOldestPermissionDenied(t *testing.T) {
	t.Parallel()
	if os.Getuid() == 0 {
		t.Skip("running as root: directory permission bits don't block root")
	}
	dir := t.TempDir()
	c := &Capturer{Dir: dir, MaxFiles: 1, Compress: false}
	// MaxFiles=1 makes the active file's own oldest backup slot base.1: create
	// it so the very first remove step targets an existing entry.
	if err := os.WriteFile(filepath.Join(dir, stdoutName+".1"), []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	err := c.rotateNow(stdoutName)
	if err == nil {
		t.Fatal("expected remove-permission error")
	}
	if !strings.Contains(err.Error(), "remove") {
		t.Fatalf("err = %v, want a remove error", err)
	}
}

func TestRotateNowShiftRenamePermissionDenied(t *testing.T) {
	t.Parallel()
	if os.Getuid() == 0 {
		t.Skip("running as root: directory permission bits don't block root")
	}
	dir := t.TempDir()
	c := &Capturer{Dir: dir, MaxFiles: 2, Compress: false}
	// base.1 exists but base.2 (oldest, removed first) does not, so the
	// oldest-removal step is a tolerated no-op (ENOENT even on a read-only
	// dir) and the failure surfaces in the shift loop's rename instead.
	if err := os.WriteFile(filepath.Join(dir, stdoutName+".1"), []byte("one"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	err := c.rotateNow(stdoutName)
	if err == nil {
		t.Fatal("expected rename-permission error")
	}
	if !strings.Contains(err.Error(), "rename") {
		t.Fatalf("err = %v, want a rename error", err)
	}
}

func TestRotateNowActiveFileMissingReturnsNil(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	c := &Capturer{Dir: dir, MaxFiles: 2, Compress: false}
	// No active file and no backups at all: every step is a tolerated no-op,
	// including the final active-file rename hitting its IsNotExist shortcut.
	if err := c.rotateNow(stdoutName); err != nil {
		t.Fatalf("rotateNow with nothing to rotate = %v, want nil", err)
	}
}

func TestRotateNowActiveRenamePermissionDenied(t *testing.T) {
	t.Parallel()
	if os.Getuid() == 0 {
		t.Skip("running as root: directory permission bits don't block root")
	}
	dir := t.TempDir()
	c := &Capturer{Dir: dir, MaxFiles: 2, Compress: false}
	if err := os.WriteFile(filepath.Join(dir, stdoutName), []byte("active"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	err := c.rotateNow(stdoutName)
	if err == nil {
		t.Fatal("expected active-file rename error")
	}
	if !strings.Contains(err.Error(), "rename") {
		t.Fatalf("err = %v, want a rename error", err)
	}
}

func TestRotateNowCompressFailurePropagates(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	c := &Capturer{Dir: dir, MaxFiles: 2, Compress: true}
	// The "active file" is a directory: the rename to base.1 succeeds (rename
	// doesn't care about file type when the destination is free), but the
	// subsequent gzipFile compression step fails reading it (EISDIR),
	// propagating the error out of rotateNow.
	if err := os.Mkdir(filepath.Join(dir, stdoutName), 0o700); err != nil {
		t.Fatal(err)
	}
	err := c.rotateNow(stdoutName)
	if err == nil {
		t.Fatal("expected gzip compression error to propagate")
	}
	if !strings.Contains(err.Error(), "gzip write") {
		t.Fatalf("err = %v, want a gzip write error", err)
	}
}
