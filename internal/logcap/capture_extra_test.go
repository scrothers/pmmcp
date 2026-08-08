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
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scrothers/pmmcp/internal/logcap"
	"github.com/scrothers/pmmcp/internal/secret"
)

func TestRedactWriterRedactsKeyAndCapsPartial(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	w := &logcap.RedactWriter{W: &buf}
	// Key-name redaction flows through secret.RedactLine (hermetic; no global state).
	if _, err := io.WriteString(w, "DB_TOKEN=abcsecret\n"); err != nil {
		t.Fatalf("write: %v", err)
	}
	if out := buf.String(); strings.Contains(out, "abcsecret") || !strings.Contains(out, "REDACTED") {
		t.Fatalf("key-name redaction failed: %q", out)
	}

	// Newline-free run beyond the cap must be flushed, not buffered forever.
	buf.Reset()
	if _, err := io.WriteString(w, strings.Repeat("A", 300*1024)); err != nil {
		t.Fatalf("write big: %v", err)
	}
	if buf.Len() == 0 {
		t.Fatal("over-cap partial line was not flushed")
	}
}

func TestRedactWriterScrubsRegisteredValue(t *testing.T) {
	// Registers a value into the secret package default (a package global), so no
	// t.Parallel(). Proves the daemon's RegisterNamedValue wiring reaches the
	// capture write path via secret.RedactLine.
	const val = "zq7-logcap-registered-value-marker"
	secret.RegisterValue(val)
	var buf bytes.Buffer
	w := &logcap.RedactWriter{W: &buf}
	if _, err := io.WriteString(w, "leaked "+val+" in a url\n"); err != nil {
		t.Fatalf("write: %v", err)
	}
	if out := buf.String(); strings.Contains(out, val) {
		t.Fatalf("registered value not scrubbed: %q", out)
	}
}

func TestRotatingWriterPreservesDataAcrossRotation(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	c, err := logcap.New(dir, 0, 5)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	c.MaxFileBytes = 1024 // force frequent rotation

	w, err := c.OpenStdoutWriter()
	if err != nil {
		t.Fatalf("OpenStdoutWriter: %v", err)
	}
	const n = 500
	for i := range n {
		if _, err := io.WriteString(w, "line-"+itoa(i)+"\n"); err != nil {
			t.Fatalf("write %d: %v", i, err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Reassemble every retained file (active + rotated .gz) and confirm no line lost.
	got := collectLines(t, dir, "stdout.log")
	for i := range n {
		if !got["line-"+itoa(i)] {
			t.Fatalf("line-%d lost across rotation", i)
		}
	}
	// Rotation must actually have happened.
	if matches, _ := filepath.Glob(filepath.Join(dir, "stdout.log.*")); len(matches) == 0 {
		t.Fatal("expected rotated segments")
	}
}

func TestExportIncludesArchivesAndManifest(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "stdout.log"), []byte("active\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "stdout.log.1.gz"), []byte("archived"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "stderr.log"), []byte("err\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := logcap.ExportTarGz(dir, &buf); err != nil {
		t.Fatalf("ExportTarGz: %v", err)
	}
	names := tarEntries(t, buf.Bytes())
	for _, want := range []string{"manifest.json", "stdout.log", "stdout.log.1.gz", "stderr.log"} {
		if !names[want] {
			t.Fatalf("export missing %q; got %v", want, names)
		}
	}
}

func TestTailDoesNotFailOnOverlongLine(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	long := strings.Repeat("Z", 2*1024*1024) // > 1MiB
	content := "before\n" + long + "\nafter\n"
	if err := os.WriteFile(filepath.Join(dir, "stdout.log"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	// Previously a >1MiB line made the whole read return bufio.ErrTooLong; now it
	// degrades (line-level truncation) instead of failing the query.
	if _, err := logcap.Tail(dir, logcap.TailOptions{Stream: "stdout", Lines: 10}); err != nil {
		t.Fatalf("Tail must not fail on long line: %v", err)
	}
}

func TestRedactWriterCloseFlushesPartial(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	w := &logcap.RedactWriter{W: &buf}
	// No trailing newline: content stays in the partial buffer until Close.
	if _, err := io.WriteString(w, "tail line PASSWORD=leakme"); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	out := buf.String()
	if out == "" {
		t.Fatal("Close did not flush partial line")
	}
	if strings.Contains(out, "leakme") {
		t.Fatalf("partial flush left secret: %q", out)
	}
}

func TestRotatingWriterStderrPartialFlushed(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	c, err := logcap.New(dir, 0, 3)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	w, err := c.OpenStderrWriter()
	if err != nil {
		t.Fatalf("OpenStderrWriter: %v", err)
	}
	if got := w.Path(); got != filepath.Join(dir, "stderr.log") {
		t.Fatalf("Path = %q", got)
	}
	// Mid-line KEY=value, no trailing newline: redacted and flushed on Close.
	if _, err := io.WriteString(w, "partial API_KEY=shhsecretval"); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "stderr.log"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("partial not flushed to file on Close")
	}
	if strings.Contains(string(data), "shhsecretval") {
		t.Fatalf("secret survived in file: %q", data)
	}
}

func TestRotatingWriterLargeNoNewline(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	c, err := logcap.New(dir, 0, 3)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	c.MaxFileBytes = 128 * 1024 // small cap so the flushed chunk forces a rotation
	w, err := c.OpenStdoutWriter()
	if err != nil {
		t.Fatalf("OpenStdoutWriter: %v", err)
	}
	// A newline-free run beyond maxPartialBytes must be flushed, not buffered.
	if _, err := io.WriteString(w, strings.Repeat("q", 300*1024)); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := io.WriteString(w, "\ntrailer\n"); err != nil {
		t.Fatalf("write2: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	total := 0
	matches, _ := filepath.Glob(filepath.Join(dir, "stdout.log*"))
	for _, p := range matches {
		fi, _ := os.Stat(p)
		total += int(fi.Size())
	}
	if total == 0 {
		t.Fatal("nothing written")
	}
}

func TestGrepBothWithContext(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "stdout.log"), []byte("a\nHIT one\nb\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "stderr.log"), []byte("c\nHIT two\nd\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := logcap.Grep(dir, logcap.GrepOptions{Stream: "both", Pattern: "HIT", Context: 1, MaxMatches: 10})
	if err != nil {
		t.Fatalf("Grep: %v", err)
	}
	if !strings.Contains(got, "stdout| 2:HIT one") || !strings.Contains(got, "stderr| 2:HIT two") {
		t.Fatalf("both-stream grep missing matches: %q", got)
	}
}

func TestErrorsBoth(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "stdout.log"), []byte("ok\npanic: out\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "stderr.log"), []byte("ERROR err\nfine\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := logcap.Errors(dir, logcap.ErrorsOptions{Stream: "both", Lines: 10})
	if err != nil {
		t.Fatalf("Errors: %v", err)
	}
	if !strings.Contains(got, "stdout| 2:panic: out") || !strings.Contains(got, "stderr| 1:ERROR err") {
		t.Fatalf("both-stream errors missing: %q", got)
	}
}

func TestExportNoArchivesWithMeta(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "stdout.log"), []byte("a\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "stdout.log.1.gz"), []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	err := logcap.ExportTarGzWithOptions(dir, &buf, logcap.ExportOptions{
		IncludeArchives: false,
		Meta:            map[string]string{"id": "proc-1"},
	})
	if err != nil {
		t.Fatalf("ExportTarGzWithOptions: %v", err)
	}
	names := tarEntries(t, buf.Bytes())
	if !names["manifest.json"] || !names["stdout.log"] {
		t.Fatalf("missing expected entries: %v", names)
	}
	if names["stdout.log.1.gz"] {
		t.Fatal("archive included despite IncludeArchives=false")
	}
}

func TestFilterLevel(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	content := "info starting\n" +
		`{"level":"error","msg":"boom"}` + "\n" +
		"level=warn slow\n" +
		"error: hard failure\n"
	if err := os.WriteFile(filepath.Join(dir, "stdout.log"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := logcap.FilterLevel(dir, logcap.StructuredOptions{Stream: "stdout", MinLevel: "error", Lines: 100})
	if err != nil {
		t.Fatalf("FilterLevel: %v", err)
	}
	if !strings.Contains(got, "boom") || !strings.Contains(got, "hard failure") {
		t.Fatalf("error lines missing: %q", got)
	}
	if strings.Contains(got, "starting") || strings.Contains(got, "slow") {
		t.Fatalf("sub-threshold lines leaked: %q", got)
	}
	if _, err := logcap.FilterLevel(dir, logcap.StructuredOptions{MinLevel: "bogus"}); err == nil {
		t.Fatal("expected error for unknown min_level")
	}
}

func TestWriterErrorPaths(t *testing.T) {
	t.Parallel()
	// nil capturer
	var nc *logcap.Capturer
	if _, err := nc.OpenStdoutWriter(); err == nil {
		t.Fatal("nil capturer OpenStdoutWriter should error")
	}
	if err := nc.RotateIfNeeded(); err == nil {
		t.Fatal("nil capturer RotateIfNeeded should error")
	}
	// Dir that is actually a file → open fails.
	f := filepath.Join(t.TempDir(), "notadir")
	if err := os.WriteFile(f, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	bad := &logcap.Capturer{Dir: f, MaxFileBytes: 1024}
	if _, err := bad.OpenStdoutWriter(); err == nil {
		t.Fatal("OpenStdoutWriter under a file path should error")
	}
}

func TestRotatingWriterWriteAfterClose(t *testing.T) {
	t.Parallel()
	c, err := logcap.New(t.TempDir(), 0, 2)
	if err != nil {
		t.Fatal(err)
	}
	w, err := c.OpenStdoutWriter()
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("second Close should be nil: %v", err)
	}
	if _, err := io.WriteString(w, "x\n"); err == nil {
		t.Fatal("write after close should error")
	}
}

func TestRedactWriterNilSink(t *testing.T) {
	t.Parallel()
	w := &logcap.RedactWriter{}
	if _, err := io.WriteString(w, "x\n"); err == nil {
		t.Fatal("nil sink write should error")
	}
	if err := w.Close(); err != nil {
		t.Fatalf("nil sink Close = %v, want nil", err)
	}
}

// helpers

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b [20]byte
	pos := len(b)
	for i > 0 {
		pos--
		b[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(b[pos:])
}

func collectLines(t *testing.T, dir, base string) map[string]bool {
	t.Helper()
	got := make(map[string]bool)
	matches, _ := filepath.Glob(filepath.Join(dir, base+"*"))
	for _, path := range matches {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if strings.HasSuffix(path, ".gz") {
			zr, err := gzip.NewReader(bytes.NewReader(data))
			if err != nil {
				t.Fatalf("gzip %s: %v", path, err)
			}
			data, err = io.ReadAll(zr)
			if err != nil {
				t.Fatalf("gunzip %s: %v", path, err)
			}
			_ = zr.Close()
		}
		for _, ln := range strings.Split(string(data), "\n") {
			if ln != "" {
				got[ln] = true
			}
		}
	}
	return got
}

func tarEntries(t *testing.T, gztar []byte) map[string]bool {
	t.Helper()
	zr, err := gzip.NewReader(bytes.NewReader(gztar))
	if err != nil {
		t.Fatalf("gzip: %v", err)
	}
	defer zr.Close()
	names := make(map[string]bool)
	tr := tar.NewReader(zr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("tar: %v", err)
		}
		names[hdr.Name] = true
	}
	return names
}
