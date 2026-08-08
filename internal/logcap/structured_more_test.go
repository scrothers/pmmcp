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

func TestFilterLevelStreamError(t *testing.T) {
	t.Parallel()
	if _, err := logcap.FilterLevel(t.TempDir(), logcap.StructuredOptions{Stream: "nope"}); err == nil {
		t.Fatal("expected invalid stream error")
	}
}

func TestFilterLevelDefaultMinLevelAndLines(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	content := "info default level\n" +
		"debug: should be dropped\n"
	if err := os.WriteFile(filepath.Join(dir, "stdout.log"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	// MinLevel and Lines both left at their zero values, exercising the
	// "info" default and the defaultTailLines fallback.
	got, err := logcap.FilterLevel(dir, logcap.StructuredOptions{Stream: "stdout"})
	if err != nil {
		t.Fatalf("FilterLevel: %v", err)
	}
	if !strings.Contains(got, "default level") {
		t.Fatalf("expected info-level line kept: %q", got)
	}
	if strings.Contains(got, "should be dropped") {
		t.Fatalf("debug line should be filtered by default info floor: %q", got)
	}
}

func TestFilterLevelBothStreamsPrefixed(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "stdout.log"), []byte("error: out boom\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "stderr.log"), []byte("error: err boom\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := logcap.FilterLevel(dir, logcap.StructuredOptions{Stream: "both", MinLevel: "error", Lines: 10})
	if err != nil {
		t.Fatalf("FilterLevel: %v", err)
	}
	if !strings.Contains(got, "stdout| error: out boom") || !strings.Contains(got, "stderr| error: err boom") {
		t.Fatalf("expected both-stream prefixes: %q", got)
	}
}

func TestFilterLevelReadError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// A directory named "stdout.log" makes the underlying read fail with a
	// non-EOF error (reading a directory returns EISDIR), deterministically
	// exercising FilterLevel's readAllLines-error propagation without any
	// permission games.
	if err := os.Mkdir(filepath.Join(dir, "stdout.log"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := logcap.FilterLevel(dir, logcap.StructuredOptions{Stream: "stdout"}); err == nil {
		t.Fatal("expected read error when stdout.log is a directory")
	}
}

func TestLineLevelRankLvlField(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	content := `{"lvl":"error","msg":"boom"}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, "stdout.log"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := logcap.FilterLevel(dir, logcap.StructuredOptions{Stream: "stdout", MinLevel: "error", Lines: 10})
	if err != nil {
		t.Fatalf("FilterLevel: %v", err)
	}
	if !strings.Contains(got, "boom") {
		t.Fatalf("expected \"lvl\" field to classify as error: %q", got)
	}
}

func TestLineLevelRankInvalidJSONAndNoLevelKey(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	content := "{not valid json\n" +
		`{"msg":"no level field here"}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, "stdout.log"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	// Neither line carries a recognizable level, so both default to "info"
	// and should surface when filtering at the "info" floor.
	got, err := logcap.FilterLevel(dir, logcap.StructuredOptions{Stream: "stdout", MinLevel: "info", Lines: 10})
	if err != nil {
		t.Fatalf("FilterLevel: %v", err)
	}
	if !strings.Contains(got, "not valid json") {
		t.Fatalf("malformed JSON line should default to info: %q", got)
	}
	if !strings.Contains(got, "no level field here") {
		t.Fatalf("JSON without a level key should default to info: %q", got)
	}
}
