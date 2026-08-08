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

package cli

import (
	"testing"

	"github.com/scrothers/pmmcp/internal/api"
)

func TestToolMethodMethodsInAllMethods(t *testing.T) {
	t.Parallel()
	known := AllMethodsSet()
	for tool, method := range ToolMethod {
		if _, ok := known[method]; !ok {
			t.Errorf("ToolMethod[%q]=%q not present in api.AllMethods", tool, method)
		}
	}
}

func TestToolMethodCount(t *testing.T) {
	t.Parallel()
	// Frozen catalog size (tools/list surface).
	const want = 65
	if got := len(ToolMethod); got != want {
		t.Errorf("len(ToolMethod) = %d, want %d", got, want)
	}
}

func TestCLIOmissionsAreDocumented(t *testing.T) {
	t.Parallel()
	for tool, reason := range IntentionalCLIOmissions {
		if reason == "" {
			t.Errorf("omission %q empty reason", tool)
		}
		if _, ok := ToolMethod[tool]; !ok {
			t.Errorf("omission %q not in ToolMethod", tool)
		}
		if _, ok := cliVerbs[tool]; ok {
			t.Errorf("tool %q is both omitted and has a CLI verb", tool)
		}
	}
}

// TestEveryToolIsDispatchable is the real catalog↔CLI parity check: every
// non-omitted tool must have a CLI verb whose command path Run actually
// dispatches. It fails on drift — a new tool without a verb, or a verb whose
// subcommand is not routed. (The old test only asserted a non-empty string via
// a tool[3:] fallback, so it could never fail.)
func TestEveryToolIsDispatchable(t *testing.T) {
	t.Parallel()
	for tool := range ToolMethod {
		if _, omitted := IntentionalCLIOmissions[tool]; omitted {
			continue
		}
		verb := CommandForTool(tool)
		if verb == "" {
			t.Errorf("tool %q has no CLI verb and is not an intentional omission", tool)
			continue
		}
		if !Dispatchable(verb) {
			t.Errorf("tool %q verb %q is not dispatchable by Run", tool, verb)
		}
	}
}

// TestNonDispatchableVerbFails guards Dispatchable itself against regressing to
// a vacuous always-true.
func TestNonDispatchableVerbFails(t *testing.T) {
	t.Parallel()
	for _, verb := range []string{"", "bogus", "logs bogus", "group frobnicate"} {
		if Dispatchable(verb) {
			t.Errorf("Dispatchable(%q) = true, want false", verb)
		}
	}
}

func TestToolNamesSorted(t *testing.T) {
	t.Parallel()
	names := ToolNames()
	if len(names) != len(ToolMethod) {
		t.Fatalf("ToolNames len %d != ToolMethod len %d", len(names), len(ToolMethod))
	}
	for i := 1; i < len(names); i++ {
		if names[i-1] >= names[i] {
			t.Errorf("ToolNames not sorted at %d: %q >= %q", i, names[i-1], names[i])
		}
	}
	for _, n := range names {
		if _, ok := ToolMethod[n]; !ok {
			t.Errorf("ToolNames entry %q missing from ToolMethod", n)
		}
	}
}

func TestToolDescriptionCoversAll(t *testing.T) {
	t.Parallel()
	for name := range ToolMethod {
		if ToolDescription[name] == "" {
			t.Errorf("ToolDescription missing for %q", name)
		}
	}
}

func TestReverseToolMethod(t *testing.T) {
	t.Parallel()
	rev := ReverseToolMethod()
	for tool, method := range ToolMethod {
		got, ok := rev[method]
		if !ok {
			t.Errorf("ReverseToolMethod missing method %q for tool %q", method, tool)
			continue
		}
		if ToolMethod[got] != method {
			t.Errorf("ReverseToolMethod[%q]=%q maps back to %q", method, got, ToolMethod[got])
		}
	}
}

func TestAllMethodsIncludesHello(t *testing.T) {
	t.Parallel()
	// hello is IPC-only (not a pm_* tool).
	found := false
	for _, m := range api.AllMethods {
		if m == api.MethodHello {
			found = true
			break
		}
	}
	if !found {
		t.Error("api.AllMethods missing MethodHello")
	}
	if _, ok := ToolMethod["pm_hello"]; ok {
		t.Error("pm_hello must not be a catalog tool")
	}
}
