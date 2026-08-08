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
	"crypto/rand"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/scrothers/pmmcp/internal/event"

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
	if _, err := event.NewSQLiteLog(db); err == nil {
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

	b := event.NewBus(10)
	if _, err := b.Append(context.Background(), event.Event{}); err == nil {
		t.Fatal("expected id generation failure")
	}
}

func TestAppendSQLInsertError(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	b, err := event.NewSQLiteLog(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Append(context.Background(), event.Event{Type: "t"}); err == nil {
		t.Fatal("expected insert error on closed db")
	}
}

func TestAppendSQLLastInsertIDError(t *testing.T) {
	t.Parallel()
	db, c := openFlaky(t)
	b, err := event.NewSQLiteLog(db)
	if err != nil {
		t.Fatal(err)
	}
	c.armLastID()
	if _, err := b.Append(context.Background(), event.Event{Type: "t"}); err == nil {
		t.Fatal("expected last-insert-id error")
	}
}

func TestAppendSQLSweepAgeError(t *testing.T) {
	t.Parallel()
	db, c := openFlaky(t)
	b, err := event.NewSQLiteLog(db)
	if err != nil {
		t.Fatal(err)
	}
	c.resetExecCounter()
	c.armExecAt(2) // 1 = the insert; 2 = sweep's age-based delete
	if _, err := b.Append(context.Background(), event.Event{Type: "t"}); err == nil {
		t.Fatal("expected sweep age-delete error")
	}
}

func TestAppendSQLSweepCountError(t *testing.T) {
	t.Parallel()
	db, c := openFlaky(t)
	b, err := event.NewSQLiteLog(db)
	if err != nil {
		t.Fatal(err)
	}
	c.resetExecCounter()
	c.armExecAt(3) // 1 = insert, 2 = sweep age-delete, 3 = sweep count-delete
	if _, err := b.Append(context.Background(), event.Event{Type: "t"}); err == nil {
		t.Fatal("expected sweep count-delete error")
	}
}

func TestQuerySinceDefaultLimit(t *testing.T) {
	t.Parallel()
	b := event.NewBus(100)
	ctx := context.Background()
	for range 3 {
		if _, err := b.Append(ctx, event.Event{Type: "t"}); err != nil {
			t.Fatal(err)
		}
	}
	if got := b.QuerySince(ctx, 0, "", 0); len(got) != 3 { // 0 -> default 100
		t.Fatalf("default limit len = %d, want 3", len(got))
	}
}

func TestQuerySinceMemProcessFilter(t *testing.T) {
	t.Parallel()
	b := event.NewBus(100)
	ctx := context.Background()
	if _, err := b.Append(ctx, event.Event{Type: "t", ProcessID: "a"}); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Append(ctx, event.Event{Type: "t", ProcessID: "b"}); err != nil {
		t.Fatal(err)
	}
	got := b.QuerySince(ctx, 0, "a", 100)
	if len(got) != 1 || got[0].ProcessID != "a" {
		t.Fatalf("filtered = %+v", got)
	}
}

func TestQuerySinceSQLError(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	b, err := event.NewSQLiteLog(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if got := b.QuerySince(context.Background(), 0, "", 10); got != nil {
		t.Fatalf("since on closed db = %+v, want nil", got)
	}
}

func TestQuerySQLError(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	b, err := event.NewSQLiteLog(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if got := b.Query(context.Background(), "", 10); got != nil {
		t.Fatalf("query on closed db = %+v, want nil", got)
	}
}

func TestScanEventsScanError(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	b, err := event.NewSQLiteLog(db)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := b.Append(ctx, event.Event{Type: "t"}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE events SET at_unix_nano = ?`, "not-a-number"); err != nil {
		t.Fatal(err)
	}
	if got := b.Query(ctx, "", 10); got != nil {
		t.Fatalf("query over corrupt row = %+v, want nil (scan error)", got)
	}
}

func TestScanEventsRowsErr(t *testing.T) {
	t.Parallel()
	db, c := openFlaky(t)
	b, err := event.NewSQLiteLog(db)
	if err != nil {
		t.Fatal(err)
	}
	c.armNextRow()
	if got := b.Query(context.Background(), "", 10); got != nil {
		t.Fatalf("query with forced rows error = %+v, want nil", got)
	}
}

func TestTruncateMidRuneBackstep(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	// "abc" + "é" (2-byte rune) + more: a cap of 4 bytes lands on é's
	// continuation byte, forcing truncate to back up to the rune boundary.
	b, err := event.NewSQLiteLog(db, event.WithMaxPayload(4))
	if err != nil {
		t.Fatal(err)
	}
	got, err := b.Append(context.Background(), event.Event{Type: "t", Message: "abc" + "é" + "trailing"})
	if err != nil {
		t.Fatal(err)
	}
	if !utf8.ValidString(strings.TrimSuffix(got.Message, "…[truncated]")) {
		t.Fatalf("truncated message is not valid utf8: %q", got.Message)
	}
	if got.Message != "abc…[truncated]" {
		t.Fatalf("truncated = %q, want back-stepped to the rune boundary", got.Message)
	}
}
