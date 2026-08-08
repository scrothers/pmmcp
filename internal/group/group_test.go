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
)

func TestCreateGetListRemove(t *testing.T) {
	t.Parallel()
	r := group.NewRegistry()
	g, err := r.Create(group.Group{
		Name:      "web",
		ProjectID: "proj-a",
		Members: []group.Member{
			{Name: "api", Order: 0},
			{Name: "worker", Order: 1},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(g.ID, "grp-") {
		t.Fatalf("id %q", g.ID)
	}
	got, err := r.Get(g.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "web" || len(got.Members) != 2 {
		t.Fatalf("get: %+v", got)
	}
	list := r.List("proj-a")
	if len(list) != 1 {
		t.Fatalf("list len %d", len(list))
	}
	if err := r.Remove(g.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Get(g.ID); !errors.Is(err, group.ErrNotFound) {
		t.Fatalf("after remove: %v", err)
	}
}

func TestStartOrderDependsOn(t *testing.T) {
	t.Parallel()
	// A depends on B → B starts first, then A.
	r := group.NewRegistry()
	g, err := r.Create(group.Group{
		Name: "stack",
		Members: []group.Member{
			{Name: "A", DependsOn: []string{"B"}},
			{Name: "B"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	order, err := r.StartOrder(g.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(order) != 2 || order[0] != "B" || order[1] != "A" {
		t.Fatalf("start order = %v, want [B A]", order)
	}
	stop, err := r.StopOrder(g.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(stop) != 2 || stop[0] != "A" || stop[1] != "B" {
		t.Fatalf("stop order = %v, want [A B]", stop)
	}
}

func TestStartOrderChain(t *testing.T) {
	t.Parallel()
	// api depends on db; web depends on api → db, api, web.
	r := group.NewRegistry()
	g, err := r.Create(group.Group{
		Name: "full",
		Members: []group.Member{
			{Name: "web", DependsOn: []string{"api"}},
			{Name: "api", DependsOn: []string{"db"}},
			{Name: "db"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	order, err := r.StartOrder(g.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"db", "api", "web"}
	if len(order) != len(want) {
		t.Fatalf("order %v, want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("order %v, want %v", order, want)
		}
	}
}

func TestCycleFails(t *testing.T) {
	t.Parallel()
	r := group.NewRegistry()
	_, err := r.Create(group.Group{
		Name: "cyclic",
		Members: []group.Member{
			{Name: "A", DependsOn: []string{"B"}},
			{Name: "B", DependsOn: []string{"A"}},
		},
	})
	if !errors.Is(err, group.ErrCycle) {
		t.Fatalf("create cycle: err = %v, want ErrCycle", err)
	}
}

func TestSelfCycleFails(t *testing.T) {
	t.Parallel()
	r := group.NewRegistry()
	_, err := r.Create(group.Group{
		Name: "self",
		Members: []group.Member{
			{Name: "A", DependsOn: []string{"A"}},
		},
	})
	if !errors.Is(err, group.ErrCycle) {
		t.Fatalf("self cycle: err = %v, want ErrCycle", err)
	}
}

func TestUnknownDependency(t *testing.T) {
	t.Parallel()
	r := group.NewRegistry()
	_, err := r.Create(group.Group{
		Name: "bad",
		Members: []group.Member{
			{Name: "A", DependsOn: []string{"missing"}},
		},
	})
	if err == nil {
		t.Fatal("expected error for unknown dependency")
	}
}
