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

package group_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/scrothers/pmmcp/internal/group"
	"github.com/scrothers/pmmcp/internal/id"
)

func mustGroupID(t *testing.T) string {
	t.Helper()
	gid, err := id.New(id.Group)
	if err != nil {
		t.Fatalf("id.New: %v", err)
	}
	return gid
}

// TestDuplicateDependsOn is the regression for the indegree-corruption bug:
// duplicate depends_on entries must not let a dependent start before all its
// dependencies.
func TestDuplicateDependsOn(t *testing.T) {
	t.Parallel()
	r := group.NewRegistry()
	g, err := r.Create(group.Group{
		Name: "dup",
		Members: []group.Member{
			{Name: "m", DependsOn: []string{"B", "B", "C"}},
			{Name: "B"},
			{Name: "C"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	order, err := r.StartOrder(g.ID)
	if err != nil {
		t.Fatal(err)
	}
	pos := map[string]int{}
	for i, n := range order {
		pos[n] = i
	}
	if pos["m"] < pos["B"] || pos["m"] < pos["C"] {
		t.Fatalf("start order %v: m must come after B and C", order)
	}
}

func TestSiblingInputOrderPreserved(t *testing.T) {
	t.Parallel()
	r := group.NewRegistry()
	g, err := r.Create(group.Group{
		Name: "siblings",
		Members: []group.Member{
			{Name: "c"},
			{Name: "a"},
			{Name: "b"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	order, err := r.StartOrder(g.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"c", "a", "b"}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("order %v, want %v (input order preserved)", order, want)
		}
	}
}

func TestMemberOrderTieBreak(t *testing.T) {
	t.Parallel()
	r := group.NewRegistry()
	g, err := r.Create(group.Group{
		Name: "explicit",
		Members: []group.Member{
			{Name: "x", Order: 2},
			{Name: "y", Order: 0},
			{Name: "z", Order: 1},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	order, err := r.StartOrder(g.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"y", "z", "x"}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("order %v, want %v (Member.Order honored)", order, want)
		}
	}
}

func TestDiamondOrder(t *testing.T) {
	t.Parallel()
	// d depends on b and c; both depend on a. Siblings b,c ordered by input.
	r := group.NewRegistry()
	g, err := r.Create(group.Group{
		Name: "diamond",
		Members: []group.Member{
			{Name: "d", DependsOn: []string{"b", "c"}},
			{Name: "b", DependsOn: []string{"a"}},
			{Name: "c", DependsOn: []string{"a"}},
			{Name: "a"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	order, err := r.StartOrder(g.ID)
	if err != nil {
		t.Fatal(err)
	}
	pos := map[string]int{}
	for i, n := range order {
		pos[n] = i
	}
	if pos["a"] != 0 {
		t.Fatalf("a must start first: %v", order)
	}
	if pos["b"] > pos["c"] {
		t.Fatalf("siblings b,c must keep input order: %v", order)
	}
	if pos["d"] != 3 {
		t.Fatalf("d must start last: %v", order)
	}
}

func TestListDeterministic(t *testing.T) {
	t.Parallel()
	r := group.NewRegistry()
	ids := make([]string, 0, 3)
	for _, name := range []string{"a", "b", "c"} {
		g, err := r.Create(group.Group{Name: name, ProjectID: "p"})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, g.ID)
	}
	first := r.List("p")
	for range 5 {
		got := r.List("p")
		if len(got) != len(first) {
			t.Fatalf("list length varies")
		}
		for i := range got {
			if got[i].ID != first[i].ID {
				t.Fatalf("list order not stable: %v vs %v", got, first)
			}
		}
	}
	_ = ids
}

func TestCreateInvalidID(t *testing.T) {
	t.Parallel()
	r := group.NewRegistry()
	if _, err := r.Create(group.Group{ID: "proc-123", Name: "bad"}); err == nil {
		t.Fatal("expected error for non-grp id prefix")
	}
	if _, err := r.Create(group.Group{ID: "not-an-id", Name: "bad2"}); err == nil {
		t.Fatal("expected error for malformed id")
	}
	// A valid grp- ULID is accepted verbatim.
	if _, err := r.Create(group.Group{ID: mustGroupID(t), Name: "ok"}); err != nil {
		t.Fatalf("valid grp- id rejected: %v", err)
	}
}

func TestErrExists(t *testing.T) {
	t.Parallel()
	r := group.NewRegistry()
	gid := mustGroupID(t)
	if _, err := r.Create(group.Group{ID: gid, Name: "one"}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Create(group.Group{ID: gid, Name: "two"}); !errors.Is(err, group.ErrExists) {
		t.Fatalf("err = %v, want ErrExists", err)
	}
}

func TestCyclePathReported(t *testing.T) {
	t.Parallel()
	// A<->B with C depending on A: cycle path should include A and B, not C.
	r := group.NewRegistry()
	_, err := r.Create(group.Group{
		Name: "cyclic",
		Members: []group.Member{
			{Name: "A", DependsOn: []string{"B"}},
			{Name: "B", DependsOn: []string{"A"}},
			{Name: "C", DependsOn: []string{"A"}},
		},
	})
	if !errors.Is(err, group.ErrCycle) {
		t.Fatalf("err = %v, want ErrCycle", err)
	}
	msg := err.Error()
	if !strings.Contains(msg, "A") || !strings.Contains(msg, "B") {
		t.Fatalf("cycle message %q should name A and B", msg)
	}
	if strings.Contains(msg, "C") {
		t.Fatalf("cycle message %q should not include downstream node C", msg)
	}
}

func TestDuplicateMemberName(t *testing.T) {
	t.Parallel()
	r := group.NewRegistry()
	_, err := r.Create(group.Group{
		Name: "dupname",
		Members: []group.Member{
			{Name: "a"},
			{Name: "a"},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "duplicate member name") {
		t.Fatalf("err = %v, want duplicate member name", err)
	}
}

func TestEmptyMemberName(t *testing.T) {
	t.Parallel()
	r := group.NewRegistry()
	_, err := r.Create(group.Group{
		Name:    "empty",
		Members: []group.Member{{Name: ""}},
	})
	if err == nil || !strings.Contains(err.Error(), "empty name") {
		t.Fatalf("err = %v, want empty name", err)
	}
}

func TestCloneIsolation(t *testing.T) {
	t.Parallel()
	r := group.NewRegistry()
	g, err := r.Create(group.Group{
		Name: "iso",
		Members: []group.Member{
			{Name: "a", DependsOn: []string{"b"}},
			{Name: "b"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	// Mutate the returned copy; the registry's stored copy must be unaffected.
	g.Members[0].Name = "hacked"
	g.Members[0].DependsOn[0] = "hacked"
	got, err := r.Get(g.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Members[0].Name != "a" || got.Members[0].DependsOn[0] != "b" {
		t.Fatalf("registry copy was mutated through returned pointer: %+v", got.Members[0])
	}
}

func TestRemoveNotFound(t *testing.T) {
	t.Parallel()
	r := group.NewRegistry()
	if err := r.Remove("grp-missing"); !errors.Is(err, group.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}
