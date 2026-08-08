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
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadAllLinesTruncatesOverlong(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	long := strings.Repeat("Z", maxLineBytes+4096)
	content := "before\n" + long + "\nafter\n"
	path := filepath.Join(dir, "stdout.log")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	lines, err := readAllLines(path)
	if err != nil {
		t.Fatalf("readAllLines: %v", err)
	}
	if len(lines) != 3 {
		t.Fatalf("got %d lines, want 3", len(lines))
	}
	if lines[0] != "before" || lines[2] != "after" {
		t.Fatalf("neighbors mangled: %q / %q", lines[0], lines[2])
	}
	if !strings.HasSuffix(lines[1], lineTruncatedMarker) {
		t.Fatalf("long line not marked truncated: ...%q", lines[1][len(lines[1])-40:])
	}
	if len(lines[1]) > maxLineBytes+len(lineTruncatedMarker) {
		t.Fatalf("truncated line too long: %d", len(lines[1]))
	}
}

func TestStreamLabelUnknownName(t *testing.T) {
	t.Parallel()
	// streamLabel is only ever called by the package with stdoutName/
	// stderrName in practice (via streamFiles), so the default case is
	// reachable only through a direct unexported call.
	if got := streamLabel("weird.log"); got != "weird.log" {
		t.Fatalf("streamLabel(unknown) = %q, want the name unchanged", got)
	}
}

func TestReadAllLinesNoTrailingNewline(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "stdout.log")
	if err := os.WriteFile(path, []byte("a\nb\nc"), 0o600); err != nil {
		t.Fatal(err)
	}
	lines, err := readAllLines(path)
	if err != nil {
		t.Fatalf("readAllLines: %v", err)
	}
	if len(lines) != 3 || lines[2] != "c" {
		t.Fatalf("lines = %q", lines)
	}
}
