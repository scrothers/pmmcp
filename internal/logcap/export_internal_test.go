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

package logcap

import (
	"archive/tar"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// countingFailWriter fails every Write call after the first failAfter calls
// succeed. Used to force tar.Writer's WriteHeader/Write to fail deterministically
// without going through gzip's buffering (see export_fault_test.go for the
// gzip-wrapped variant exercised through the public API).
type countingFailWriter struct {
	calls     int
	failAfter int
}

func (w *countingFailWriter) Write(p []byte) (int, error) {
	w.calls++
	if w.calls > w.failAfter {
		return 0, errors.New("injected tar write failure")
	}
	return len(p), nil
}

func TestAddFileToTarMissingFileReturnsNil(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	var cw countingFailWriter
	cw.failAfter = 1000
	tw := tar.NewWriter(&cw)
	if err := addFileToTar(tw, dir, "does-not-exist.log"); err != nil {
		t.Fatalf("addFileToTar for missing file = %v, want nil", err)
	}
}

func TestAddFileToTarStatPermissionDenied(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits don't block reads the way chmod 600 implies")
	}
	if os.Getuid() == 0 {
		t.Skip("running as root: directory permission bits don't block root")
	}
	base := t.TempDir()
	dir := filepath.Join(base, "sub")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "stdout.log"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Remove execute (search) permission on dir: lookups inside it now fail
	// with EACCES rather than ENOENT, hitting the non-NotExist stat branch.
	if err := os.Chmod(dir, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	var cw countingFailWriter
	cw.failAfter = 1000
	tw := tar.NewWriter(&cw)
	err := addFileToTar(tw, dir, "stdout.log")
	if err == nil {
		t.Fatal("expected stat permission error")
	}
	if !strings.Contains(err.Error(), "export stat") {
		t.Fatalf("err = %v, want export stat", err)
	}
}

func TestAddFileToTarOpenPermissionDenied(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits don't block reads the way chmod 000 implies")
	}
	if os.Getuid() == 0 {
		t.Skip("running as root: file permission bits don't block root reads")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "stdout.log")
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o600) })

	var cw countingFailWriter
	cw.failAfter = 1000
	tw := tar.NewWriter(&cw)
	err := addFileToTar(tw, dir, "stdout.log")
	if err == nil {
		t.Fatal("expected open permission error")
	}
	if !strings.Contains(err.Error(), "export open") {
		t.Fatalf("err = %v, want export open", err)
	}
}

func TestAddFileToTarWriteHeaderFails(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "stdout.log"), []byte("hello\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cw := &countingFailWriter{failAfter: 0} // fails on the very first Write (the header)
	tw := tar.NewWriter(cw)
	err := addFileToTar(tw, dir, "stdout.log")
	if err == nil {
		t.Fatal("expected header write error")
	}
	if !strings.Contains(err.Error(), "export header stdout.log") {
		t.Fatalf("err = %v, want export header stdout.log", err)
	}
}

func TestAddFileToTarCopyFails(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "stdout.log"), []byte("hello world\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cw := &countingFailWriter{failAfter: 3} // header succeeds (3 underlying writes), body copy fails
	tw := tar.NewWriter(cw)
	err := addFileToTar(tw, dir, "stdout.log")
	if err == nil {
		t.Fatal("expected copy error")
	}
	if !strings.Contains(err.Error(), "export copy stdout.log") {
		t.Fatalf("err = %v, want export copy stdout.log", err)
	}
}

func TestWriteTarBytesHeaderFails(t *testing.T) {
	t.Parallel()
	cw := &countingFailWriter{failAfter: 0}
	tw := tar.NewWriter(cw)
	err := writeTarBytes(tw, "manifest.json", []byte("{}"))
	if err == nil {
		t.Fatal("expected header error")
	}
	if !strings.Contains(err.Error(), "export header manifest.json") {
		t.Fatalf("err = %v, want export header manifest.json", err)
	}
}

func TestWriteTarBytesWriteFails(t *testing.T) {
	t.Parallel()
	cw := &countingFailWriter{failAfter: 3}
	tw := tar.NewWriter(cw)
	err := writeTarBytes(tw, "manifest.json", []byte("some manifest bytes"))
	if err == nil {
		t.Fatal("expected write error")
	}
	if !strings.Contains(err.Error(), "export write manifest.json") {
		t.Fatalf("err = %v, want export write manifest.json", err)
	}
}
