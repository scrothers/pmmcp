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

package event

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/scrothers/pmmcp/internal/id"
)

// Retention and payload defaults.
const (
	// DefaultMaxEvents is the count bound on retained events.
	DefaultMaxEvents = 100000
	// DefaultMaxAge is the age bound on retained events.
	DefaultMaxAge = 7 * 24 * time.Hour
	// DefaultMaxPayload bounds a single event's Message in bytes; longer
	// messages are truncated with a marker.
	DefaultMaxPayload = 16 * 1024
)

const truncMarker = "…[truncated]"

// Event is a domain lifecycle/control event (evt- IDs).
type Event struct {
	// Seq is a monotonic sequence number assigned on append. It is stable
	// across the log's lifetime and usable as a resumable stream cursor.
	Seq       int64
	ID        string
	Type      string
	ProcessID string
	GroupID   string
	SessionID string
	// Severity is an optional level (e.g. info, warn, error).
	Severity string
	// ProjectID scopes the event to a project when known.
	ProjectID string
	Message   string
	At        time.Time
}

// Bus is an append-only domain-event log with retention bounds. It has two
// backends selected at construction: an in-memory ring (NewBus) for interim or
// test use, and a durable SQLite table (NewSQLiteLog) that survives daemon
// restarts per.
type Bus struct {
	mu     sync.Mutex
	events []Event
	seqCtr int64

	maxKeep    int
	maxAge     time.Duration
	maxPayload int

	// db is nil for the in-memory backend.
	db *sql.DB
}

// Option configures a Bus.
type Option func(*Bus)

// WithMaxCount overrides the retained-event count bound.
func WithMaxCount(n int) Option {
	return func(b *Bus) {
		if n > 0 {
			b.maxKeep = n
		}
	}
}

// WithMaxAge overrides the retained-event age bound.
func WithMaxAge(d time.Duration) Option {
	return func(b *Bus) {
		if d > 0 {
			b.maxAge = d
		}
	}
}

// WithMaxPayload overrides the per-event Message byte cap.
func WithMaxPayload(n int) Option {
	return func(b *Bus) {
		if n > 0 {
			b.maxPayload = n
		}
	}
}

// NewBus creates an in-memory bus keeping at most maxKeep events
// (0 → DefaultMaxEvents). Its history is volatile; prefer NewSQLiteLog for a
// durable log.
func NewBus(maxKeep int) *Bus {
	if maxKeep <= 0 {
		maxKeep = DefaultMaxEvents
	}
	return &Bus{
		maxKeep:    maxKeep,
		maxAge:     DefaultMaxAge,
		maxPayload: DefaultMaxPayload,
	}
}

// NewSQLiteLog creates a SQLite-backed durable event log on db, creating its
// schema if absent. db is the daemon's shared handle (see sqlite.Store.DB).
func NewSQLiteLog(db *sql.DB, opts ...Option) (*Bus, error) {
	if db == nil {
		return nil, fmt.Errorf("event: nil db")
	}
	b := &Bus{
		db:         db,
		maxKeep:    DefaultMaxEvents,
		maxAge:     DefaultMaxAge,
		maxPayload: DefaultMaxPayload,
	}
	for _, o := range opts {
		o(b)
	}
	if err := b.migrate(context.Background()); err != nil {
		return nil, err
	}
	return b, nil
}

func (b *Bus) migrate(ctx context.Context) error {
	const ddl = `
CREATE TABLE IF NOT EXISTS events (
	seq INTEGER PRIMARY KEY AUTOINCREMENT,
	id TEXT NOT NULL UNIQUE,
	type TEXT NOT NULL DEFAULT '',
	process_id TEXT NOT NULL DEFAULT '',
	group_id TEXT NOT NULL DEFAULT '',
	session_id TEXT NOT NULL DEFAULT '',
	severity TEXT NOT NULL DEFAULT '',
	project_id TEXT NOT NULL DEFAULT '',
	message TEXT NOT NULL DEFAULT '',
	at_unix_nano INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS events_process ON events(process_id, seq);
CREATE INDEX IF NOT EXISTS events_at ON events(at_unix_nano);`
	if _, err := b.db.ExecContext(ctx, ddl); err != nil {
		return fmt.Errorf("event: migrate: %w", err)
	}
	return nil
}

// Append records an event, assigning an evt- ID and timestamp when empty and a
// monotonic Seq. Message is truncated to the payload cap. The returned Event
// carries the assigned fields.
func (b *Bus) Append(ctx context.Context, e Event) (Event, error) {
	if e.ID == "" {
		eid, err := id.New(id.Event)
		if err != nil {
			return Event{}, err
		}
		e.ID = eid
	}
	if e.At.IsZero() {
		e.At = time.Now().UTC()
	} else {
		e.At = e.At.UTC()
	}
	e.Message = truncate(e.Message, b.maxPayload)
	if b.db != nil {
		return b.appendSQL(ctx, e)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.seqCtr++
	e.Seq = b.seqCtr
	b.events = append(b.events, e)
	if len(b.events) > b.maxKeep {
		// Copy into a fresh backing array so evicted events (and their
		// strings) become unreachable rather than lingering behind the window.
		trimmed := make([]Event, b.maxKeep)
		copy(trimmed, b.events[len(b.events)-b.maxKeep:])
		b.events = trimmed
	}
	return e, nil
}

func (b *Bus) appendSQL(ctx context.Context, e Event) (Event, error) {
	res, err := b.db.ExecContext(ctx, `
INSERT INTO events (id, type, process_id, group_id, session_id, severity, project_id, message, at_unix_nano)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.ID, e.Type, e.ProcessID, e.GroupID, e.SessionID, e.Severity, e.ProjectID, e.Message, e.At.UnixNano(),
	)
	if err != nil {
		return Event{}, fmt.Errorf("event: append: %w", err)
	}
	seq, err := res.LastInsertId()
	if err != nil {
		return Event{}, fmt.Errorf("event: append seq: %w", err)
	}
	e.Seq = seq
	if err := b.sweep(ctx); err != nil {
		return Event{}, err
	}
	return e, nil
}

// sweep enforces the age and count retention bounds.
func (b *Bus) sweep(ctx context.Context) error {
	cutoff := time.Now().Add(-b.maxAge).UnixNano()
	if _, err := b.db.ExecContext(ctx, `DELETE FROM events WHERE at_unix_nano < ?`, cutoff); err != nil {
		return fmt.Errorf("event: sweep age: %w", err)
	}
	if _, err := b.db.ExecContext(ctx,
		`DELETE FROM events WHERE seq <= (SELECT seq FROM events ORDER BY seq DESC LIMIT 1 OFFSET ?)`,
		b.maxKeep,
	); err != nil {
		return fmt.Errorf("event: sweep count: %w", err)
	}
	return nil
}

// Query returns events newest-last (chronological), optionally filtered by
// process ID, capped at the newest limit (0 → 100).
func (b *Bus) Query(ctx context.Context, processID string, limit int) []Event {
	return b.query(ctx, processID, 0, limit)
}

// QuerySince returns events with Seq greater than sinceSeq in ascending order,
// optionally filtered by process ID, capped at limit (0 → 100). It lets a
// streaming consumer resume from a cursor without gaps or duplicates.
func (b *Bus) QuerySince(ctx context.Context, sinceSeq int64, processID string, limit int) []Event {
	if limit <= 0 {
		limit = 100
	}
	if b.db != nil {
		out, err := b.querySQLSince(ctx, sinceSeq, processID, limit)
		if err != nil {
			slog.Default().Error("event query since failed", "err", err)
			return nil
		}
		return out
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	var out []Event
	for _, e := range b.events {
		if e.Seq <= sinceSeq {
			continue
		}
		if processID != "" && e.ProcessID != processID {
			continue
		}
		out = append(out, e)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func (b *Bus) query(ctx context.Context, processID string, sinceSeq int64, limit int) []Event {
	if limit <= 0 {
		limit = 100
	}
	if b.db != nil {
		out, err := b.querySQLNewest(ctx, processID, sinceSeq, limit)
		if err != nil {
			slog.Default().Error("event query failed", "err", err)
			return nil
		}
		return out
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	var out []Event
	// sinceSeq is unused here: Query (the only caller) always passes 0, and
	// real events always have Seq >= 1, so a Seq-based filter would never
	// exclude anything. It still bounds the SQL path above.
	for _, e := range b.events {
		if processID != "" && e.ProcessID != processID {
			continue
		}
		out = append(out, e)
	}
	if len(out) > limit {
		out = out[len(out)-limit:]
	}
	cp := make([]Event, len(out))
	copy(cp, out)
	return cp
}

// querySQLNewest returns the newest limit rows in chronological (ascending) order.
func (b *Bus) querySQLNewest(ctx context.Context, processID string, sinceSeq int64, limit int) ([]Event, error) {
	rows, err := b.db.QueryContext(ctx, `
SELECT seq, id, type, process_id, group_id, session_id, severity, project_id, message, at_unix_nano
FROM (
	SELECT seq, id, type, process_id, group_id, session_id, severity, project_id, message, at_unix_nano
	FROM events
	WHERE (? = '' OR process_id = ?) AND seq > ?
	ORDER BY seq DESC
	LIMIT ?
) ORDER BY seq ASC`,
		processID, processID, sinceSeq, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("event: query: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanEvents(rows)
}

// querySQLSince returns rows after the cursor in ascending order.
func (b *Bus) querySQLSince(ctx context.Context, sinceSeq int64, processID string, limit int) ([]Event, error) {
	rows, err := b.db.QueryContext(ctx, `
SELECT seq, id, type, process_id, group_id, session_id, severity, project_id, message, at_unix_nano
FROM events
WHERE (? = '' OR process_id = ?) AND seq > ?
ORDER BY seq ASC
LIMIT ?`,
		processID, processID, sinceSeq, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("event: query since: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanEvents(rows)
}

func scanEvents(rows *sql.Rows) ([]Event, error) {
	var out []Event
	for rows.Next() {
		var (
			e     Event
			nanos int64
		)
		if err := rows.Scan(
			&e.Seq, &e.ID, &e.Type, &e.ProcessID, &e.GroupID, &e.SessionID,
			&e.Severity, &e.ProjectID, &e.Message, &nanos,
		); err != nil {
			return nil, fmt.Errorf("event: scan: %w", err)
		}
		e.At = time.Unix(0, nanos).UTC()
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("event: rows: %w", err)
	}
	return out, nil
}

// truncate bounds s to maxBytes without splitting a UTF-8 rune, appending a
// marker when it shortens the input.
func truncate(s string, maxBytes int) string {
	if maxBytes <= 0 || len(s) <= maxBytes {
		return s
	}
	cut := maxBytes
	for cut > 0 && !utf8RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + truncMarker
}

// utf8RuneStart reports whether b is the first byte of a UTF-8 rune.
func utf8RuneStart(b byte) bool {
	return b&0xC0 != 0x80
}
