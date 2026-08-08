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

func TestTailDefaultLines(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	content := "l1\nl2\nl3\n"
	if err := os.WriteFile(filepath.Join(dir, "stdout.log"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	// Lines left at its zero value exercises the defaultTailLines fallback.
	got, err := logcap.Tail(dir, logcap.TailOptions{Stream: "stdout"})
	if err != nil {
		t.Fatalf("Tail: %v", err)
	}
	if got != "l1\nl2\nl3" {
		t.Fatalf("Tail default lines = %q", got)
	}
}

func TestTailReadError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "stdout.log"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := logcap.Tail(dir, logcap.TailOptions{Stream: "stdout"}); err == nil {
		t.Fatal("expected read error when stdout.log is a directory")
	}
}

func TestTailOpenPermissionDenied(t *testing.T) {
	t.Parallel()
	if os.Getuid() == 0 {
		t.Skip("running as root: directory permission bits don't block root")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "stdout.log"), []byte("x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	_, err := logcap.Tail(dir, logcap.TailOptions{Stream: "stdout"})
	if err == nil {
		t.Fatal("expected a non-not-exist open error")
	}
	if !strings.Contains(err.Error(), "open") {
		t.Fatalf("err = %v, want an open error", err)
	}
}

func TestGrepNegativeContextClampedToZero(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "stdout.log"), []byte("a\nHIT\nb\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := logcap.Grep(dir, logcap.GrepOptions{Stream: "stdout", Pattern: "HIT", Context: -5})
	if err != nil {
		t.Fatalf("Grep: %v", err)
	}
	if got != "2:HIT" {
		t.Fatalf("Grep with negative context = %q, want just the match line", got)
	}
}

func TestGrepReadError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "stdout.log"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := logcap.Grep(dir, logcap.GrepOptions{Stream: "stdout", Pattern: "x"}); err == nil {
		t.Fatal("expected read error when stdout.log is a directory")
	}
}

func TestGrepCollectionStopsAtMaxMatches(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	content := "HIT\nHIT\nHIT\nHIT\n"
	if err := os.WriteFile(filepath.Join(dir, "stdout.log"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := logcap.Grep(dir, logcap.GrepOptions{Stream: "stdout", Pattern: "HIT", MaxMatches: 2})
	if err != nil {
		t.Fatalf("Grep: %v", err)
	}
	if strings.Count(got, "HIT") != 2 {
		t.Fatalf("Grep with MaxMatches=2 = %q, want exactly 2 hits", got)
	}
}

func TestGrepBothOneStreamNoMatches(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "stdout.log"), []byte("nothing here\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "stderr.log"), []byte("HIT here\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := logcap.Grep(dir, logcap.GrepOptions{Stream: "both", Pattern: "HIT"})
	if err != nil {
		t.Fatalf("Grep: %v", err)
	}
	if !strings.Contains(got, "stderr| 1:HIT here") {
		t.Fatalf("expected stderr match: %q", got)
	}
	if strings.Contains(got, "stdout|") {
		t.Fatalf("stdout should have contributed nothing: %q", got)
	}
}

func TestGrepBothStopsAtMaxMatchesBeforeSecondStream(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "stdout.log"), []byte("HIT out\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "stderr.log"), []byte("HIT err\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := logcap.Grep(dir, logcap.GrepOptions{Stream: "both", Pattern: "HIT", MaxMatches: 1})
	if err != nil {
		t.Fatalf("Grep: %v", err)
	}
	if !strings.Contains(got, "stdout| 1:HIT out") {
		t.Fatalf("expected stdout match kept: %q", got)
	}
	if strings.Contains(got, "stderr") {
		t.Fatalf("stderr should have been skipped once MaxMatches was reached: %q", got)
	}
}

func TestGrepOverlappingContextWindowsMerge(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// Two matches close enough (with a large context) that their windows fully
	// overlap: exercises start/end clamping at file boundaries and the
	// "already emitted" skip on the second window.
	content := "HITa\nmid1\nmid2\nHITb\n"
	if err := os.WriteFile(filepath.Join(dir, "stdout.log"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := logcap.Grep(dir, logcap.GrepOptions{Stream: "stdout", Pattern: "HIT", Context: 5})
	if err != nil {
		t.Fatalf("Grep: %v", err)
	}
	if strings.Contains(got, "--") {
		t.Fatalf("overlapping windows should not be separated: %q", got)
	}
	if !strings.Contains(got, "1:HITa") || !strings.Contains(got, "4:HITb") {
		t.Fatalf("expected both matches present: %q", got)
	}
}

func TestGrepNonOverlappingContextWindowsAreSeparated(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	lines := make([]string, 0, 11)
	lines = append(lines, "HITfirst")
	for range 9 {
		lines = append(lines, "filler")
	}
	lines = append(lines, "HITlast")
	content := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(dir, "stdout.log"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := logcap.Grep(dir, logcap.GrepOptions{Stream: "stdout", Pattern: "HIT", Context: 1})
	if err != nil {
		t.Fatalf("Grep: %v", err)
	}
	if !strings.Contains(got, "--") {
		t.Fatalf("expected a separator between non-overlapping windows: %q", got)
	}
}

func TestErrorsDefaultLimit(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "stdout.log"), []byte("ERROR one\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := logcap.Errors(dir, logcap.ErrorsOptions{Stream: "stdout"})
	if err != nil {
		t.Fatalf("Errors: %v", err)
	}
	if !strings.Contains(got, "ERROR one") {
		t.Fatalf("Errors default limit = %q", got)
	}
}

func TestErrorsReadError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "stdout.log"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := logcap.Errors(dir, logcap.ErrorsOptions{Stream: "stdout"}); err == nil {
		t.Fatal("expected read error when stdout.log is a directory")
	}
}

func TestReadOverBufferSizeLineNotTruncated(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// 100KiB: bigger than bufio's internal 64KiB buffer but well under
	// maxLineBytes (1MiB), forcing at least one bufio.ErrBufferFull
	// continuation without triggering the overlong-line truncation path.
	long := strings.Repeat("Q", 100*1024)
	content := "before\n" + long + "\nafter\n"
	if err := os.WriteFile(filepath.Join(dir, "stdout.log"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := logcap.Tail(dir, logcap.TailOptions{Stream: "stdout", Lines: 10})
	if err != nil {
		t.Fatalf("Tail: %v", err)
	}
	if !strings.Contains(got, long) {
		t.Fatal("100KiB line should be preserved in full, not truncated")
	}
	if strings.Contains(got, "[line truncated]") {
		t.Fatalf("100KiB line under maxLineBytes should not be marked truncated: %q", got[:80])
	}
}

func TestReadStripsCRLF(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	content := "line one\r\nline two\r\n"
	if err := os.WriteFile(filepath.Join(dir, "stdout.log"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := logcap.Tail(dir, logcap.TailOptions{Stream: "stdout", Lines: 10})
	if err != nil {
		t.Fatalf("Tail: %v", err)
	}
	if strings.Contains(got, "\r") {
		t.Fatalf("carriage returns should be stripped: %q", got)
	}
	if got != "line one\nline two" {
		t.Fatalf("got = %q", got)
	}
}

func TestOutputCapTruncatesAtLineBoundary(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// Distinct per-line content (not a uniform fill byte) so a mid-line cut
	// would be detectable as a partial "LINE-<n>" fragment.
	var b strings.Builder
	n := 0
	for b.Len() < logcap.DefaultMaxOutputBytes+50_000 {
		b.WriteString("LINE-")
		b.WriteString(strings.Repeat("x", 490))
		b.WriteByte('\n')
		n++
	}
	if err := os.WriteFile(filepath.Join(dir, "stdout.log"), []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := logcap.Tail(dir, logcap.TailOptions{Stream: "stdout", Lines: n + 10})
	if err != nil {
		t.Fatalf("Tail: %v", err)
	}
	prefix := strings.TrimSuffix(got, "\n... [truncated]\n")
	for _, ln := range strings.Split(prefix, "\n") {
		if ln == "" {
			continue
		}
		if !strings.HasPrefix(ln, "LINE-") || len(ln) != 495 {
			t.Fatalf("truncation cut mid-line, found partial line %q", ln)
		}
	}
}
