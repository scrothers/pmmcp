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

package sqlite_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/scrothers/pmmcp/internal/store/sqlite"

	_ "modernc.org/sqlite"
)

// openFlaky opens a *sqlite.Store backed by a flakyConnector, giving the test
// control over the underlying driver without touching production code.
func openFlaky(t *testing.T) (*sqlite.Store, *flakyConnector) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "flaky.db")
	c := newFlakyConnector(path)
	db := sql.OpenDB(c)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	return sqlite.NewForTest(db), c
}

func TestMigrateAfterCloseFails(t *testing.T) {
	t.Parallel()
	s := openMigrated(t)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s.Migrate(context.Background()); err == nil {
		t.Fatal("migrate on closed db should error")
	}
}

func TestMigrateVersionCheckError(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "x.db")
	s, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	// Pre-create schema_migrations without a "version" column. Migrate's
	// CREATE TABLE IF NOT EXISTS no-ops (name already exists), but its
	// SELECT COUNT(1) ... WHERE version = ? fails: no such column.
	if _, err := s.DB().ExecContext(context.Background(), `CREATE TABLE schema_migrations (id INTEGER)`); err != nil {
		t.Fatal(err)
	}
	if err := s.Migrate(context.Background()); err == nil {
		t.Fatal("expected version-check query error")
	}
}

func TestMigrateApplyError(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "x.db")
	s, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	// Pre-create "processes" so migration v1's CREATE TABLE collides.
	if _, err := s.DB().ExecContext(context.Background(), `CREATE TABLE processes (x INTEGER)`); err != nil {
		t.Fatal(err)
	}
	if err := s.Migrate(context.Background()); err == nil {
		t.Fatal("expected migration apply error")
	}
}

func TestMigrateRecordInsertError(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "x.db")
	s, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	// Pre-create schema_migrations with only "version" (no "applied_at"), so
	// the version-check query succeeds but recording the applied migration
	// fails, after the migration SQL itself already ran (and is rolled back).
	if _, err := s.DB().ExecContext(context.Background(),
		`CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY NOT NULL)`,
	); err != nil {
		t.Fatal(err)
	}
	if err := s.Migrate(context.Background()); err == nil {
		t.Fatal("expected migration-record insert error")
	}
}

func TestMigrateBeginTxError(t *testing.T) {
	t.Parallel()
	s, c := openFlaky(t)
	c.armBegin()
	if err := s.Migrate(context.Background()); err == nil {
		t.Fatal("expected begin-tx error")
	}
}

func TestMigrateCommitError(t *testing.T) {
	t.Parallel()
	s, c := openFlaky(t)
	c.armCommit()
	if err := s.Migrate(context.Background()); err == nil {
		t.Fatal("expected commit error")
	}
}
