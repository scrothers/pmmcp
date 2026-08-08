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

package daemon

import (
	"context"
	"database/sql"
	"os/user"

	"github.com/scrothers/pmmcp/internal/api"
	"github.com/scrothers/pmmcp/internal/audit"
	"github.com/scrothers/pmmcp/internal/authz"
	"github.com/scrothers/pmmcp/internal/event"
	"github.com/scrothers/pmmcp/internal/store/sqlite"
)

// SetUserCurrentForTest overrides the userCurrent seam used by New. The
// caller must invoke the returned restore func (e.g. via t.Cleanup) before
// the test ends.
func SetUserCurrentForTest(fn func() (*user.User, error)) (restore func()) {
	orig := userCurrent
	userCurrent = fn
	return func() { userCurrent = orig }
}

// SetMigrateStoreForTest overrides the migrateStore seam used by New.
func SetMigrateStoreForTest(fn func(context.Context, *sqlite.Store) error) (restore func()) {
	orig := migrateStore
	migrateStore = fn
	return func() { migrateStore = orig }
}

// SetNewAuditSQLiteLogForTest overrides the newAuditSQLiteLog seam used by New.
func SetNewAuditSQLiteLogForTest(fn func(*sql.DB, ...audit.Option) (*audit.Log, error)) (restore func()) {
	orig := newAuditSQLiteLog
	newAuditSQLiteLog = fn
	return func() { newAuditSQLiteLog = orig }
}

// SetNewEventSQLiteLogForTest overrides the newEventSQLiteLog seam used by New.
func SetNewEventSQLiteLogForTest(fn func(*sql.DB, ...event.Option) (*event.Bus, error)) (restore func()) {
	orig := newEventSQLiteLog
	newEventSQLiteLog = fn
	return func() { newEventSQLiteLog = orig }
}

// ErrFromForTest exposes the unexported errFrom error-code mapper for direct
// unit testing of its branches (some, like process.ErrSandboxFailed, aren't
// reachable through the public IPC surface in this sandbox without host
// toolchain conditions like a missing bwrap).
func ErrFromForTest(err error) api.Response { return errFrom(err) }

// SandboxIsRelaxationForTest exposes the unexported, pure sandboxIsRelaxation
// helper for direct table-driven testing of its rank() switch.
func SandboxIsRelaxationForTest(cfgDefault, requested string) bool {
	return sandboxIsRelaxation(cfgDefault, requested)
}

// JSONOKForTest exposes jsonOK for direct testing of its marshal-error branch.
func (s *Server) JSONOKForTest(v any) api.Response { return s.jsonOK(v) }

// PrincipalForTest exposes the unexported principal constructor.
func (s *Server) PrincipalForTest(role, session string) authz.Principal {
	return s.principal(role, session)
}

// RecordStartTimeForTest exposes recordStartTime for direct testing of its
// observability.ReadStartTime failure branch (a PID with no /proc entry).
func (s *Server) RecordStartTimeForTest(id string, pid int) { s.recordStartTime(id, pid) }

// StartTimeForTest returns whatever recordStartTime captured for id (0 if
// nothing was recorded), for asserting the failure branch had no side effect.
func (s *Server) StartTimeForTest(id string) uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.startTimes[id]
}
