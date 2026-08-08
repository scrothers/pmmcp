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
	"fmt"
	"os"
	"strconv"
	"strings"
)

// readProcMetrics reads RSS from /proc/<pid>/statm and CPU seconds from /proc/<pid>/stat.
// Missing or unreadable entries yield zeros.
func readProcMetrics(pid int) (rssBytes uint64, cpuSec float64) {
	rssBytes = readRSS(pid)
	cpuSec = readCPUSec(pid)
	return rssBytes, cpuSec
}

func readRSS(pid int) uint64 {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/statm", pid))
	if err != nil {
		return 0
	}
	return parseRSS(data)
}

// parseRSS parses the resident-set-size field (2nd field, in pages) out of a
// /proc/<pid>/statm payload. Split from readRSS so malformed-content branches
// are testable without needing to fabricate a real /proc entry.
func parseRSS(data []byte) uint64 {
	fields := strings.Fields(string(data))
	if len(fields) < 2 {
		return 0
	}
	// Field 1 is resident set size in pages.
	pages, err := strconv.ParseUint(fields[1], 10, 64)
	if err != nil {
		return 0
	}
	return pages * uint64(os.Getpagesize())
}

func readCPUSec(pid int) float64 {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return 0
	}
	return parseCPUSec(string(data))
}

// parseCPUSec parses utime+stime (in seconds) out of a /proc/<pid>/stat
// payload. Split from readCPUSec so malformed-content branches are testable
// without needing to fabricate a real /proc entry.
func parseCPUSec(s string) float64 {
	// comm can contain spaces/parens; split after last ')'.
	idx := strings.LastIndex(s, ")")
	if idx < 0 || idx+2 >= len(s) {
		return 0
	}
	fields := strings.Fields(s[idx+2:])
	// After comm: state(1) … utime(12) stime(13) — 0-based from post-comm fields:
	// field index 11 = utime, 12 = stime (man proc_pid_stat).
	if len(fields) < 13 {
		return 0
	}
	utime, err1 := strconv.ParseUint(fields[11], 10, 64)
	stime, err2 := strconv.ParseUint(fields[12], 10, 64)
	if err1 != nil || err2 != nil {
		return 0
	}
	// USER_HZ is configurable via CONFIG_HZ but is 100 on effectively all
	// mainstream kernels; reading _SC_CLK_TCK would require cgo. On a kernel
	// with a different tick this figure is off by a constant factor.
	const userHZ = 100.0
	return float64(utime+stime) / userHZ
}

// ReadStartTime returns /proc/<pid>/stat field 22 (process start time in clock
// ticks since boot). The daemon records this at spawn so SnapshotVerified can
// detect PID reuse.
func ReadStartTime(pid int) (uint64, error) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return 0, fmt.Errorf("observability: read stat: %w", err)
	}
	return parseStartTime(string(data))
}

func parseStartTime(s string) (uint64, error) {
	// comm can contain spaces/parens; split after last ')'.
	idx := strings.LastIndex(s, ")")
	if idx < 0 || idx+2 >= len(s) {
		return 0, fmt.Errorf("observability: malformed stat")
	}
	fields := strings.Fields(s[idx+2:])
	// Post-comm fields are 0-based from state; starttime is field 22 → index 19.
	if len(fields) < 20 {
		return 0, fmt.Errorf("observability: short stat")
	}
	v, err := strconv.ParseUint(fields[19], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("observability: parse starttime: %w", err)
	}
	return v, nil
}

// procMatches reports whether pid's current start-time matches want. A zero want
// skips the check (returns true); a read failure returns false (fail closed:
// treat an unreadable/gone PID as a mismatch so metrics are not attributed).
func procMatches(pid int, want uint64) bool {
	if want == 0 {
		return true
	}
	got, err := ReadStartTime(pid)
	if err != nil {
		return false
	}
	return got == want
}
