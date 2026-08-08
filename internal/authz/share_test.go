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

package authz_test

import (
	"sync"
	"testing"

	"github.com/scrothers/pmmcp/internal/authz"
)

func TestShareBookAllowed(t *testing.T) {
	t.Parallel()
	b := authz.NewShareBook()
	if b.Allowed("proc-1", "sess-b", authz.CapProcessStop) {
		t.Fatal("no grant yet, must deny")
	}
	b.Share(authz.Grant{Target: "proc-1", Cap: authz.CapProcessStop, ToSession: "sess-b", Grantor: "sess-a"})
	if !b.Allowed("proc-1", "sess-b", authz.CapProcessStop) {
		t.Fatal("granted pair must be allowed")
	}
	// Wrong session, target, or capability must not match.
	if b.Allowed("proc-1", "sess-c", authz.CapProcessStop) {
		t.Fatal("different session must deny")
	}
	if b.Allowed("proc-2", "sess-b", authz.CapProcessStop) {
		t.Fatal("different target must deny")
	}
	if b.Allowed("proc-1", "sess-b", authz.CapProcessStart) {
		t.Fatal("different capability must deny")
	}
}

func TestShareBookDedupe(t *testing.T) {
	t.Parallel()
	b := authz.NewShareBook()
	g := authz.Grant{Target: "proc-1", Cap: authz.CapProcessStop, ToSession: "sess-b"}
	b.Share(g)
	b.Share(g)
	b.Share(g)
	// Duplicates collapse: a single Unshare removes exactly one logical grant.
	if n := b.Unshare("proc-1", "sess-b", authz.CapProcessStop); n != 1 {
		t.Fatalf("Unshare removed %d, want 1 (dedupe on insert)", n)
	}
	if b.Allowed("proc-1", "sess-b", authz.CapProcessStop) {
		t.Fatal("grant should be gone")
	}
}

func TestShareBookUnshareWildcard(t *testing.T) {
	t.Parallel()
	b := authz.NewShareBook()
	b.Share(authz.Grant{Target: "proc-1", Cap: authz.CapProcessStop, ToSession: "sess-b"})
	b.Share(authz.Grant{Target: "proc-1", Cap: authz.CapProcessRestart, ToSession: "sess-b"})
	b.Share(authz.Grant{Target: "proc-2", Cap: authz.CapProcessStop, ToSession: "sess-b"})
	// Empty capability removes every grant for the (target, session) pair.
	if n := b.Unshare("proc-1", "sess-b", ""); n != 2 {
		t.Fatalf("wildcard Unshare removed %d, want 2", n)
	}
	if b.Allowed("proc-1", "sess-b", authz.CapProcessStop) ||
		b.Allowed("proc-1", "sess-b", authz.CapProcessRestart) {
		t.Fatal("proc-1 grants should be gone")
	}
	if !b.Allowed("proc-2", "sess-b", authz.CapProcessStop) {
		t.Fatal("proc-2 grant should survive")
	}
}

func TestShareBookConcurrent(t *testing.T) {
	t.Parallel()
	b := authz.NewShareBook()
	var wg sync.WaitGroup
	for i := range 50 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			g := authz.Grant{Target: "proc-1", Cap: authz.CapProcessStop, ToSession: "sess-b"}
			b.Share(g)
			_ = b.Allowed("proc-1", "sess-b", authz.CapProcessStop)
			if n%2 == 0 {
				b.Unshare("proc-1", "sess-b", authz.CapProcessStop)
			}
		}(i)
	}
	wg.Wait()
}
