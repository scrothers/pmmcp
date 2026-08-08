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
	"database/sql/driver"
	"errors"
	"sync"

	sqlitedrv "modernc.org/sqlite"
)

// The errors below are forced by flakyConnector to reach db-error branches
// that a real, healthy on-disk SQLite database cannot be made to hit
// deterministically (a mid-migration BeginTx or Commit failure, a Result
// whose RowsAffected errors, a Rows.Next that fails outright, a query
// issued after a prior statement already succeeded).
var (
	errFlakyBegin        = errors.New("flaky: forced begin failure")
	errFlakyCommit       = errors.New("flaky: forced commit failure")
	errFlakyQuery        = errors.New("flaky: forced query failure")
	errFlakyRowsAffected = errors.New("flaky: forced rows-affected failure")
	errFlakyRowsNext     = errors.New("flaky: forced rows iteration failure")
)

// flakyConnector wraps modernc's sqlite driver so a test can arm the next
// Begin/Commit/Query call, or the Result/Rows a statement returns, to fail,
// then let everything else pass through untouched to a real SQLite
// connection. All arm/consume state is guarded by mu so armed tests using
// t.Parallel() are race-safe (each test owns its own connector instance).
type flakyConnector struct {
	dsn string
	drv sqlitedrv.Driver

	mu             sync.Mutex
	failBegin      bool
	failCommit     bool
	failQuery      bool
	failRowsAffect bool
	failNextRow    bool
}

func newFlakyConnector(dsn string) *flakyConnector {
	return &flakyConnector{dsn: dsn}
}

func (c *flakyConnector) Connect(context.Context) (driver.Conn, error) {
	conn, err := c.drv.Open(c.dsn)
	if err != nil {
		return nil, err
	}
	return &flakyConn{Conn: conn, ctrl: c}, nil
}

func (c *flakyConnector) Driver() driver.Driver { return &c.drv }

func (c *flakyConnector) armBegin()      { c.mu.Lock(); c.failBegin = true; c.mu.Unlock() }
func (c *flakyConnector) armCommit()     { c.mu.Lock(); c.failCommit = true; c.mu.Unlock() }
func (c *flakyConnector) armQuery()      { c.mu.Lock(); c.failQuery = true; c.mu.Unlock() }
func (c *flakyConnector) armRowsAffect() { c.mu.Lock(); c.failRowsAffect = true; c.mu.Unlock() }
func (c *flakyConnector) armNextRow()    { c.mu.Lock(); c.failNextRow = true; c.mu.Unlock() }

func (c *flakyConnector) consume(flag *bool) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	v := *flag
	*flag = false
	return v
}

type flakyConn struct {
	driver.Conn
	ctrl *flakyConnector
}

func (c *flakyConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	if c.ctrl.consume(&c.ctrl.failBegin) {
		return nil, errFlakyBegin
	}
	cb, ok := c.Conn.(driver.ConnBeginTx)
	if !ok {
		return nil, errors.New("flaky: conn does not support BeginTx")
	}
	tx, err := cb.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return &flakyTx{Tx: tx, ctrl: c.ctrl}, nil
}

func (c *flakyConn) Prepare(query string) (driver.Stmt, error) {
	stmt, err := c.Conn.Prepare(query)
	if err != nil {
		return nil, err
	}
	return &flakyStmt{Stmt: stmt, ctrl: c.ctrl}, nil
}

type flakyTx struct {
	driver.Tx
	ctrl *flakyConnector
}

func (t *flakyTx) Commit() error {
	if t.ctrl.consume(&t.ctrl.failCommit) {
		_ = t.Rollback()
		return errFlakyCommit
	}
	return t.Tx.Commit()
}

type flakyStmt struct {
	driver.Stmt
	ctrl *flakyConnector
}

func (s *flakyStmt) Exec(args []driver.Value) (driver.Result, error) {
	res, err := s.Stmt.Exec(args) //nolint:staticcheck // legacy driver.Stmt.Exec is the fallback path exercised here
	if err != nil {
		return nil, err
	}
	return &flakyResult{Result: res, ctrl: s.ctrl}, nil
}

func (s *flakyStmt) Query(args []driver.Value) (driver.Rows, error) {
	if s.ctrl.consume(&s.ctrl.failQuery) {
		return nil, errFlakyQuery
	}
	rows, err := s.Stmt.Query(args) //nolint:staticcheck // legacy driver.Stmt.Query is the fallback path exercised here
	if err != nil {
		return nil, err
	}
	return &flakyRows{Rows: rows, ctrl: s.ctrl}, nil
}

type flakyResult struct {
	driver.Result
	ctrl *flakyConnector
}

func (r *flakyResult) RowsAffected() (int64, error) {
	if r.ctrl.consume(&r.ctrl.failRowsAffect) {
		return 0, errFlakyRowsAffected
	}
	return r.Result.RowsAffected()
}

type flakyRows struct {
	driver.Rows
	ctrl *flakyConnector
}

func (r *flakyRows) Next(dest []driver.Value) error {
	if r.ctrl.consume(&r.ctrl.failNextRow) {
		return errFlakyRowsNext
	}
	return r.Rows.Next(dest)
}
