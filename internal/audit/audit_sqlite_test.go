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

package audit_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/scrothers/pmmcp/internal/audit"

	_ "modernc.org/sqlite"
)

func openDB(t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "audit.db")
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestSQLiteAppendQueryOutcome(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	l, err := audit.NewSQLiteLog(db)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	r, err := l.Append(ctx, audit.Record{
		Action:     "process.start",
		Actor:      "agent",
		Role:       "agent",
		SessionID:  "sess-1",
		Target:     "proc-1",
		Outcome:    audit.OutcomeDenied,
		Capability: "process.write",
		Client:     "mcp",
		Reason:     "role lacks capability",
		RequestID:  "req-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(r.ID, "aud-") || r.Seq == 0 {
		t.Fatalf("assigned = %+v", r)
	}
	got := l.Query(ctx, "proc-1", 5)
	if len(got) != 1 {
		t.Fatalf("query len = %d", len(got))
	}
	g := got[0]
	if g.Outcome != audit.OutcomeDenied || g.Capability != "process.write" || g.Client != "mcp" || g.Reason != "role lacks capability" || g.RequestID != "req-1" || g.Role != "agent" {
		t.Fatalf("round-trip = %+v", g)
	}
}

func TestSQLiteDurableAcrossReopen(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	ctx := context.Background()
	l1, err := audit.NewSQLiteLog(db)
	if err != nil {
		t.Fatal(err)
	}
	for range 3 {
		if _, err := l1.Append(ctx, audit.Record{Action: "a", Target: "t"}); err != nil {
			t.Fatal(err)
		}
	}
	l2, err := audit.NewSQLiteLog(db)
	if err != nil {
		t.Fatal(err)
	}
	if got := l2.Query(ctx, "", 100); len(got) != 3 {
		t.Fatalf("after reopen = %d, want 3", len(got))
	}
}

func TestSQLiteQueryFilter(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	l, err := audit.NewSQLiteLog(db)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	_, _ = l.Append(ctx, audit.Record{Action: "process.start", Actor: "alice", SessionID: "s1", Target: "proc-1", Outcome: audit.OutcomeAllowed})
	_, _ = l.Append(ctx, audit.Record{Action: "process.stop", Actor: "bob", SessionID: "s2", Target: "proc-1", Outcome: audit.OutcomeDenied})
	_, _ = l.Append(ctx, audit.Record{Action: "process.start", Actor: "alice", SessionID: "s1", Target: "proc-2", Outcome: audit.OutcomeAllowed})

	if got := l.QueryFilter(ctx, audit.Filter{Actor: "alice"}, 100); len(got) != 2 {
		t.Fatalf("by actor = %d, want 2", len(got))
	}
	if got := l.QueryFilter(ctx, audit.Filter{Action: "process.stop"}, 100); len(got) != 1 {
		t.Fatalf("by action = %d, want 1", len(got))
	}
	if got := l.QueryFilter(ctx, audit.Filter{Outcome: audit.OutcomeDenied}, 100); len(got) != 1 {
		t.Fatalf("by outcome = %d, want 1", len(got))
	}
	if got := l.QueryFilter(ctx, audit.Filter{Actor: "alice", Target: "proc-2"}, 100); len(got) != 1 {
		t.Fatalf("combined = %d, want 1", len(got))
	}
	if got := l.QueryFilter(ctx, audit.Filter{Actor: "nobody"}, 100); len(got) != 0 {
		t.Fatalf("miss = %d, want 0", len(got))
	}
}

func TestSQLiteTimeRangeFilter(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	l, err := audit.NewSQLiteLog(db)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	// Recent timestamps so the 90-day age sweep does not evict them.
	base := time.Now().UTC().Add(-5 * time.Hour)
	for i := range 3 {
		if _, err := l.Append(ctx, audit.Record{Action: "a", Target: "t", At: base.Add(time.Duration(i) * time.Hour)}); err != nil {
			t.Fatal(err)
		}
	}
	got := l.QueryFilter(ctx, audit.Filter{Since: base.Add(30 * time.Minute), Until: base.Add(90 * time.Minute)}, 100)
	if len(got) != 1 {
		t.Fatalf("time range = %d, want 1", len(got))
	}
}

func TestSQLiteAgeRetention(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	l, err := audit.NewSQLiteLog(db, audit.WithMaxAge(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := l.Append(ctx, audit.Record{Action: "old", At: time.Now().Add(-2 * time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Append(ctx, audit.Record{Action: "new"}); err != nil {
		t.Fatal(err)
	}
	got := l.Query(ctx, "", 100)
	if len(got) != 1 || got[0].Action != "new" {
		t.Fatalf("after sweep = %+v", got)
	}
}

func TestNewSQLiteLogNilDB(t *testing.T) {
	t.Parallel()
	if _, err := audit.NewSQLiteLog(nil); err == nil {
		t.Fatal("nil db should error")
	}
}

func TestSQLiteConcurrent(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	l, err := audit.NewSQLiteLog(db)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := l.Append(ctx, audit.Record{Action: "a", Target: "t"}); err != nil {
				t.Errorf("append: %v", err)
			}
			_ = l.Query(ctx, "t", 10)
		}()
	}
	wg.Wait()
	if got := l.Query(ctx, "t", 100); len(got) != 10 {
		t.Fatalf("concurrent = %d, want 10", len(got))
	}
}
