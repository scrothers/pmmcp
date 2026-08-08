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
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/scrothers/pmmcp/internal/domain"
	"github.com/scrothers/pmmcp/internal/store"
	"github.com/scrothers/pmmcp/internal/store/sqlite"
)

func TestOpenPingFailure(t *testing.T) {
	t.Parallel()
	// The parent directory does not exist, so SQLite cannot create the file;
	// the failure surfaces on PingContext, not on the lazy sql.Open.
	_, err := sqlite.Open(filepath.Join(t.TempDir(), "no-such-dir", "x.db"))
	if err == nil {
		t.Fatal("expected open/ping failure")
	}
}

func TestIsUniqueFallback(t *testing.T) {
	t.Parallel()
	if !sqlite.IsUnique(errors.New("UNIQUE constraint failed: processes.id")) {
		t.Fatal("message-matching fallback should recognize UNIQUE")
	}
	if !sqlite.IsUnique(errors.New("unique violation")) {
		t.Fatal("message-matching fallback should recognize lowercase unique")
	}
	if sqlite.IsUnique(errors.New("some other failure")) {
		t.Fatal("unrelated error should not match")
	}
}

func TestCreateNilProcess(t *testing.T) {
	t.Parallel()
	s := openMigrated(t)
	if err := s.Create(context.Background(), nil); err == nil {
		t.Fatal("expected error for nil process")
	}
}

func TestCreateEmptyID(t *testing.T) {
	t.Parallel()
	s := openMigrated(t)
	p := sampleProcess(t, "web")
	p.ID = ""
	if err := s.Create(context.Background(), p); err == nil {
		t.Fatal("expected error for empty id")
	}
}

func TestCreateNonConflictError(t *testing.T) {
	t.Parallel()
	s := openMigrated(t)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	p := sampleProcess(t, "web")
	err := s.Create(context.Background(), p)
	if err == nil || errors.Is(err, store.ErrConflict) {
		t.Fatalf("create on closed db = %v, want non-conflict error", err)
	}
}

func TestUpdateNilOrEmptyID(t *testing.T) {
	t.Parallel()
	s := openMigrated(t)
	if err := s.Update(context.Background(), nil); err == nil {
		t.Fatal("expected error for nil process")
	}
	p := sampleProcess(t, "web")
	p.ID = ""
	if err := s.Update(context.Background(), p); err == nil {
		t.Fatal("expected error for empty id")
	}
}

func TestUpdateValidationFailure(t *testing.T) {
	t.Parallel()
	s := openMigrated(t)
	p := sampleProcess(t, "web")
	if err := s.Create(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	p.Name = ""
	if err := s.Update(context.Background(), p); !errors.Is(err, domain.ErrInvalidProcess) {
		t.Fatalf("update invalid = %v, want ErrInvalidProcess", err)
	}
}

func TestUpdateNonConflictError(t *testing.T) {
	t.Parallel()
	s := openMigrated(t)
	p := sampleProcess(t, "web")
	if err := s.Create(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	err := s.Update(context.Background(), p)
	if err == nil || errors.Is(err, store.ErrConflict) {
		t.Fatalf("update on closed db = %v, want non-conflict error", err)
	}
}

func TestUpdateWithCASNilOrEmptyID(t *testing.T) {
	t.Parallel()
	s := openMigrated(t)
	if err := s.UpdateWithCAS(context.Background(), nil); err == nil {
		t.Fatal("expected error for nil process")
	}
}

func TestUpdateWithCASValidationFailure(t *testing.T) {
	t.Parallel()
	s := openMigrated(t)
	p := sampleProcess(t, "web")
	if err := s.Create(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get(context.Background(), p.ID)
	if err != nil {
		t.Fatal(err)
	}
	got.Name = ""
	if err := s.UpdateWithCAS(context.Background(), got); !errors.Is(err, domain.ErrInvalidProcess) {
		t.Fatalf("cas invalid = %v, want ErrInvalidProcess", err)
	}
}

func TestUpdateWithCASTokenCollisionAvoidance(t *testing.T) {
	t.Parallel()
	s := openMigrated(t)
	ctx := context.Background()
	p := sampleProcess(t, "web")
	if err := s.Create(ctx, p); err != nil {
		t.Fatal(err)
	}
	// Push the persisted updated_at into the future so time.Now() during CAS
	// is not After(token), exercising the nanosecond-bump collision guard.
	// Must match the store's fixed-width layout (fmtTime): RFC3339Nano would
	// strip trailing-zero fractional digits and silently desync the token.
	const tsLayout = "2006-01-02T15:04:05.000000000Z07:00"
	future := time.Now().UTC().Add(time.Hour)
	if _, err := s.DB().ExecContext(ctx, `UPDATE processes SET updated_at = ? WHERE id = ?`,
		future.Format(tsLayout), p.ID); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !got.UpdatedAt.Equal(future) {
		t.Fatalf("updated_at = %v, want %v", got.UpdatedAt, future)
	}
	got.LastError = "bump"
	if err := s.UpdateWithCAS(ctx, got); err != nil {
		t.Fatalf("cas with future token: %v", err)
	}
	if !got.UpdatedAt.After(future) {
		t.Fatalf("new token %v should be after old future token %v", got.UpdatedAt, future)
	}
}

func TestUpdateWithCASConflictOnDuplicateName(t *testing.T) {
	t.Parallel()
	s := openMigrated(t)
	ctx := context.Background()
	a := sampleProcess(t, "web")
	a.ProjectID = "p"
	b := sampleProcess(t, "api")
	b.ProjectID = "p"
	if err := s.Create(ctx, a); err != nil {
		t.Fatal(err)
	}
	if err := s.Create(ctx, b); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get(ctx, b.ID)
	if err != nil {
		t.Fatal(err)
	}
	got.Name = "web" // collides with a's live name
	if err := s.UpdateWithCAS(ctx, got); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("cas dup name = %v, want ErrConflict", err)
	}
}

func TestUpdateWithCASNonConflictError(t *testing.T) {
	t.Parallel()
	s := openMigrated(t)
	ctx := context.Background()
	p := sampleProcess(t, "web")
	if err := s.Create(ctx, p); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	err = s.UpdateWithCAS(ctx, got)
	if err == nil || errors.Is(err, store.ErrConflict) {
		t.Fatalf("cas on closed db = %v, want non-conflict error", err)
	}
}

func TestDeleteExecError(t *testing.T) {
	t.Parallel()
	s := openMigrated(t)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(context.Background(), "proc-missing"); err == nil {
		t.Fatal("expected exec error on closed db")
	}
}

func TestListQueryError(t *testing.T) {
	t.Parallel()
	s := openMigrated(t)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.List(context.Background(), store.ProcessFilter{}); err == nil {
		t.Fatal("expected query error on closed db")
	}
}

func TestListScanError(t *testing.T) {
	t.Parallel()
	s := openMigrated(t)
	ctx := context.Background()
	p := sampleProcess(t, "web")
	if err := s.Create(ctx, p); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB().ExecContext(ctx, `UPDATE processes SET command_json = ? WHERE id = ?`, "{not json", p.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.List(ctx, store.ProcessFilter{}); err == nil {
		t.Fatal("expected scan error from corrupt row")
	}
}
