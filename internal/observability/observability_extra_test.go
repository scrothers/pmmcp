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

package observability_test

import (
	"os"
	"runtime"
	"testing"

	"github.com/scrothers/pmmcp/internal/observability"
)

func TestSnapshotSelfCPUPopulated(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "linux" {
		t.Skip("proc metrics only on linux")
	}
	// Burn a little CPU so utime/stime advance past the clock-tick granularity.
	burnCPU()
	snap := observability.SnapshotProcesses(map[string]int{"proc-self": os.Getpid()})
	if len(snap.Processes) != 1 {
		t.Fatalf("len = %d", len(snap.Processes))
	}
	if snap.Processes[0].CPUSec <= 0 {
		t.Fatalf("CPUSec = %v, want > 0 after CPU burn", snap.Processes[0].CPUSec)
	}
}

func TestSnapshotDeadPIDZero(t *testing.T) {
	t.Parallel()
	// A PID that does not exist yields zeros without error or panic.
	snap := observability.SnapshotProcesses(map[string]int{"gone": 2147480000})
	if len(snap.Processes) != 1 {
		t.Fatalf("len = %d", len(snap.Processes))
	}
	m := snap.Processes[0]
	if m.RSSBytes != 0 || m.CPUSec != 0 {
		t.Fatalf("dead pid metrics = %+v, want zero", m)
	}
}

func TestSnapshotVerifiedDetectsPIDReuse(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "linux" {
		t.Skip("proc metrics only on linux")
	}
	pid := os.Getpid()
	start, err := observability.ReadStartTime(pid)
	if err != nil {
		t.Fatalf("ReadStartTime: %v", err)
	}

	// Correct start-time: metrics attributed.
	good := observability.SnapshotVerified(map[string]observability.ProcRef{
		"self": {PID: pid, StartTime: start},
	})
	if good.Processes[0].RSSBytes == 0 {
		t.Fatal("matching start-time should attribute RSS")
	}

	// Wrong start-time (simulated PID reuse): metrics zeroed.
	bad := observability.SnapshotVerified(map[string]observability.ProcRef{
		"self": {PID: pid, StartTime: start + 1},
	})
	if bad.Processes[0].RSSBytes != 0 || bad.Processes[0].CPUSec != 0 {
		t.Fatalf("mismatched start-time should zero metrics, got %+v", bad.Processes[0])
	}
}

func burnCPU() {
	x := 0
	for i := range 50_000_000 {
		x += i % 7
	}
	_ = x
}
