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

//go:build !linux

package observability

// readProcMetrics is a no-op off Linux (RSS/CPU not collected from /proc).
func readProcMetrics(pid int) (rssBytes uint64, cpuSec float64) {
	_ = pid
	return 0, 0
}

// ReadStartTime is unavailable off Linux; it reports zero (no start-time to record).
func ReadStartTime(pid int) (uint64, error) {
	_ = pid
	return 0, nil
}

// procMatches cannot verify off Linux; it accepts (no /proc to consult).
func procMatches(pid int, want uint64) bool {
	_ = pid
	_ = want
	return true
}
