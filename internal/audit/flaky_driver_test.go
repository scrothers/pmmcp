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
	"database/sql/driver"
	"errors"
	"sync"
	"sync/atomic"

	sqlitedrv "modernc.org/sqlite"
)

// flakyConnector wraps modernc's sqlite driver so a test can force the Nth
// Exec call, or the Result/Rows a statement returns, to fail, reaching
// db-error branches a healthy real connection cannot hit deterministically
// (a LastInsertId failure, a sweep delete failing after its insert
// succeeded, a Rows iteration that fails outright).
type flakyConnector struct {
	dsn string
	drv sqlitedrv.Driver

	mu          sync.Mutex
	failLastID  bool
	failNextRow bool

	execN      int64
	failExecAt int64 // 0 = disabled
}

func newFlakyConnector(dsn string) *flakyConnector { return &flakyConnector{dsn: dsn} }

func (c *flakyConnector) Connect(context.Context) (driver.Conn, error) {
	conn, err := c.drv.Open(c.dsn)
	if err != nil {
		return nil, err
	}
	return &flakyConn{Conn: conn, ctrl: c}, nil
}

func (c *flakyConnector) Driver() driver.Driver { return &c.drv }

func (c *flakyConnector) armLastID()  { c.mu.Lock(); c.failLastID = true; c.mu.Unlock() }
func (c *flakyConnector) armNextRow() { c.mu.Lock(); c.failNextRow = true; c.mu.Unlock() }

// armExecAt fails the nth Exec call (1-indexed) counting from the most
// recent resetExecCounter.
func (c *flakyConnector) armExecAt(n int64) { atomic.StoreInt64(&c.failExecAt, n) }

func (c *flakyConnector) resetExecCounter() { atomic.StoreInt64(&c.execN, 0) }

func (c *flakyConnector) consume(flag *bool) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	v := *flag
	*flag = false
	return v
}

func (c *flakyConnector) consumeExecAt() bool {
	target := atomic.LoadInt64(&c.failExecAt)
	if target == 0 {
		return false
	}
	return atomic.AddInt64(&c.execN, 1) == target
}

var (
	errFlakyExec         = errors.New("flaky: forced exec failure")
	errFlakyLastInsertID = errors.New("flaky: forced last-insert-id failure")
	errFlakyRowsNext     = errors.New("flaky: forced rows iteration failure")
)

type flakyConn struct {
	driver.Conn
	ctrl *flakyConnector
}

func (c *flakyConn) Prepare(query string) (driver.Stmt, error) {
	stmt, err := c.Conn.Prepare(query)
	if err != nil {
		return nil, err
	}
	return &flakyStmt{Stmt: stmt, ctrl: c.ctrl}, nil
}

type flakyStmt struct {
	driver.Stmt
	ctrl *flakyConnector
}

func (s *flakyStmt) Exec(args []driver.Value) (driver.Result, error) {
	if s.ctrl.consumeExecAt() {
		return nil, errFlakyExec
	}
	res, err := s.Stmt.Exec(args) //nolint:staticcheck // legacy driver.Stmt.Exec is the fallback path exercised here
	if err != nil {
		return nil, err
	}
	return &flakyResult{Result: res, ctrl: s.ctrl}, nil
}

func (s *flakyStmt) Query(args []driver.Value) (driver.Rows, error) {
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

func (r *flakyResult) LastInsertId() (int64, error) {
	if r.ctrl.consume(&r.ctrl.failLastID) {
		return 0, errFlakyLastInsertID
	}
	return r.Result.LastInsertId()
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
