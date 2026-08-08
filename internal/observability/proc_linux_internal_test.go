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

//go:build linux

package observability

import (
	"os"
	"strings"
	"testing"
)

func TestParseStartTime(t *testing.T) {
	t.Parallel()
	// comm contains a space and parens; starttime is field 22 → post-comm index 19.
	// Post-comm fields, 0-based from state: index 19 is starttime (field 22).
	fields := make([]string, 25)
	for i := range fields {
		fields[i] = "0"
	}
	fields[0] = "R"       // state
	fields[11] = "100"    // utime
	fields[12] = "200"    // stime
	fields[19] = "987654" // starttime
	stat := "1000 (pmmcp test) " + strings.Join(fields, " ") + "\n"
	got, err := parseStartTime(stat)
	if err != nil {
		t.Fatalf("parseStartTime: %v", err)
	}
	if got != 987654 {
		t.Fatalf("starttime = %d, want 987654", got)
	}
}

func TestParseStartTimeMalformed(t *testing.T) {
	t.Parallel()
	if _, err := parseStartTime("no paren here"); err == nil {
		t.Fatal("expected error for malformed stat")
	}
	if _, err := parseStartTime("1 (c) R 1 2 3"); err == nil {
		t.Fatal("expected error for short stat")
	}
	fields := make([]string, 25)
	for i := range fields {
		fields[i] = "0"
	}
	fields[19] = "notanumber" // starttime
	bad := "1 (c) " + strings.Join(fields, " ")
	if _, err := parseStartTime(bad); err == nil {
		t.Fatal("expected error for non-numeric starttime")
	}
}

func TestProcMatchesZeroSkips(t *testing.T) {
	t.Parallel()
	// want == 0 skips the check regardless of pid.
	if !procMatches(999999999, 0) {
		t.Fatal("zero want should skip check and return true")
	}
}

// nonexistentPID is unlikely to exist under the default pid_max, so /proc
// reads against it deterministically fail.
const nonexistentPID = 1 << 30

func TestProcMatchesReadErrorFailsClosed(t *testing.T) {
	t.Parallel()
	if procMatches(nonexistentPID, 42) {
		t.Fatal("unreadable pid with nonzero want should fail closed")
	}
}

func TestProcMatchesMismatch(t *testing.T) {
	t.Parallel()
	pid := os.Getpid()
	start, err := ReadStartTime(pid)
	if err != nil {
		t.Fatalf("ReadStartTime: %v", err)
	}
	if procMatches(pid, start+1) {
		t.Fatal("mismatched start-time should not match")
	}
}

func TestReadStartTimeMissingPID(t *testing.T) {
	t.Parallel()
	if _, err := ReadStartTime(nonexistentPID); err == nil {
		t.Fatal("expected error reading start time of a nonexistent pid")
	}
}

func TestReadRSSMissingPID(t *testing.T) {
	t.Parallel()
	if got := readRSS(nonexistentPID); got != 0 {
		t.Fatalf("readRSS(missing) = %d, want 0", got)
	}
}

func TestReadCPUSecMissingPID(t *testing.T) {
	t.Parallel()
	if got := readCPUSec(nonexistentPID); got != 0 {
		t.Fatalf("readCPUSec(missing) = %f, want 0", got)
	}
}

func TestParseRSS(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		data string
		want uint64
	}{
		{"too few fields", "12345", 0},
		{"bad number", "12345 notanumber 1 1 1 1 1", 0},
		{"valid", "100 250 1 1 1 1 1", 250 * uint64(pageSizeForTest())},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := parseRSS([]byte(tt.data)); got != tt.want {
				t.Fatalf("parseRSS(%q) = %d, want %d", tt.data, got, tt.want)
			}
		})
	}
}

func TestParseCPUSec(t *testing.T) {
	t.Parallel()
	fields := make([]string, 20)
	for i := range fields {
		fields[i] = "0"
	}
	fields[0] = "R"    // state
	fields[11] = "300" // utime
	fields[12] = "400" // stime
	valid := "1 (ok) " + strings.Join(fields, " ")

	badUtime := append([]string(nil), fields...)
	badUtime[11] = "notanumber"

	tests := []struct {
		name string
		stat string
		want float64
	}{
		{"no paren", "no paren here", 0},
		{"short after paren", "1 (c)", 0},
		{"too few post-comm fields", "1 (c) R 1 2 3", 0},
		{"bad utime", "1 (c) " + strings.Join(badUtime, " "), 0},
		{"valid", valid, 7}, // (300+400)/100
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := parseCPUSec(tt.stat); got != tt.want {
				t.Fatalf("parseCPUSec(%q) = %v, want %v", tt.stat, got, tt.want)
			}
		})
	}
}

func pageSizeForTest() int {
	return os.Getpagesize()
}
