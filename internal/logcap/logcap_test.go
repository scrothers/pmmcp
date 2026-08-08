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

package logcap_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scrothers/pmmcp/internal/logcap"
)

func TestNewCreatesDir(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "logs", "proc-1")
	c, err := logcap.New(dir, 1, 3)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if c.Dir != dir {
		t.Fatalf("Dir = %q, want %q", c.Dir, dir)
	}
	if c.MaxFileBytes != 1*1024*1024 {
		t.Fatalf("MaxFileBytes = %d, want 1MiB", c.MaxFileBytes)
	}
	if c.MaxFiles != 3 {
		t.Fatalf("MaxFiles = %d, want 3", c.MaxFiles)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat dir: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("expected directory")
	}
}

func TestNewEmptyDir(t *testing.T) {
	t.Parallel()
	_, err := logcap.New("", 1, 1)
	if err == nil {
		t.Fatal("expected error for empty dir")
	}
}

func TestNewDefaults(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	c, err := logcap.New(dir, 0, 0)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if c.MaxFileBytes != 10*1024*1024 {
		t.Fatalf("MaxFileBytes = %d, want 10MiB default", c.MaxFileBytes)
	}
	if c.MaxFiles != 5 {
		t.Fatalf("MaxFiles = %d, want 5 default", c.MaxFiles)
	}
}

func TestPathsAndOpen(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	c, err := logcap.New(dir, 1, 2)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got, want := c.StdoutPath(), filepath.Join(dir, "stdout.log"); got != want {
		t.Fatalf("StdoutPath = %q, want %q", got, want)
	}
	if got, want := c.StderrPath(), filepath.Join(dir, "stderr.log"); got != want {
		t.Fatalf("StderrPath = %q, want %q", got, want)
	}

	out, err := c.OpenStdout()
	if err != nil {
		t.Fatalf("OpenStdout: %v", err)
	}
	t.Cleanup(func() { _ = out.Close() })
	if _, err := out.WriteString("hello stdout\n"); err != nil {
		t.Fatalf("write stdout: %v", err)
	}

	errf, err := c.OpenStderr()
	if err != nil {
		t.Fatalf("OpenStderr: %v", err)
	}
	t.Cleanup(func() { _ = errf.Close() })
	if _, err := errf.WriteString("hello stderr\n"); err != nil {
		t.Fatalf("write stderr: %v", err)
	}

	// Re-open appends.
	out2, err := c.OpenStdout()
	if err != nil {
		t.Fatalf("OpenStdout re: %v", err)
	}
	t.Cleanup(func() { _ = out2.Close() })
	if _, err := out2.WriteString("line2\n"); err != nil {
		t.Fatalf("write stdout2: %v", err)
	}
	_ = out.Close()
	_ = out2.Close()

	data, err := os.ReadFile(c.StdoutPath())
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(data) != "hello stdout\nline2\n" {
		t.Fatalf("stdout content = %q", data)
	}
}

func TestRotateIfNeeded(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// Small threshold: 64 bytes.
	c := &logcap.Capturer{
		Dir:          dir,
		MaxFileBytes: 64,
		MaxFiles:     2,
	}

	// Undersized: no rotation.
	if err := os.WriteFile(c.StdoutPath(), []byte("small\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := c.RotateIfNeeded(); err != nil {
		t.Fatalf("RotateIfNeeded: %v", err)
	}
	if _, err := os.Stat(c.StdoutPath()); err != nil {
		t.Fatalf("active should remain: %v", err)
	}
	if _, err := os.Stat(c.StdoutPath() + ".1"); !os.IsNotExist(err) {
		t.Fatalf("unexpected .1: %v", err)
	}

	// Oversized stdout.
	big := strings.Repeat("x", 100)
	if err := os.WriteFile(c.StdoutPath(), []byte(big), 0o600); err != nil {
		t.Fatalf("write big: %v", err)
	}
	if err := c.RotateIfNeeded(); err != nil {
		t.Fatalf("RotateIfNeeded big: %v", err)
	}
	if _, err := os.Stat(c.StdoutPath()); !os.IsNotExist(err) {
		t.Fatal("active stdout should be renamed away")
	}
	data, err := os.ReadFile(c.StdoutPath() + ".1")
	if err != nil {
		t.Fatalf("read .1: %v", err)
	}
	if string(data) != big {
		t.Fatalf(".1 content mismatch")
	}

	// Second rotation shifts.1 →.2
	if err := os.WriteFile(c.StdoutPath(), []byte(strings.Repeat("y", 100)), 0o600); err != nil {
		t.Fatalf("write second: %v", err)
	}
	if err := c.RotateIfNeeded(); err != nil {
		t.Fatalf("RotateIfNeeded second: %v", err)
	}
	if _, err := os.Stat(c.StdoutPath() + ".1"); err != nil {
		t.Fatalf("expected .1: %v", err)
	}
	if _, err := os.Stat(c.StdoutPath() + ".2"); err != nil {
		t.Fatalf("expected .2: %v", err)
	}

	// Third rotation drops oldest beyond MaxFiles=2.
	if err := os.WriteFile(c.StdoutPath(), []byte(strings.Repeat("z", 100)), 0o600); err != nil {
		t.Fatalf("write third: %v", err)
	}
	if err := c.RotateIfNeeded(); err != nil {
		t.Fatalf("RotateIfNeeded third: %v", err)
	}
	// Only.1 and.2 should exist; content of former first rotate is gone.
	if _, err := os.Stat(c.StdoutPath() + ".3"); !os.IsNotExist(err) {
		t.Fatal("should not keep .3 when MaxFiles=2")
	}
	d1, _ := os.ReadFile(c.StdoutPath() + ".1")
	if !strings.Contains(string(d1), "z") {
		t.Fatalf(".1 should be newest rotate, got %q", d1)
	}
}

func TestTailLastN(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	content := "line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\nline9\nline10\n"
	if err := os.WriteFile(filepath.Join(dir, "stdout.log"), []byte(content), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := logcap.Tail(dir, logcap.TailOptions{Stream: "stdout", Lines: 3})
	if err != nil {
		t.Fatalf("Tail: %v", err)
	}
	want := "line8\nline9\nline10"
	if got != want {
		t.Fatalf("Tail = %q, want %q", got, want)
	}
}

func TestTailBoth(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "stdout.log"), []byte("out-a\nout-b\n"), 0o600); err != nil {
		t.Fatalf("write stdout: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "stderr.log"), []byte("err-a\n"), 0o600); err != nil {
		t.Fatalf("write stderr: %v", err)
	}

	got, err := logcap.Tail(dir, logcap.TailOptions{Stream: "both", Lines: 10})
	if err != nil {
		t.Fatalf("Tail: %v", err)
	}
	if !strings.Contains(got, "stdout| out-a") || !strings.Contains(got, "stderr| err-a") {
		t.Fatalf("Tail both = %q", got)
	}
}

func TestTailMissingFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	got, err := logcap.Tail(dir, logcap.TailOptions{Stream: "stdout", Lines: 5})
	if err != nil {
		t.Fatalf("Tail: %v", err)
	}
	if got != "" {
		t.Fatalf("empty dir Tail = %q", got)
	}
}

func TestGrepFindsPattern(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	content := "alpha\nbeta match here\ngamma\nMATCH again\ndelta\n"
	if err := os.WriteFile(filepath.Join(dir, "stdout.log"), []byte(content), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := logcap.Grep(dir, logcap.GrepOptions{
		Stream:     "stdout",
		Pattern:    `(?i)match`,
		MaxMatches: 10,
	})
	if err != nil {
		t.Fatalf("Grep: %v", err)
	}
	if !strings.Contains(got, "2:beta match here") {
		t.Fatalf("missing line 2: %q", got)
	}
	if !strings.Contains(got, "4:MATCH again") {
		t.Fatalf("missing line 4: %q", got)
	}
	if strings.Contains(got, "alpha") {
		t.Fatalf("non-match leaked: %q", got)
	}
}

func TestGrepContext(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	content := "a\nb\nHIT\nc\nd\n"
	if err := os.WriteFile(filepath.Join(dir, "stderr.log"), []byte(content), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := logcap.Grep(dir, logcap.GrepOptions{
		Stream:  "stderr",
		Pattern: `HIT`,
		Context: 1,
	})
	if err != nil {
		t.Fatalf("Grep: %v", err)
	}
	if !strings.Contains(got, "2:b") || !strings.Contains(got, "3:HIT") || !strings.Contains(got, "4:c") {
		t.Fatalf("context missing: %q", got)
	}
}

func TestGrepEmptyPattern(t *testing.T) {
	t.Parallel()
	_, err := logcap.Grep(t.TempDir(), logcap.GrepOptions{Pattern: ""})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGrepBadPattern(t *testing.T) {
	t.Parallel()
	_, err := logcap.Grep(t.TempDir(), logcap.GrepOptions{Pattern: `(`})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestErrorsFindsERRORLine(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	content := "info ok\nWARN something\nERROR boom\nmore info\npanic: die\ntrace done\n"
	if err := os.WriteFile(filepath.Join(dir, "stderr.log"), []byte(content), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := logcap.Errors(dir, logcap.ErrorsOptions{Stream: "stderr", Lines: 10})
	if err != nil {
		t.Fatalf("Errors: %v", err)
	}
	if !strings.Contains(got, "3:ERROR boom") {
		t.Fatalf("expected ERROR line: %q", got)
	}
	if !strings.Contains(got, "5:panic: die") {
		t.Fatalf("expected panic line: %q", got)
	}
	if strings.Contains(got, "info ok") {
		t.Fatalf("non-error leaked: %q", got)
	}
}

func TestErrorsTailLimit(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	var b strings.Builder
	for i := 1; i <= 5; i++ {
		b.WriteString("ERROR e")
		b.WriteByte(byte('0' + i))
		b.WriteByte('\n')
	}
	if err := os.WriteFile(filepath.Join(dir, "stdout.log"), []byte(b.String()), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := logcap.Errors(dir, logcap.ErrorsOptions{Stream: "stdout", Lines: 2})
	if err != nil {
		t.Fatalf("Errors: %v", err)
	}
	// Last two: e4, e5
	if !strings.Contains(got, "ERROR e4") || !strings.Contains(got, "ERROR e5") {
		t.Fatalf("want last two errors: %q", got)
	}
	if strings.Contains(got, "ERROR e1") || strings.Contains(got, "ERROR e2") || strings.Contains(got, "ERROR e3") {
		t.Fatalf("older errors should be dropped: %q", got)
	}
}

func TestInvalidStream(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if _, err := logcap.Tail(dir, logcap.TailOptions{Stream: "nope"}); err == nil {
		t.Fatal("Tail expected invalid stream error")
	}
	if _, err := logcap.Grep(dir, logcap.GrepOptions{Stream: "nope", Pattern: "x"}); err == nil {
		t.Fatal("Grep expected invalid stream error")
	}
	if _, err := logcap.Errors(dir, logcap.ErrorsOptions{Stream: "nope"}); err == nil {
		t.Fatal("Errors expected invalid stream error")
	}
}

func TestOutputCap(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// Build content well over 256KiB.
	line := strings.Repeat("A", 200) + "\n"
	var b strings.Builder
	for b.Len() < logcap.DefaultMaxOutputBytes+10_000 {
		b.WriteString(line)
	}
	if err := os.WriteFile(filepath.Join(dir, "stdout.log"), []byte(b.String()), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := logcap.Tail(dir, logcap.TailOptions{Stream: "stdout", Lines: 50_000})
	if err != nil {
		t.Fatalf("Tail: %v", err)
	}
	if len(got) > logcap.DefaultMaxOutputBytes+len("\n... [truncated]\n") {
		t.Fatalf("output too large: %d", len(got))
	}
	if !strings.Contains(got, "[truncated]") {
		t.Fatalf("expected truncation marker, len=%d", len(got))
	}
}

func TestRotateGzip(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	c, err := logcap.New(dir, 0, 2) // defaults: compress on, 10MiB
	if err != nil {
		t.Fatal(err)
	}
	c.MaxFileBytes = 64
	big := strings.Repeat("y", 100)
	if err := os.WriteFile(c.StdoutPath(), []byte(big), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := c.RotateIfNeeded(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(c.StdoutPath() + ".1.gz"); err != nil {
		t.Fatalf("expected gzipped archive: %v", err)
	}
	if _, err := os.Stat(c.StdoutPath() + ".1"); !os.IsNotExist(err) {
		t.Fatal("plain .1 should be removed after gzip")
	}
}
