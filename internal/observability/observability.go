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

package observability

import (
	"runtime"
	"time"
)

// Snapshot is a point-in-time metrics view of managed processes and the daemon.
type Snapshot struct {
	At         time.Time
	Processes  []ProcMetrics
	Goroutines int
}

// ProcMetrics holds per-process resource counters when available from the OS.
type ProcMetrics struct {
	ID       string
	PID      int
	RSSBytes uint64
	CPUSec   float64
}

// ProcRef identifies a managed process by PID plus the start-time recorded when
// the daemon spawned it. The start-time lets SnapshotVerified detect PID reuse
// before attributing /proc metrics to a recycled PID.
type ProcRef struct {
	// PID is the process id to sample.
	PID int
	// StartTime is /proc/<pid>/stat field 22 (clock ticks since boot) captured
	// at spawn. Zero skips the reuse check (metrics attributed unconditionally).
	StartTime uint64
}

// SnapshotProcesses builds a Snapshot for the given process id → PID map.
// On Linux, RSS is read from /proc/<pid>/statm when possible; otherwise fields are zero.
// Goroutines always reflects the current Go runtime count.
//
// This form cannot detect PID reuse between the caller building the map and the
// /proc read; use SnapshotVerified with recorded start-times when that matters.
func SnapshotProcesses(pids map[string]int) Snapshot {
	refs := make(map[string]ProcRef, len(pids))
	for id, pid := range pids {
		refs[id] = ProcRef{PID: pid}
	}
	return SnapshotVerified(refs)
}

// SnapshotVerified builds a Snapshot, re-verifying that each PID still belongs to
// the expected process via its recorded start-time. On a start-time mismatch
// (the PID was recycled after the caller recorded it) the process's RSS/CPU are
// reported as zero rather than attributed to an unrelated process. A zero
// StartTime skips the check for that entry.
func SnapshotVerified(refs map[string]ProcRef) Snapshot {
	snap := Snapshot{
		At:         time.Now().UTC(),
		Goroutines: runtime.NumGoroutine(),
		Processes:  make([]ProcMetrics, 0, len(refs)),
	}
	for id, ref := range refs {
		m := ProcMetrics{ID: id, PID: ref.PID}
		if ref.PID > 0 && procMatches(ref.PID, ref.StartTime) {
			m.RSSBytes, m.CPUSec = readProcMetrics(ref.PID)
		}
		snap.Processes = append(snap.Processes, m)
	}
	return snap
}
