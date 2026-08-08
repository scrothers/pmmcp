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
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scrothers/pmmcp/internal/logcap"
)

// failAfterWriter is a deterministic fault-injecting io.Writer: it accepts the
// first failAfter Write calls, then fails every call after that. Used to force
// tar/gzip's internal Close-time flushes (and the tar/gzip Write calls that
// happen inline) to fail without any real disk error, root privilege, or
// unsafe fd manipulation.
type failAfterWriter struct {
	calls     int
	failAfter int
}

func (w *failAfterWriter) Write(p []byte) (int, error) {
	w.calls++
	if w.calls > w.failAfter {
		return 0, errors.New("injected write failure")
	}
	return len(p), nil
}

// hexBlob returns a deterministic-size, incompressible (hex-encoded random)
// string suitable for inflating manifest.json past gzip's internal flush
// thresholds, so a fault can be timed to land on a specific tar/gzip Write
// call rather than being absorbed into an internal buffer until Close.
func hexBlob(t *testing.T, n int) string {
	t.Helper()
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("rand: %v", err)
	}
	return hex.EncodeToString(b)
}

func TestExportManifestWriteHeaderFails(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	fw := &failAfterWriter{failAfter: 0} // fails on the very first underlying write
	err := logcap.ExportTarGzWithOptions(dir, fw, logcap.ExportOptions{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "export header manifest.json") {
		t.Fatalf("err = %v, want export header manifest.json", err)
	}
}

func TestExportManifestWriteDataFails(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// A large enough manifest (via Meta) forces gzip to flush mid-stream during
	// the explicit tw.Write(data) call rather than deferring everything to Close.
	fw := &failAfterWriter{failAfter: 1}
	opts := logcap.ExportOptions{Meta: map[string]string{"blob": hexBlob(t, 45000)}}
	err := logcap.ExportTarGzWithOptions(dir, fw, opts)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "export write manifest.json") {
		t.Fatalf("err = %v, want export write manifest.json", err)
	}
}

func TestExportTarCloseFails(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// A mid-sized manifest: big enough that gzip flushes once during tar's own
	// Close (writing its final zero blocks), small enough that the explicit
	// tw.Write of the manifest body still succeeds.
	fw := &failAfterWriter{failAfter: 1}
	opts := logcap.ExportOptions{Meta: map[string]string{"blob": hexBlob(t, 15000)}}
	err := logcap.ExportTarGzWithOptions(dir, fw, opts)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "export tar close") {
		t.Fatalf("err = %v, want export tar close", err)
	}
}

func TestExportGzipCloseFails(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// A tiny manifest: gzip buffers everything internally and only flushes to
	// the underlying writer during its own trailer-writing Close.
	fw := &failAfterWriter{failAfter: 1}
	err := logcap.ExportTarGzWithOptions(dir, fw, logcap.ExportOptions{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "export gzip close") {
		t.Fatalf("err = %v, want export gzip close", err)
	}
}

func TestExportAddFileCopyFails(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// A large, incompressible active log forces gzip to flush mid-copy, so the
	// injected failure surfaces inside addFileToTar's io.Copy (and propagates
	// through ExportTarGzWithOptions's per-file loop) rather than at Close.
	if err := os.WriteFile(filepath.Join(dir, "stdout.log"), []byte(hexBlob(t, 60000)), 0o600); err != nil {
		t.Fatal(err)
	}
	fw := &failAfterWriter{failAfter: 1}
	err := logcap.ExportTarGzWithOptions(dir, fw, logcap.ExportOptions{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "export copy stdout.log") {
		t.Fatalf("err = %v, want export copy stdout.log", err)
	}
}

func TestExportGlobBadPattern(t *testing.T) {
	t.Parallel()
	// A directory name containing an unmatched '[' makes the base+"*" glob
	// pattern malformed, deterministically forcing filepath.Glob to return
	// ErrBadPattern regardless of the directory's actual contents.
	base := t.TempDir()
	dir := filepath.Join(base, "[")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	var buf strings.Builder
	err := logcap.ExportTarGzWithOptions(dir, sinkWriter{&buf}, logcap.ExportOptions{IncludeArchives: true})
	if err == nil {
		t.Fatal("expected glob error")
	}
	if !strings.Contains(err.Error(), "export glob") {
		t.Fatalf("err = %v, want export glob", err)
	}
}

func TestExportStatFailsOnDanglingSymlink(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// A dangling symlink matches the archive glob (Glob doesn't resolve
	// targets) but os.Stat on it fails, deterministically hitting the
	// file-list stat-error branch without any race or permission trick.
	if err := os.Symlink(filepath.Join(dir, "nonexistent-target"), filepath.Join(dir, "stdout.log.1")); err != nil {
		t.Fatal(err)
	}
	var buf strings.Builder
	err := logcap.ExportTarGzWithOptions(dir, sinkWriter{&buf}, logcap.ExportOptions{IncludeArchives: true})
	if err == nil {
		t.Fatal("expected stat error")
	}
	if !strings.Contains(err.Error(), "export stat") {
		t.Fatalf("err = %v, want export stat", err)
	}
}

// sinkWriter adapts a strings.Builder to io.Writer for tests that need a
// working (non-failing) sink alongside a deliberately broken dir/pattern.
type sinkWriter struct{ b *strings.Builder }

func (s sinkWriter) Write(p []byte) (int, error) { return s.b.Write(p) }
