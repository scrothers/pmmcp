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

package audit

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/scrothers/pmmcp/internal/id"
)

// Outcome classifies the result of an audited action.
const (
	// OutcomeAllowed marks an action that was authorized and succeeded.
	OutcomeAllowed = "allowed"
	// OutcomeDenied marks an action refused by authorization.
	OutcomeDenied = "denied"
	// OutcomeError marks an authorized action that failed to complete.
	OutcomeError = "error"
)

// Retention defaults.
const (
	// DefaultMaxAge is the age bound on retained audit records (~90 days),
	// longer than the general event log per the glossary.
	DefaultMaxAge = 90 * 24 * time.Hour
	// DefaultMaxRecords is the count bound for the in-memory backend only.
	DefaultMaxRecords = 100000
)

// Record is a control-plane audit entry (aud- IDs).
type Record struct {
	// Seq is a monotonic sequence number assigned on append.
	Seq    int64
	ID     string
	Action string
	Actor  string
	// Role is the principal's role (e.g. admin, agent).
	Role      string
	SessionID string
	Target    string
	// Outcome is one of OutcomeAllowed/OutcomeDenied/OutcomeError.
	Outcome string
	// Capability is the permission checked for the action.
	Capability string
	// Client is the caller kind (e.g. mcp, cli).
	Client string
	// Reason explains a denial or error (no secret values).
	Reason string
	// RequestID correlates the record with events and IPC.
	RequestID string
	Detail    string
	At        time.Time
}

// Filter narrows a QueryFilter. Zero-value fields are unrestricted.
type Filter struct {
	Actor     string
	SessionID string
	Action    string
	Target    string
	Outcome   string
	Since     time.Time
	Until     time.Time
}

// Log is an append-only control-plane audit trail. It has two backends selected
// at construction: an in-memory ring (New) for interim or test use, and a
// durable SQLite table (NewSQLiteLog) that survives daemon restarts per
// /. The durable backend retains by age and never truncates by
// count, so records are not silently dropped.
type Log struct {
	mu      sync.Mutex
	records []Record
	seqCtr  int64

	maxKeep int
	maxAge  time.Duration

	// db is nil for the in-memory backend.
	db *sql.DB
}

// Option configures a Log.
type Option func(*Log)

// WithMaxAge overrides the retention age bound.
func WithMaxAge(d time.Duration) Option {
	return func(l *Log) {
		if d > 0 {
			l.maxAge = d
		}
	}
}

// New creates an in-memory audit log keeping at most maxKeep records
// (0 → DefaultMaxRecords). Its trail is volatile; prefer NewSQLiteLog for a
// durable, forensic log.
func New(maxKeep int) *Log {
	if maxKeep <= 0 {
		maxKeep = DefaultMaxRecords
	}
	return &Log{maxKeep: maxKeep, maxAge: DefaultMaxAge}
}

// NewSQLiteLog creates a SQLite-backed durable audit log on db, creating its
// schema if absent. db is the daemon's shared handle (see sqlite.Store.DB).
func NewSQLiteLog(db *sql.DB, opts ...Option) (*Log, error) {
	if db == nil {
		return nil, fmt.Errorf("audit: nil db")
	}
	l := &Log{db: db, maxAge: DefaultMaxAge}
	for _, o := range opts {
		o(l)
	}
	if err := l.migrate(context.Background()); err != nil {
		return nil, err
	}
	return l, nil
}

func (l *Log) migrate(ctx context.Context) error {
	const ddl = `
CREATE TABLE IF NOT EXISTS audit (
	seq INTEGER PRIMARY KEY AUTOINCREMENT,
	id TEXT NOT NULL UNIQUE,
	action TEXT NOT NULL DEFAULT '',
	actor TEXT NOT NULL DEFAULT '',
	role TEXT NOT NULL DEFAULT '',
	session_id TEXT NOT NULL DEFAULT '',
	target TEXT NOT NULL DEFAULT '',
	outcome TEXT NOT NULL DEFAULT '',
	capability TEXT NOT NULL DEFAULT '',
	client TEXT NOT NULL DEFAULT '',
	reason TEXT NOT NULL DEFAULT '',
	request_id TEXT NOT NULL DEFAULT '',
	detail TEXT NOT NULL DEFAULT '',
	at_unix_nano INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS audit_target ON audit(target, seq);
CREATE INDEX IF NOT EXISTS audit_actor ON audit(actor, seq);
CREATE INDEX IF NOT EXISTS audit_at ON audit(at_unix_nano);`
	if _, err := l.db.ExecContext(ctx, ddl); err != nil {
		return fmt.Errorf("audit: migrate: %w", err)
	}
	return nil
}

// Append stores an audit record, assigning an aud- ID and timestamp when empty
// and a monotonic Seq. The returned Record carries the assigned fields.
func (l *Log) Append(ctx context.Context, r Record) (Record, error) {
	if r.ID == "" {
		aid, err := id.New(id.Audit)
		if err != nil {
			return Record{}, err
		}
		r.ID = aid
	}
	if r.At.IsZero() {
		r.At = time.Now().UTC()
	} else {
		r.At = r.At.UTC()
	}
	if l.db != nil {
		return l.appendSQL(ctx, r)
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.seqCtr++
	r.Seq = l.seqCtr
	l.records = append(l.records, r)
	if len(l.records) > l.maxKeep {
		trimmed := make([]Record, l.maxKeep)
		copy(trimmed, l.records[len(l.records)-l.maxKeep:])
		l.records = trimmed
	}
	return r, nil
}

func (l *Log) appendSQL(ctx context.Context, r Record) (Record, error) {
	res, err := l.db.ExecContext(ctx, `
INSERT INTO audit (id, action, actor, role, session_id, target, outcome, capability, client, reason, request_id, detail, at_unix_nano)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.Action, r.Actor, r.Role, r.SessionID, r.Target, r.Outcome,
		r.Capability, r.Client, r.Reason, r.RequestID, r.Detail, r.At.UnixNano(),
	)
	if err != nil {
		return Record{}, fmt.Errorf("audit: append: %w", err)
	}
	seq, err := res.LastInsertId()
	if err != nil {
		return Record{}, fmt.Errorf("audit: append seq: %w", err)
	}
	r.Seq = seq
	if err := l.sweep(ctx); err != nil {
		return Record{}, err
	}
	return r, nil
}

// sweep enforces the age-based retention bound. Audit is not truncated by count.
func (l *Log) sweep(ctx context.Context) error {
	cutoff := time.Now().Add(-l.maxAge).UnixNano()
	if _, err := l.db.ExecContext(ctx, `DELETE FROM audit WHERE at_unix_nano < ?`, cutoff); err != nil {
		return fmt.Errorf("audit: sweep: %w", err)
	}
	return nil
}

// Query returns the newest limit records (0 → 100), optionally filtered by
// exact target, in chronological order.
func (l *Log) Query(ctx context.Context, target string, limit int) []Record {
	return l.QueryFilter(ctx, Filter{Target: target}, limit)
}

// QueryFilter returns the newest limit records (0 → 100) matching f, in
// chronological order.
func (l *Log) QueryFilter(ctx context.Context, f Filter, limit int) []Record {
	if limit <= 0 {
		limit = 100
	}
	if l.db != nil {
		out, err := l.querySQL(ctx, f, limit)
		if err != nil {
			slog.Default().Error("audit query failed", "err", err)
			return nil
		}
		return out
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []Record
	for _, r := range l.records {
		if matches(r, f) {
			out = append(out, r)
		}
	}
	if len(out) > limit {
		out = out[len(out)-limit:]
	}
	cp := make([]Record, len(out))
	copy(cp, out)
	return cp
}

func matches(r Record, f Filter) bool {
	if f.Target != "" && r.Target != f.Target {
		return false
	}
	if f.Actor != "" && r.Actor != f.Actor {
		return false
	}
	if f.SessionID != "" && r.SessionID != f.SessionID {
		return false
	}
	if f.Action != "" && r.Action != f.Action {
		return false
	}
	if f.Outcome != "" && r.Outcome != f.Outcome {
		return false
	}
	if !f.Since.IsZero() && r.At.Before(f.Since) {
		return false
	}
	if !f.Until.IsZero() && !r.At.Before(f.Until) {
		return false
	}
	return true
}

func (l *Log) querySQL(ctx context.Context, f Filter, limit int) ([]Record, error) {
	var (
		where []string
		args  []any
	)
	add := func(col, val string) {
		if val != "" {
			where = append(where, col+" = ?")
			args = append(args, val)
		}
	}
	add("target", f.Target)
	add("actor", f.Actor)
	add("session_id", f.SessionID)
	add("action", f.Action)
	add("outcome", f.Outcome)
	if !f.Since.IsZero() {
		where = append(where, "at_unix_nano >= ?")
		args = append(args, f.Since.UnixNano())
	}
	if !f.Until.IsZero() {
		where = append(where, "at_unix_nano < ?")
		args = append(args, f.Until.UnixNano())
	}
	clause := "1=1"
	if len(where) > 0 {
		clause = strings.Join(where, " AND ")
	}
	args = append(args, limit)
	q := `
SELECT seq, id, action, actor, role, session_id, target, outcome, capability, client, reason, request_id, detail, at_unix_nano
FROM (
	SELECT seq, id, action, actor, role, session_id, target, outcome, capability, client, reason, request_id, detail, at_unix_nano
	FROM audit
	WHERE ` + clause + `
	ORDER BY seq DESC
	LIMIT ?
) ORDER BY seq ASC`
	rows, err := l.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("audit: query: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []Record
	for rows.Next() {
		var (
			r     Record
			nanos int64
		)
		if err := rows.Scan(
			&r.Seq, &r.ID, &r.Action, &r.Actor, &r.Role, &r.SessionID, &r.Target,
			&r.Outcome, &r.Capability, &r.Client, &r.Reason, &r.RequestID, &r.Detail, &nanos,
		); err != nil {
			return nil, fmt.Errorf("audit: scan: %w", err)
		}
		r.At = time.Unix(0, nanos).UTC()
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("audit: rows: %w", err)
	}
	return out, nil
}
