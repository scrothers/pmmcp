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

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/scrothers/pmmcp/internal/id"
)

// ErrNotFound is returned when a group ID is unknown.
var ErrNotFound = errors.New("group: not found")

// ErrCycle is returned when member depends_on edges form a cycle.
var ErrCycle = errors.New("group: dependency cycle")

// ErrExists is returned when creating a group with a duplicate ID.
var ErrExists = errors.New("group: already exists")

// Member is a named unit within a process group.
type Member struct {
	Name      string
	ProcessID string
	Order     int
	DependsOn []string
}

// Group is a named collection of process units in a project.
type Group struct {
	ID        string
	Name      string
	ProjectID string
	Members   []Member
}

// Registry stores groups and computes start/stop order.
type Registry struct {
	mu   sync.Mutex
	byID map[string]*Group
}

// NewRegistry creates an empty group registry.
func NewRegistry() *Registry {
	return &Registry{byID: make(map[string]*Group)}
}

// Create registers a group. If g.ID is empty a grp- ULID is assigned.
// Members are deep-copied so callers can reuse their slice.
func (r *Registry) Create(g Group) (*Group, error) {
	if g.Name == "" {
		return nil, fmt.Errorf("group: create: empty name")
	}
	if g.ID == "" {
		gid, err := id.New(id.Group)
		if err != nil {
			return nil, fmt.Errorf("group: create: %w", err)
		}
		g.ID = gid
	} else if !id.HasPrefix(g.ID, id.Group) {
		return nil, fmt.Errorf("group: create: invalid id %q: want %s- prefix", g.ID, id.Group)
	}
	stored := cloneGroup(g)
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.byID[stored.ID]; ok {
		return nil, fmt.Errorf("%w: %s", ErrExists, stored.ID)
	}
	// Validate depends_on graph at create time so cycles fail early.
	if _, err := startOrder(stored.Members); err != nil {
		return nil, err
	}
	r.byID[stored.ID] = stored
	return cloneGroupPtr(stored), nil
}

// Get returns a group by ID.
func (r *Registry) Get(groupID string) (*Group, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	g, ok := r.byID[groupID]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, groupID)
	}
	return cloneGroupPtr(g), nil
}

// List returns all groups, optionally filtered by projectID (empty = all).
func (r *Registry) List(projectID string) []Group {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Group, 0, len(r.byID))
	for _, g := range r.byID {
		if projectID != "" && g.ProjectID != projectID {
			continue
		}
		out = append(out, *cloneGroupPtr(g))
	}
	// Deterministic order so listings are stable across calls.
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Remove deletes a group by ID.
func (r *Registry) Remove(groupID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.byID[groupID]; !ok {
		return fmt.Errorf("%w: %s", ErrNotFound, groupID)
	}
	delete(r.byID, groupID)
	return nil
}

// StartOrder returns member names in topological start order (dependencies first).
// Cycles yield ErrCycle.
func (r *Registry) StartOrder(groupID string) ([]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	g, ok := r.byID[groupID]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, groupID)
	}
	return startOrder(g.Members)
}

// StopOrder returns member names in reverse start order (dependents first).
func (r *Registry) StopOrder(groupID string) ([]string, error) {
	order, err := r.StartOrder(groupID)
	if err != nil {
		return nil, err
	}
	for i, j := 0, len(order)-1; i < j; i, j = i+1, j-1 {
		order[i], order[j] = order[j], order[i]
	}
	return order, nil
}

// startOrder computes a topological order of member names.
// Edge A depends_on B means B must start before A (B → A in the DAG).
func startOrder(members []Member) ([]string, error) {
	if len(members) == 0 {
		return nil, nil
	}

	names := make(map[string]struct{}, len(members))
	// deps[name] = set of names that name depends on (must come first).
	deps := make(map[string]map[string]struct{}, len(members))
	// reverse[name] = names that depend on name (dependents).
	reverse := make(map[string][]string, len(members))
	// Preserve input order for stable Kahn selection among zero-indegree nodes.
	orderIndex := make(map[string]int, len(members))
	// Explicit Member.Order takes precedence over input position in tie-breaks.
	memberOrder := make(map[string]int, len(members))

	for i, m := range members {
		if m.Name == "" {
			return nil, fmt.Errorf("group: member at index %d has empty name", i)
		}
		if _, dup := names[m.Name]; dup {
			return nil, fmt.Errorf("group: duplicate member name %q", m.Name)
		}
		names[m.Name] = struct{}{}
		orderIndex[m.Name] = i
		memberOrder[m.Name] = m.Order
		deps[m.Name] = make(map[string]struct{})
	}

	for _, m := range members {
		for _, d := range m.DependsOn {
			if d == m.Name {
				return nil, fmt.Errorf("%w: %s depends on itself", ErrCycle, m.Name)
			}
			if _, ok := names[d]; !ok {
				return nil, fmt.Errorf("group: %q depends on unknown member %q", m.Name, d)
			}
			// Dedupe: a repeated depends_on entry must not inflate the reverse
			// edge count, or the Kahn decrement drives indegree to zero early
			// and a dependent can start before its dependency.
			if _, seen := deps[m.Name][d]; seen {
				continue
			}
			deps[m.Name][d] = struct{}{}
			reverse[d] = append(reverse[d], m.Name)
		}
	}

	less := func(a, b string) bool {
		if memberOrder[a] != memberOrder[b] {
			return memberOrder[a] < memberOrder[b]
		}
		return orderIndex[a] < orderIndex[b]
	}

	// Kahn: start with nodes that have no dependencies.
	indegree := make(map[string]int, len(members))
	ready := make([]string, 0, len(members))
	for name, d := range deps {
		indegree[name] = len(d)
		if len(d) == 0 {
			ready = append(ready, name)
		}
	}
	// Sort ready deterministically (Member.Order, then input position).
	sortStable(ready, less)

	out := make([]string, 0, len(members))
	for len(ready) > 0 {
		// Pop the first (lowest ordering key) ready node.
		n := ready[0]
		ready = ready[1:]
		out = append(out, n)
		for _, dep := range reverse[n] {
			indegree[dep]--
			if indegree[dep] == 0 {
				ready = append(ready, dep)
				sortStable(ready, less)
			}
		}
	}

	if len(out) != len(members) {
		alive := make(map[string]bool, len(members))
		for name, deg := range indegree {
			if deg > 0 {
				alive[name] = true
			}
		}
		// A non-empty alive set here always contains a genuine cycle: Kahn's
		// algorithm only stalls on nodes whose dependencies never resolve,
		// and a finite graph where every node has an outgoing edge into the
		// same stalled set must contain a cycle reachable by following those
		// edges. findCyclePath's DFS explores every alive node as a root, so
		// it is guaranteed to find one.
		path := findCyclePath(deps, alive)
		return nil, fmt.Errorf("%w: %s", ErrCycle, strings.Join(path, " → "))
	}
	return out, nil
}

// findCyclePath returns a cycle (as node → node → … → node) reachable among the
// alive nodes, or nil if none is found. It reports only nodes on the cycle, not
// nodes merely downstream of one.
func findCyclePath(deps map[string]map[string]struct{}, alive map[string]bool) []string {
	const (
		white = 0
		gray  = 1
		black = 2
	)
	color := make(map[string]int, len(alive))
	var stack []string
	var dfs func(n string) []string
	dfs = func(n string) []string {
		color[n] = gray
		stack = append(stack, n)
		for d := range deps[n] {
			if !alive[d] {
				continue
			}
			switch color[d] {
			case gray:
				for i, s := range stack {
					if s == d {
						return append(append([]string{}, stack[i:]...), d)
					}
				}
			case white:
				if p := dfs(d); p != nil {
					return p
				}
			}
		}
		stack = stack[:len(stack)-1]
		color[n] = black
		return nil
	}
	// Sort roots for a deterministic starting point.
	roots := make([]string, 0, len(alive))
	for n := range alive {
		roots = append(roots, n)
	}
	sort.Strings(roots)
	for _, n := range roots {
		if color[n] == white {
			if p := dfs(n); p != nil {
				return p
			}
		}
	}
	return nil
}

func sortStable(names []string, less func(a, b string) bool) {
	// Insertion sort — member counts are tiny.
	for i := 1; i < len(names); i++ {
		j := i
		for j > 0 && less(names[j], names[j-1]) {
			names[j], names[j-1] = names[j-1], names[j]
			j--
		}
	}
}

func cloneGroup(g Group) *Group {
	cp := g
	if g.Members != nil {
		cp.Members = make([]Member, len(g.Members))
		for i, m := range g.Members {
			cp.Members[i] = cloneMember(m)
		}
	}
	return &cp
}

func cloneGroupPtr(g *Group) *Group {
	if g == nil {
		return nil
	}
	return cloneGroup(*g)
}

func cloneMember(m Member) Member {
	cp := m
	if m.DependsOn != nil {
		cp.DependsOn = make([]string, len(m.DependsOn))
		copy(cp.DependsOn, m.DependsOn)
	}
	return cp
}
