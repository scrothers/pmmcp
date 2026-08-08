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
	"testing"
	"time"

	"github.com/scrothers/pmmcp/internal/store"
)

// These exercise db-error branches that a healthy, real on-disk SQLite
// connection cannot be made to hit deterministically: a successful Exec
// whose Result.RowsAffected/LastInsertId errors, a Rows iteration that fails
// outright, and a query issued after a prior statement already succeeded.
// See flaky_driver_test.go for the driver.

func TestUpdateRowsAffectedError(t *testing.T) {
	t.Parallel()
	s, c := openFlaky(t)
	ctx := context.Background()
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	p := sampleProcess(t, "web")
	if err := s.Create(ctx, p); err != nil {
		t.Fatal(err)
	}
	c.armRowsAffect()
	if err := s.Update(ctx, p); err == nil {
		t.Fatal("expected rows-affected error")
	}
}

func TestUpdateWithCASRowsAffectedError(t *testing.T) {
	t.Parallel()
	s, c := openFlaky(t)
	ctx := context.Background()
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	p := sampleProcess(t, "web")
	if err := s.Create(ctx, p); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	c.armRowsAffect()
	if err := s.UpdateWithCAS(ctx, got); err == nil {
		t.Fatal("expected rows-affected error")
	}
}

func TestUpdateWithCASExistenceCheckError(t *testing.T) {
	t.Parallel()
	s, c := openFlaky(t)
	ctx := context.Background()
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	p := sampleProcess(t, "web")
	if err := s.Create(ctx, p); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	// Stale token: the UPDATE...WHERE id=? AND updated_at=? matches 0 rows
	// (an Exec, unaffected by armQuery), driving UpdateWithCAS into the
	// existence-check SELECT (a Query), which armQuery forces to fail.
	got.UpdatedAt = got.UpdatedAt.Add(-time.Hour)
	c.armQuery()
	if err := s.UpdateWithCAS(ctx, got); err == nil {
		t.Fatal("expected existence-check query error")
	} else if errors.Is(err, store.ErrNotFound) || errors.Is(err, store.ErrConflict) {
		t.Fatalf("err = %v, want a bare query error", err)
	}
}

func TestDeleteRowsAffectedError(t *testing.T) {
	t.Parallel()
	s, c := openFlaky(t)
	ctx := context.Background()
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	c.armRowsAffect()
	if err := s.Delete(ctx, "proc-anything"); err == nil {
		t.Fatal("expected rows-affected error")
	}
}

func TestListRowsIterationError(t *testing.T) {
	t.Parallel()
	s, c := openFlaky(t)
	ctx := context.Background()
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	c.armNextRow()
	if _, err := s.List(ctx, store.ProcessFilter{}); err == nil {
		t.Fatal("expected rows iteration error")
	}
}
