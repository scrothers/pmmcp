// Copyright 2026 Steven Crothers
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package sqlite implements store interfaces with modernc.org/sqlite.
//
// Access pattern: open only from pmmcpd (the daemon). Do not open from CLI/MCP
// processes.
package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/scrothers/pmmcp/internal/domain"
	"github.com/scrothers/pmmcp/internal/store"

	sqlitedrv "modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

// tsLayout is a fixed-width RFC3339 timestamp with nine fractional digits.
// Unlike time.RFC3339Nano (which strips trailing zeros), fixed width keeps
// lexicographic order equal to chronological order so string ORDER BY is sound.
const tsLayout = "2006-01-02T15:04:05.000000000Z07:00"

// Store is a SQLite-backed ProcessStore.
type Store struct {
	db *sql.DB
}

// Open opens (or creates) a SQLite database at path and returns a Store.
// Caller must Close. Does not auto-migrate; call Migrate.
// Prefer OpenContext when a parent context is available.
func Open(path string) (*Store, error) {
	return OpenContext(context.Background(), path)
}

// OpenContext is like Open but uses ctx for the initial Ping.
func OpenContext(ctx context.Context, path string) (*Store, error) {
	// modernc driver name is "sqlite". Pragmas ride in the DSN so every
	// connection the pool opens (including any recycled replacement) applies
	// them — a bare per-connection PRAGMA is lost on reconnect.
	dsn := path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"
	// sql.Open cannot fail here: the "sqlite" driver is registered by this
	// package's import of modernc.org/sqlite and does not implement
	// driver.DriverContext, so sql.Open never eagerly dials or parses the DSN.
	// Connection failures (bad path, bad pragma) surface below, on Ping.
	db, _ := sql.Open("sqlite", dsn)
	// One writer; keep it simple for local daemon.
	db.SetMaxOpenConns(1)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("sqlite: open: %w", err)
	}
	return &Store{db: db}, nil
}

// DB returns the underlying database handle. The event and audit logs share
// the daemon's single connection pool through it (: one process opens
// the database). Callers must not Close it; Store.Close owns the lifecycle.
func (s *Store) DB() *sql.DB {
	return s.db
}

// Close closes the database.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// Migrate applies ordered schema migrations.
func (s *Store) Migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS schema_migrations (
	version INTEGER PRIMARY KEY NOT NULL,
	applied_at TEXT NOT NULL
);`); err != nil {
		return fmt.Errorf("sqlite: migrations table: %w", err)
	}

	for _, m := range migrations {
		var n int
		err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM schema_migrations WHERE version = ?`, m.version).Scan(&n)
		if err != nil {
			return fmt.Errorf("sqlite: migration version check: %w", err)
		}
		if n > 0 {
			continue
		}
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("sqlite: begin: %w", err)
		}
		if _, err := tx.ExecContext(ctx, m.sql); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("sqlite: migrate v%d: %w", m.version, err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO schema_migrations(version, applied_at) VALUES (?, ?)`,
			m.version, time.Now().UTC().Format(time.RFC3339Nano),
		); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("sqlite: record migration v%d: %w", m.version, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("sqlite: commit migration v%d: %w", m.version, err)
		}
	}
	return nil
}

type migration struct {
	version int
	sql     string
}

// Ordered migrations — never edit applied SQL; append new versions.
var migrations = []migration{
	{
		version: 1,
		sql: `
CREATE TABLE processes (
	id TEXT PRIMARY KEY NOT NULL,
	name TEXT NOT NULL,
	project_id TEXT NOT NULL DEFAULT '',
	profile TEXT NOT NULL DEFAULT '',
	session_id TEXT NOT NULL DEFAULT '',
	status TEXT NOT NULL,
	desired TEXT NOT NULL,
	command_json TEXT NOT NULL,
	cwd TEXT NOT NULL DEFAULT '',
	sandbox TEXT NOT NULL DEFAULT '',
	runtime TEXT NOT NULL DEFAULT 'local',
	pid INTEGER NOT NULL DEFAULT 0,
	exit_code INTEGER NULL,
	last_error TEXT NOT NULL DEFAULT '',
	log_dir TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL,
	started_at TEXT NULL,
	exited_at TEXT NULL
);
CREATE INDEX processes_project_name ON processes(project_id, name);
CREATE INDEX processes_status ON processes(status);
`,
	},
	{
		//: restart chain links + env key names (never values).
		version: 2,
		sql: `
ALTER TABLE processes ADD COLUMN env_keys_json TEXT NOT NULL DEFAULT '[]';
ALTER TABLE processes ADD COLUMN predecessor_id TEXT NOT NULL DEFAULT '';
ALTER TABLE processes ADD COLUMN successor_id TEXT NOT NULL DEFAULT '';
`,
	},
	{
		// Enforce name-scope uniqueness over live generations only. A plain
		// UNIQUE(project_id, name) would break restart chains (a successor
		// reuses the predecessor's name); scoping the index to rows with no
		// successor keeps exactly one live process per (project_id, name) while
		// letting retired generations retain the name. Surfaces store.ErrConflict.
		version: 3,
		sql: `
CREATE UNIQUE INDEX processes_live_name ON processes(project_id, name) WHERE successor_id = '';
`,
	},
}

// Create implements store.ProcessStore.
func (s *Store) Create(ctx context.Context, p *domain.Process) error {
	if p == nil {
		return fmt.Errorf("sqlite: create: nil process")
	}
	if err := p.Validate(); err != nil {
		return err
	}
	if p.ID == "" {
		return fmt.Errorf("sqlite: create: empty id")
	}
	now := time.Now().UTC()
	createdAt := p.CreatedAt
	if createdAt.IsZero() {
		createdAt = now
	}
	updatedAt := now
	// Command and EnvKeys are []string; json.Marshal cannot fail for this type.
	cmdJSON, _ := json.Marshal(p.Command)
	envKeysJSON := marshalEnvKeys(p.EnvKeys)
	_, err := s.db.ExecContext(ctx, `
INSERT INTO processes (
	id, name, project_id, profile, session_id, status, desired, command_json, cwd,
	sandbox, runtime, pid, exit_code, last_error, log_dir,
	env_keys_json, predecessor_id, successor_id,
	created_at, updated_at, started_at, exited_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.Name, p.ProjectID, p.Profile, p.SessionID,
		string(p.Status), string(p.Desired), string(cmdJSON), p.Cwd,
		p.Sandbox, nullRuntime(p.Runtime), p.PID, nullInt(p.ExitCode), p.LastError, p.LogDir,
		string(envKeysJSON), p.PredecessorID, p.SuccessorID,
		fmtTime(createdAt), fmtTime(updatedAt), fmtTimePtr(p.StartedAt), fmtTimePtr(p.ExitedAt),
	)
	if err != nil {
		if isUnique(err) {
			return fmt.Errorf("%w: %w", store.ErrConflict, err)
		}
		return fmt.Errorf("sqlite: insert: %w", err)
	}
	// Only reflect persisted timestamps once the write has succeeded.
	p.CreatedAt = createdAt
	p.UpdatedAt = updatedAt
	return nil
}

// Get implements store.ProcessStore.
func (s *Store) Get(ctx context.Context, id string) (*domain.Process, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, name, project_id, profile, session_id, status, desired, command_json, cwd,
	sandbox, runtime, pid, exit_code, last_error, log_dir,
	env_keys_json, predecessor_id, successor_id,
	created_at, updated_at, started_at, exited_at
FROM processes WHERE id = ?`, id)
	p, err := scanProcess(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("sqlite: get: %w", err)
	}
	return p, nil
}

// Update implements store.ProcessStore. It replaces the whole row keyed by ID
// (last-writer-wins). Concurrent read-modify-write callers that need to detect
// a lost update should use UpdateWithCAS instead.
func (s *Store) Update(ctx context.Context, p *domain.Process) error {
	if p == nil || p.ID == "" {
		return fmt.Errorf("sqlite: update: missing id")
	}
	if err := p.Validate(); err != nil {
		return err
	}
	updatedAt := time.Now().UTC()
	cols, args := updateArgs(p, updatedAt)
	args = append(args, p.ID)
	res, err := s.db.ExecContext(ctx, `UPDATE processes SET `+cols+` WHERE id = ?`, args...)
	if err != nil {
		if isUnique(err) {
			return fmt.Errorf("%w: %w", store.ErrConflict, err)
		}
		return fmt.Errorf("sqlite: update: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite: rows: %w", err)
	}
	if n == 0 {
		return store.ErrNotFound
	}
	p.UpdatedAt = updatedAt
	return nil
}

// UpdateWithCAS implements store.ProcessStore. It performs an optimistic
// compare-and-swap: the update applies only if the row's persisted updated_at
// still equals p.UpdatedAt (the value the caller read). On a concurrent write
// that advanced updated_at it returns store.ErrConflict without clobbering the
// other writer; a missing row returns store.ErrNotFound. On success p.UpdatedAt
// advances to the new persisted value so the same struct can be updated again.
func (s *Store) UpdateWithCAS(ctx context.Context, p *domain.Process) error {
	if p == nil || p.ID == "" {
		return fmt.Errorf("sqlite: update cas: missing id")
	}
	if p.UpdatedAt.IsZero() {
		return fmt.Errorf("sqlite: update cas: missing updated_at token")
	}
	if err := p.Validate(); err != nil {
		return err
	}
	token := p.UpdatedAt
	updatedAt := time.Now().UTC()
	// Never let the new token collide with the old one; the CAS predicate must
	// change even under coarse clock resolution.
	if !updatedAt.After(token) {
		updatedAt = token.Add(time.Nanosecond)
	}
	cols, args := updateArgs(p, updatedAt)
	args = append(args, p.ID, fmtTime(token))
	res, err := s.db.ExecContext(ctx, `UPDATE processes SET `+cols+` WHERE id = ? AND updated_at = ?`, args...)
	if err != nil {
		if isUnique(err) {
			return fmt.Errorf("%w: %w", store.ErrConflict, err)
		}
		return fmt.Errorf("sqlite: update cas: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite: rows: %w", err)
	}
	if n == 0 {
		var exists int
		err := s.db.QueryRowContext(ctx, `SELECT 1 FROM processes WHERE id = ?`, p.ID).Scan(&exists)
		if errors.Is(err, sql.ErrNoRows) {
			return store.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("sqlite: update cas: existence check: %w", err)
		}
		return store.ErrConflict
	}
	p.UpdatedAt = updatedAt
	return nil
}

// updateArgs builds the shared SET clause and its positional arguments for the
// two update paths. The trailing WHERE arguments are appended by the caller.
func updateArgs(p *domain.Process, updatedAt time.Time) (string, []any) {
	// Command and EnvKeys are []string; json.Marshal cannot fail for this type.
	cmdJSON, _ := json.Marshal(p.Command)
	envKeysJSON := marshalEnvKeys(p.EnvKeys)
	const cols = `
	name = ?, project_id = ?, profile = ?, session_id = ?, status = ?, desired = ?,
	command_json = ?, cwd = ?, sandbox = ?, runtime = ?, pid = ?, exit_code = ?,
	last_error = ?, log_dir = ?, env_keys_json = ?, predecessor_id = ?, successor_id = ?,
	updated_at = ?, started_at = ?, exited_at = ?`
	args := []any{
		p.Name, p.ProjectID, p.Profile, p.SessionID, string(p.Status), string(p.Desired),
		string(cmdJSON), p.Cwd, p.Sandbox, nullRuntime(p.Runtime), p.PID, nullInt(p.ExitCode),
		p.LastError, p.LogDir, string(envKeysJSON), p.PredecessorID, p.SuccessorID,
		fmtTime(updatedAt), fmtTimePtr(p.StartedAt), fmtTimePtr(p.ExitedAt),
	}
	return cols, args
}

// Delete implements store.ProcessStore.
func (s *Store) Delete(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM processes WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("sqlite: delete: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite: rows: %w", err)
	}
	if n == 0 {
		return store.ErrNotFound
	}
	return nil
}

// List implements store.ProcessStore.
func (s *Store) List(ctx context.Context, f store.ProcessFilter) ([]*domain.Process, error) {
	q := `
SELECT id, name, project_id, profile, session_id, status, desired, command_json, cwd,
	sandbox, runtime, pid, exit_code, last_error, log_dir,
	env_keys_json, predecessor_id, successor_id,
	created_at, updated_at, started_at, exited_at
FROM processes WHERE 1=1`
	var args []any
	if f.ProjectID != "" {
		q += ` AND project_id = ?`
		args = append(args, f.ProjectID)
	}
	if f.Status != "" {
		q += ` AND status = ?`
		args = append(args, string(f.Status))
	}
	if f.Name != "" {
		q += ` AND name = ?`
		args = append(args, f.Name)
	}
	q += ` ORDER BY created_at ASC`
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []*domain.Process
	for rows.Next() {
		p, err := scanProcess(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: list rows: %w", err)
	}
	return out, nil
}

type scannable interface {
	Scan(dest ...any) error
}

func scanProcess(row scannable) (*domain.Process, error) {
	var (
		p           domain.Process
		status      string
		desired     string
		cmdJSON     string
		envKeysJSON string
		exitCode    sql.NullInt64
		createdAt   string
		updatedAt   string
		startedAt   sql.NullString
		exitedAt    sql.NullString
	)
	err := row.Scan(
		&p.ID, &p.Name, &p.ProjectID, &p.Profile, &p.SessionID,
		&status, &desired, &cmdJSON, &p.Cwd, &p.Sandbox, &p.Runtime, &p.PID,
		&exitCode, &p.LastError, &p.LogDir,
		&envKeysJSON, &p.PredecessorID, &p.SuccessorID,
		&createdAt, &updatedAt, &startedAt, &exitedAt,
	)
	if err != nil {
		return nil, err
	}
	p.Status = domain.Status(status)
	p.Desired = domain.Desired(desired)
	if err := json.Unmarshal([]byte(cmdJSON), &p.Command); err != nil {
		return nil, fmt.Errorf("sqlite: command json: %w", err)
	}
	if envKeysJSON != "" && envKeysJSON != "[]" {
		if err := json.Unmarshal([]byte(envKeysJSON), &p.EnvKeys); err != nil {
			return nil, fmt.Errorf("sqlite: env_keys json: %w", err)
		}
	}
	if exitCode.Valid {
		v := int(exitCode.Int64)
		p.ExitCode = &v
	}
	p.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return nil, fmt.Errorf("sqlite: created_at: %w", err)
	}
	p.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return nil, fmt.Errorf("sqlite: updated_at: %w", err)
	}
	if startedAt.Valid && startedAt.String != "" {
		t, err := time.Parse(time.RFC3339Nano, startedAt.String)
		if err != nil {
			return nil, fmt.Errorf("sqlite: started_at: %w", err)
		}
		p.StartedAt = &t
	}
	if exitedAt.Valid && exitedAt.String != "" {
		t, err := time.Parse(time.RFC3339Nano, exitedAt.String)
		if err != nil {
			return nil, fmt.Errorf("sqlite: exited_at: %w", err)
		}
		p.ExitedAt = &t
	}
	return &p, nil
}

func fmtTime(t time.Time) string {
	return t.UTC().Format(tsLayout)
}

func fmtTimePtr(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.UTC().Format(tsLayout)
}

// marshalEnvKeys encodes env key names, normalizing a nil slice to "[]" so the
// column never carries the JSON literal null. Encoding []string cannot fail.
func marshalEnvKeys(keys []string) []byte {
	if keys == nil {
		return []byte("[]")
	}
	b, _ := json.Marshal(keys)
	return b
}

func nullInt(v *int) any {
	if v == nil {
		return nil
	}
	return *v
}

func nullRuntime(r string) string {
	if r == "" {
		return "local"
	}
	return r
}

func isUnique(err error) bool {
	// Prefer modernc's typed error codes over message matching.
	var serr *sqlitedrv.Error
	if errors.As(err, &serr) {
		switch serr.Code() {
		case sqlite3.SQLITE_CONSTRAINT_UNIQUE, sqlite3.SQLITE_CONSTRAINT_PRIMARYKEY:
			return true
		}
	}
	// Fallback for wrapped/rewritten messages.
	msg := err.Error()
	return strings.Contains(msg, "UNIQUE") || strings.Contains(msg, "unique")
}

// Compile-time check.
var _ store.ProcessStore = (*Store)(nil)
