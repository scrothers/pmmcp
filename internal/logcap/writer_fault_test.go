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

package logcap_test

import (
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"testing"

	"github.com/scrothers/pmmcp/internal/logcap"
)

// withLowFileSizeLimit lowers RLIMIT_FSIZE for the duration of fn and ignores
// SIGXFSZ so an over-limit write returns EFBIG instead of killing the process.
// RotatingWriter.f is a concrete *os.File (not an injectable io.Writer), so
// this is the only portable, non-root way to make its underlying disk writes
// fail deterministically.
//
// RLIMIT_FSIZE is process-wide, so these tests intentionally do NOT call
// t.Parallel(): they run to completion (restoring the limit before
// returning) before the package's t.Parallel() tests are ever unpaused to
// run concurrently, per Go's test scheduling model.
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

func TestRotatingWriterEmitOverCapFlushWriteFails(t *testing.T) {
	dir := t.TempDir()
	c, err := logcap.New(dir, 0, 3)
	if err != nil {
		t.Fatal(err)
	}
	w, err := c.OpenStdoutWriter()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = w.Close() }()

	withLowFileSizeLimit(t, 50, func() {
		// No newline and over maxPartialBytes: Write flushes immediately via
		// emit(), whose underlying io.WriteString exceeds the file-size limit.
		_, werr := io.WriteString(w, strings.Repeat("A", 300*1024))
		if werr == nil {
			t.Fatal("expected write error from RLIMIT_FSIZE")
		}
	})
}

func TestRotatingWriterEmitLineWriteFails(t *testing.T) {
	dir := t.TempDir()
	c, err := logcap.New(dir, 0, 3)
	if err != nil {
		t.Fatal(err)
	}
	w, err := c.OpenStdoutWriter()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = w.Close() }()

	withLowFileSizeLimit(t, 5, func() {
		_, werr := io.WriteString(w, "this complete line exceeds the tiny rlimit\n")
		if werr == nil {
			t.Fatal("expected write error from RLIMIT_FSIZE")
		}
	})
}

func TestRotatingWriterCloseFlushFails(t *testing.T) {
	dir := t.TempDir()
	c, err := logcap.New(dir, 0, 3)
	if err != nil {
		t.Fatal(err)
	}
	w, err := c.OpenStdoutWriter()
	if err != nil {
		t.Fatal(err)
	}
	// Buffer a partial (no-newline) line while the limit is still generous.
	if _, err := io.WriteString(w, "buffered partial, no newline"); err != nil {
		t.Fatalf("buffering write: %v", err)
	}

	withLowFileSizeLimit(t, 5, func() {
		if err := w.Close(); err == nil {
			t.Fatal("expected Close to report the flush failure")
		}
	})
}

func TestRotatingWriterRotateFailsBothRotateAndReopenFail(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root: directory permission bits don't block root")
	}
	dir := t.TempDir()
	c, err := logcap.New(dir, 0, 1) // MaxFiles=1: the oldest-backup slot is base.1
	if err != nil {
		t.Fatal(err)
	}
	c.MaxFileBytes = 32 // tiny cap so the next emit forces a rotation
	w, err := c.OpenStdoutWriter()
	if err != nil {
		t.Fatal(err)
	}
	// Push size past the cap so the next line write triggers emit's rotate().
	if _, err := io.WriteString(w, strings.Repeat("x", 40)+"\n"); err != nil {
		t.Fatal(err)
	}
	// Pre-create the oldest-backup slot so rotateNow's very first step (remove
	// stdout.log.1) targets an existing entry instead of a tolerated no-op.
	if err := os.WriteFile(w.Path()+".1", []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Unlink the active file's directory entry (the open fd stays valid) so
	// the eventual reopen must CREATE a fresh file rather than just re-open
	// an existing one.
	if err := os.Remove(w.Path()); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	// rotateNow fails immediately removing the oldest backup (needs dir write
	// permission), and the reopen also fails needing to create a new file in
	// the same read-only dir: both rotErr and the reopen error are non-nil.
	_, err = io.WriteString(w, "trigger\n")
	if err == nil {
		t.Fatal("expected rotate/reopen failure")
	}
	if !strings.Contains(err.Error(), "remove") {
		t.Fatalf("err = %v, want the rotateNow remove error surfaced", err)
	}
}

func TestRotatingWriterRotateReopenFailsRotateNowSucceeds(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root: directory permission bits don't block root")
	}
	dir := t.TempDir()
	c, err := logcap.New(dir, 0, 2)
	if err != nil {
		t.Fatal(err)
	}
	c.MaxFileBytes = 32
	w, err := c.OpenStdoutWriter()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(w, strings.Repeat("x", 40)+"\n"); err != nil {
		t.Fatal(err)
	}
	// Unlink the active file's directory entry while the fd is still open and
	// the dir is still writable: rotateNow's final rename will find nothing
	// there (its IsNotExist shortcut) and succeed, but the reopen that
	// follows still needs to create a fresh file in what is about to become
	// a read-only directory.
	if err := os.Remove(w.Path()); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	_, err = io.WriteString(w, "trigger\n")
	if err == nil {
		t.Fatal("expected reopen failure")
	}
	if !strings.Contains(err.Error(), "reopen") {
		t.Fatalf("err = %v, want a reopen error", err)
	}
}
