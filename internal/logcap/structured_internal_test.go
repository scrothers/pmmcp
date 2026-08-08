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

import "testing"

func TestLineLevelRankDeterministicMultiMarker(t *testing.T) {
	t.Parallel()
	// A line carrying both an error prefix and a lower-severity marker must
	// classify as the most severe (error), stably across runs.
	line := "error: recovered from level=warn state"
	want := LevelOrder["error"]
	for range 50 {
		if got := lineLevelRank(line); got != want {
			t.Fatalf("lineLevelRank = %d, want %d (non-deterministic)", got, want)
		}
	}
}

func TestLineLevelRankJSONAndDefault(t *testing.T) {
	t.Parallel()
	if got := lineLevelRank(`{"level":"panic","msg":"x"}`); got != LevelOrder["panic"] {
		t.Fatalf("json panic rank = %d", got)
	}
	if got := lineLevelRank("plain text with no marker"); got != LevelOrder["info"] {
		t.Fatalf("default rank = %d, want info", got)
	}
}
