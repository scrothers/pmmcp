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

package group

import "testing"

// TestCloneGroupPtrNil covers the nil-guard in cloneGroupPtr. No exported
// path passes cloneGroupPtr a nil *Group (every call site checks a map
// lookup first), so this exercises the defensive branch directly.
func TestCloneGroupPtrNil(t *testing.T) {
	t.Parallel()
	if got := cloneGroupPtr(nil); got != nil {
		t.Fatalf("cloneGroupPtr(nil) = %+v, want nil", got)
	}
}

// TestFindCyclePathSkipsNonAliveDeps covers three findCyclePath branches in
// one deterministic call: a dependency edge to a node outside the alive set
// (the !alive[d] continue), a root whose exploration dead-ends without
// finding a cycle (the backtrack/black-marking path), and the final "no
// cycle found" return. These are unreachable through the public API — every
// alive node reached via startOrder's Kahn stall is guaranteed to have an
// alive out-edge — so this constructs the graph directly.
func TestFindCyclePathSkipsNonAliveDeps(t *testing.T) {
	t.Parallel()
	deps := map[string]map[string]struct{}{
		"solo": {"resolved": struct{}{}}, // "resolved" is not in the alive set
	}
	alive := map[string]bool{"solo": true}
	if got := findCyclePath(deps, alive); len(got) != 0 {
		t.Fatalf("findCyclePath = %v, want no cycle found", got)
	}
}

// TestFindCyclePathFindsCycle is a direct sanity check that findCyclePath
// still reports a real cycle when one exists among alive nodes.
func TestFindCyclePathFindsCycle(t *testing.T) {
	t.Parallel()
	deps := map[string]map[string]struct{}{
		"a": {"b": struct{}{}},
		"b": {"a": struct{}{}},
	}
	alive := map[string]bool{"a": true, "b": true}
	got := findCyclePath(deps, alive)
	if len(got) == 0 {
		t.Fatal("findCyclePath: expected a cycle, got none")
	}
}
