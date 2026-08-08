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

func TestSnapshotProcessesEmptyMap(t *testing.T) {
	t.Parallel()
	snap := observability.SnapshotProcesses(nil)
	if snap.Goroutines <= 0 {
		t.Fatalf("Goroutines = %d, want > 0", snap.Goroutines)
	}
	if len(snap.Processes) != 0 {
		t.Fatalf("Processes len = %d, want 0", len(snap.Processes))
	}
	if snap.At.IsZero() {
		t.Fatal("At is zero")
	}
}

func TestSnapshotProcessesEmptyMapExplicit(t *testing.T) {
	t.Parallel()
	snap := observability.SnapshotProcesses(map[string]int{})
	if snap.Goroutines <= 0 {
		t.Fatalf("Goroutines = %d, want > 0", snap.Goroutines)
	}
}

func TestSnapshotProcessesSelfPID(t *testing.T) {
	t.Parallel()
	pid := os.Getpid()
	snap := observability.SnapshotProcesses(map[string]int{
		"proc-self": pid,
	})
	if len(snap.Processes) != 1 {
		t.Fatalf("len = %d, want 1", len(snap.Processes))
	}
	m := snap.Processes[0]
	if m.ID != "proc-self" || m.PID != pid {
		t.Fatalf("got %+v", m)
	}
	if runtime.GOOS == "linux" && m.RSSBytes == 0 {
		t.Fatalf("RSSBytes = 0 on linux for self pid, want > 0")
	}
	if snap.Goroutines <= 0 {
		t.Fatalf("Goroutines = %d, want > 0", snap.Goroutines)
	}
}
