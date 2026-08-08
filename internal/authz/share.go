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

package authz

import "sync"

// Grant is a cross-session share of a capability on a target id.
type Grant struct {
	Target    string
	Cap       Capability
	ToSession string
	// Grantor is the session that created the share (for audit attribution).
	// Optional; empty when the caller does not record it.
	Grantor string
}

// ShareBook tracks explicit shares (pm_share / pm_unshare). Its Allowed method
// is the canonical grant check: enforcement points consult it to decide whether
// a session may act on a target it does not own.
type ShareBook struct {
	mu     sync.Mutex
	grants []Grant
}

// NewShareBook creates an empty share book.
func NewShareBook() *ShareBook { return &ShareBook{} }

// Share adds a grant, deduplicating on (Target, Cap, ToSession) so repeated
// shares do not pile up duplicates. The Grantor of the first insert is kept.
func (b *ShareBook) Share(g Grant) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, existing := range b.grants {
		if existing.Target == g.Target && existing.ToSession == g.ToSession && existing.Cap == g.Cap {
			return
		}
	}
	b.grants = append(b.grants, g)
}

// Unshare removes matching grants and returns the number removed. An empty
// c matches every capability for the (target, session) pair.
func (b *ShareBook) Unshare(target, session string, c Capability) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := 0
	out := b.grants[:0]
	for _, g := range b.grants {
		if g.Target == target && g.ToSession == session && (c == "" || g.Cap == c) {
			n++
			continue
		}
		out = append(out, g)
	}
	b.grants = out
	return n
}

// Allowed reports whether session holds an explicit grant for c on target.
func (b *ShareBook) Allowed(target, session string, c Capability) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, g := range b.grants {
		if g.Target == target && g.ToSession == session && g.Cap == c {
			return true
		}
	}
	return false
}
