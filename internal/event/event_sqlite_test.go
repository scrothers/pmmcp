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

package event_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/scrothers/pmmcp/internal/event"

	_ "modernc.org/sqlite"
)

func openDB(t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "events.db")
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestSQLiteAppendQuery(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	b, err := event.NewSQLiteLog(db)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	e, err := b.Append(ctx, event.Event{Type: "process.started", ProcessID: "proc-x", Severity: "info", ProjectID: "proj-1", Message: "hi"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(e.ID, "evt-") || e.Seq == 0 {
		t.Fatalf("assigned = %+v", e)
	}
	got := b.Query(ctx, "proc-x", 10)
	if len(got) != 1 || got[0].Type != "process.started" || got[0].Severity != "info" || got[0].ProjectID != "proj-1" {
		t.Fatalf("query = %+v", got)
	}
	if len(b.Query(ctx, "other", 10)) != 0 {
		t.Fatal("process filter failed")
	}
}

func TestSQLiteDurableAcrossReopen(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	ctx := context.Background()
	b1, err := event.NewSQLiteLog(db)
	if err != nil {
		t.Fatal(err)
	}
	for i := range 3 {
		if _, err := b1.Append(ctx, event.Event{Type: "t", Message: string(rune('a' + i))}); err != nil {
			t.Fatal(err)
		}
	}
	// A fresh log over the same db (a daemon restart) still sees the history.
	b2, err := event.NewSQLiteLog(db)
	if err != nil {
		t.Fatal(err)
	}
	if got := b2.Query(ctx, "", 100); len(got) != 3 {
		t.Fatalf("after reopen len = %d, want 3", len(got))
	}
}

func TestSQLiteCountRetention(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	b, err := event.NewSQLiteLog(db, event.WithMaxCount(3))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for i := range 5 {
		if _, err := b.Append(ctx, event.Event{Type: "t", Message: string(rune('a' + i))}); err != nil {
			t.Fatal(err)
		}
	}
	got := b.Query(ctx, "", 100)
	if len(got) != 3 {
		t.Fatalf("retained = %d, want 3", len(got))
	}
	if got[0].Message != "c" || got[2].Message != "e" {
		t.Fatalf("retained newest = %+v", got)
	}
}

func TestSQLiteAgeRetention(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	b, err := event.NewSQLiteLog(db, event.WithMaxAge(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	old := time.Now().Add(-2 * time.Hour)
	if _, err := b.Append(ctx, event.Event{Type: "old", At: old}); err != nil {
		t.Fatal(err)
	}
	// A fresh append triggers the age sweep, evicting the stale record.
	if _, err := b.Append(ctx, event.Event{Type: "new"}); err != nil {
		t.Fatal(err)
	}
	got := b.Query(ctx, "", 100)
	if len(got) != 1 || got[0].Type != "new" {
		t.Fatalf("after age sweep = %+v", got)
	}
}

func TestSQLitePayloadTruncation(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	b, err := event.NewSQLiteLog(db, event.WithMaxPayload(10))
	if err != nil {
		t.Fatal(err)
	}
	e, err := b.Append(context.Background(), event.Event{Type: "t", Message: strings.Repeat("x", 100)})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(e.Message, "truncated]") {
		t.Fatalf("no truncation marker: %q", e.Message)
	}
	if len(e.Message) > 10+len("…[truncated]") {
		t.Fatalf("message too long: %d", len(e.Message))
	}
}

func TestSQLiteQuerySinceCursor(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	b, err := event.NewSQLiteLog(db)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	seqs := make([]int64, 0, 5)
	for range 5 {
		e, err := b.Append(ctx, event.Event{Type: "t"})
		if err != nil {
			t.Fatal(err)
		}
		seqs = append(seqs, e.Seq)
	}
	all := b.QuerySince(ctx, 0, "", 100)
	if len(all) != 5 {
		t.Fatalf("since 0 len = %d", len(all))
	}
	tail := b.QuerySince(ctx, seqs[2], "", 100)
	if len(tail) != 2 || tail[0].Seq != seqs[3] {
		t.Fatalf("since cursor = %+v", tail)
	}
}

func TestNewSQLiteLogNilDB(t *testing.T) {
	t.Parallel()
	if _, err := event.NewSQLiteLog(nil); err == nil {
		t.Fatal("nil db should error")
	}
}

func TestSQLiteConcurrent(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	b, err := event.NewSQLiteLog(db)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := b.Append(ctx, event.Event{Type: "t", ProcessID: "p"}); err != nil {
				t.Errorf("append: %v", err)
			}
			_ = b.Query(ctx, "p", 10)
			_ = b.QuerySince(ctx, 0, "", 10)
		}()
	}
	wg.Wait()
	if got := b.Query(ctx, "p", 100); len(got) != 10 {
		t.Fatalf("concurrent appends = %d, want 10", len(got))
	}
}
