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
	"crypto/rand"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/scrothers/pmmcp/internal/audit"

	_ "modernc.org/sqlite"
)

// openFlaky opens a *sql.DB backed by a flakyConnector, giving the test
// control over the underlying driver.
func openFlaky(t *testing.T) (*sql.DB, *flakyConnector) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "flaky.db")
	c := newFlakyConnector(path)
	db := sql.OpenDB(c)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	return db, c
}

func TestNewSQLiteLogMigrateError(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := audit.NewSQLiteLog(db); err == nil {
		t.Fatal("expected migrate error on closed db")
	}
}

// failingReader always errors, simulating exhausted entropy for id.New.
type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("entropy exhausted") }

// TestAppendIDGenerationFailure swaps the process-global crypto/rand.Reader,
// so it must not run in parallel with anything else touching it. Go's test
// scheduler runs all non-parallel tests to completion, sequentially, before
// any parallel-marked test in the package begins, so this is race-safe.
func TestAppendIDGenerationFailure(t *testing.T) {
	orig := rand.Reader
	rand.Reader = failingReader{}
	defer func() { rand.Reader = orig }()

	l := audit.New(10)
	if _, err := l.Append(context.Background(), audit.Record{}); err == nil {
		t.Fatal("expected id generation failure")
	}
}

func TestAppendSQLInsertError(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	l, err := audit.NewSQLiteLog(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Append(context.Background(), audit.Record{Action: "a"}); err == nil {
		t.Fatal("expected insert error on closed db")
	}
}

func TestAppendSQLLastInsertIDError(t *testing.T) {
	t.Parallel()
	db, c := openFlaky(t)
	l, err := audit.NewSQLiteLog(db)
	if err != nil {
		t.Fatal(err)
	}
	c.armLastID()
	if _, err := l.Append(context.Background(), audit.Record{Action: "a"}); err == nil {
		t.Fatal("expected last-insert-id error")
	}
}

func TestAppendSQLSweepError(t *testing.T) {
	t.Parallel()
	db, c := openFlaky(t)
	l, err := audit.NewSQLiteLog(db)
	if err != nil {
		t.Fatal(err)
	}
	c.resetExecCounter()
	c.armExecAt(2) // 1 = the insert; 2 = sweep's delete
	if _, err := l.Append(context.Background(), audit.Record{Action: "a"}); err == nil {
		t.Fatal("expected sweep error")
	}
}

func TestQueryFilterDefaultLimit(t *testing.T) {
	t.Parallel()
	l := audit.New(100)
	ctx := context.Background()
	for range 3 {
		if _, err := l.Append(ctx, audit.Record{Action: "a", Target: "t"}); err != nil {
			t.Fatal(err)
		}
	}
	if got := l.QueryFilter(ctx, audit.Filter{}, 0); len(got) != 3 { // 0 -> default 100
		t.Fatalf("default limit len = %d, want 3", len(got))
	}
}

func TestQueryFilterSQLError(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	l, err := audit.NewSQLiteLog(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if got := l.QueryFilter(context.Background(), audit.Filter{}, 10); got != nil {
		t.Fatalf("query on closed db = %+v, want nil", got)
	}
}

func TestMatchesAllFilterFields(t *testing.T) {
	t.Parallel()
	l := audit.New(100)
	ctx := context.Background()
	base := time.Now().UTC().Add(-time.Hour)
	if _, err := l.Append(ctx, audit.Record{
		Action: "process.start", Actor: "alice", SessionID: "s1",
		Target: "proc-1", Outcome: audit.OutcomeAllowed, At: base,
	}); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		f    audit.Filter
	}{
		{"target-miss", audit.Filter{Target: "proc-2"}},
		{"session-miss", audit.Filter{SessionID: "s2"}},
		{"action-miss", audit.Filter{Action: "process.stop"}},
		{"outcome-miss", audit.Filter{Outcome: audit.OutcomeDenied}},
		{"since-miss", audit.Filter{Since: base.Add(time.Minute)}},
		{"until-miss", audit.Filter{Until: base}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := l.QueryFilter(ctx, tc.f, 100); len(got) != 0 {
				t.Fatalf("%s: got %+v, want no match", tc.name, got)
			}
		})
	}
}

func TestScanAuditScanError(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	l, err := audit.NewSQLiteLog(db)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := l.Append(ctx, audit.Record{Action: "a", Target: "t"}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE audit SET at_unix_nano = ?`, "not-a-number"); err != nil {
		t.Fatal(err)
	}
	if got := l.QueryFilter(ctx, audit.Filter{}, 10); got != nil {
		t.Fatalf("query over corrupt row = %+v, want nil (scan error)", got)
	}
}

func TestScanAuditRowsErr(t *testing.T) {
	t.Parallel()
	db, c := openFlaky(t)
	l, err := audit.NewSQLiteLog(db)
	if err != nil {
		t.Fatal(err)
	}
	c.armNextRow()
	if got := l.QueryFilter(context.Background(), audit.Filter{}, 10); got != nil {
		t.Fatalf("query with forced rows error = %+v, want nil", got)
	}
}
