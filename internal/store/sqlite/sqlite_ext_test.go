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

	"github.com/scrothers/pmmcp/internal/domain"
	"github.com/scrothers/pmmcp/internal/id"
	"github.com/scrothers/pmmcp/internal/store"
	"github.com/scrothers/pmmcp/internal/store/sqlite"
)

func TestUpdateNotFound(t *testing.T) {
	t.Parallel()
	s := openMigrated(t)
	p := sampleProcess(t, "ghost")
	if err := s.Update(context.Background(), p); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("update missing = %v, want ErrNotFound", err)
	}
}

func TestUpdateWithCAS(t *testing.T) {
	t.Parallel()
	s := openMigrated(t)
	ctx := context.Background()
	p := sampleProcess(t, "web")
	if err := s.Create(ctx, p); err != nil {
		t.Fatal(err)
	}

	// Two independent reads share the same optimistic token.
	c1, err := s.Get(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	c2, err := s.Get(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}

	c1.Status = domain.StatusStopping
	if err := s.UpdateWithCAS(ctx, c1); err != nil {
		t.Fatalf("first cas: %v", err)
	}
	// c2's token is now stale.
	c2.Status = domain.StatusExited
	if err := s.UpdateWithCAS(ctx, c2); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("stale cas = %v, want ErrConflict", err)
	}
	// c1's token advanced on success, so it can update again.
	c1.LastError = "boom"
	if err := s.UpdateWithCAS(ctx, c1); err != nil {
		t.Fatalf("second cas: %v", err)
	}
}

func TestUpdateWithCASMissingRow(t *testing.T) {
	t.Parallel()
	s := openMigrated(t)
	p := sampleProcess(t, "ghost")
	p.UpdatedAt = time.Now().UTC()
	if err := s.UpdateWithCAS(context.Background(), p); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("cas missing = %v, want ErrNotFound", err)
	}
}

func TestUpdateWithCASMissingToken(t *testing.T) {
	t.Parallel()
	s := openMigrated(t)
	p := sampleProcess(t, "web")
	if err := s.Create(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	p.UpdatedAt = time.Time{}
	if err := s.UpdateWithCAS(context.Background(), p); err == nil {
		t.Fatal("cas without token should error")
	}
}

func TestLiveNameUniqueness(t *testing.T) {
	t.Parallel()
	s := openMigrated(t)
	ctx := context.Background()
	a := sampleProcess(t, "web")
	a.ProjectID = "proj-1"
	if err := s.Create(ctx, a); err != nil {
		t.Fatal(err)
	}
	// Second live process with the same (project, name) conflicts.
	b := sampleProcess(t, "web")
	b.ProjectID = "proj-1"
	if err := s.Create(ctx, b); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("duplicate live name = %v, want ErrConflict", err)
	}
	// Retiring a's generation (giving it a successor) frees the name.
	a.SuccessorID = "proc-successor"
	if err := s.Update(ctx, a); err != nil {
		t.Fatal(err)
	}
	if err := s.Create(ctx, b); err != nil {
		t.Fatalf("create after retire: %v", err)
	}
	// A different project may reuse the name freely.
	c := sampleProcess(t, "web")
	c.ProjectID = "proj-2"
	if err := s.Create(ctx, c); err != nil {
		t.Fatalf("other project: %v", err)
	}
}

func TestListNameAndCombinedFilter(t *testing.T) {
	t.Parallel()
	s := openMigrated(t)
	ctx := context.Background()
	a := sampleProcess(t, "web")
	a.ProjectID = "proj-1"
	b := sampleProcess(t, "worker")
	b.ProjectID = "proj-1"
	c := sampleProcess(t, "web")
	c.ProjectID = "proj-2"
	for _, p := range []*domain.Process{a, b, c} {
		if err := s.Create(ctx, p); err != nil {
			t.Fatal(err)
		}
	}
	byName, err := s.List(ctx, store.ProcessFilter{Name: "web"})
	if err != nil {
		t.Fatal(err)
	}
	if len(byName) != 2 {
		t.Fatalf("name filter len = %d, want 2", len(byName))
	}
	combined, err := s.List(ctx, store.ProcessFilter{ProjectID: "proj-1", Name: "web"})
	if err != nil {
		t.Fatal(err)
	}
	if len(combined) != 1 || combined[0].ID != a.ID {
		t.Fatalf("combined filter = %+v", combined)
	}
}

func TestTimestampRoundTripAndOrdering(t *testing.T) {
	t.Parallel()
	s := openMigrated(t)
	ctx := context.Background()
	base := time.Date(2020, 1, 1, 12, 0, 0, 0, time.UTC) // exactly zero nanoseconds
	mid := base.Add(500 * time.Millisecond)
	late := base.Add(999 * time.Millisecond)
	started := base.Add(2 * time.Second)
	exited := base.Add(3 * time.Second)

	// Insert shuffled; the zero-nanosecond value must still sort first.
	for i, at := range []time.Time{mid, base, late} {
		p := sampleProcess(t, "p")
		p.ProjectID = "ord"
		p.Name = "n" + string(rune('a'+i))
		p.CreatedAt = at
		if i == 1 {
			p.StartedAt = &started
			p.ExitedAt = &exited
		}
		if err := s.Create(ctx, p); err != nil {
			t.Fatal(err)
		}
	}
	list, err := s.List(ctx, store.ProcessFilter{ProjectID: "ord"})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 3 {
		t.Fatalf("len = %d", len(list))
	}
	if !list[0].CreatedAt.Equal(base) || !list[1].CreatedAt.Equal(mid) || !list[2].CreatedAt.Equal(late) {
		t.Fatalf("order = %v, %v, %v", list[0].CreatedAt, list[1].CreatedAt, list[2].CreatedAt)
	}
	if list[0].StartedAt == nil || !list[0].StartedAt.Equal(started) {
		t.Fatalf("started round-trip = %v", list[0].StartedAt)
	}
	if list[0].ExitedAt == nil || !list[0].ExitedAt.Equal(exited) {
		t.Fatalf("exited round-trip = %v", list[0].ExitedAt)
	}
}

func TestEnvKeysAndChainRoundTrip(t *testing.T) {
	t.Parallel()
	s := openMigrated(t)
	ctx := context.Background()
	p := sampleProcess(t, "web")
	p.EnvKeys = []string{"PATH", "HOME"}
	p.PredecessorID = "proc-old"
	p.SuccessorID = "proc-new"
	if err := s.Create(ctx, p); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.EnvKeys) != 2 || got.EnvKeys[0] != "PATH" || got.EnvKeys[1] != "HOME" {
		t.Fatalf("env keys = %#v", got.EnvKeys)
	}
	if got.PredecessorID != "proc-old" || got.SuccessorID != "proc-new" {
		t.Fatalf("chain = %q/%q", got.PredecessorID, got.SuccessorID)
	}
}

func TestCreateValidationFailure(t *testing.T) {
	t.Parallel()
	s := openMigrated(t)
	pid, err := id.New(id.Proc)
	if err != nil {
		t.Fatal(err)
	}
	// Empty name fails domain validation before any SQL runs.
	p := &domain.Process{ID: pid, Command: []string{"sleep", "1"}}
	if err := s.Create(context.Background(), p); !errors.Is(err, domain.ErrInvalidProcess) {
		t.Fatalf("create invalid = %v, want ErrInvalidProcess", err)
	}
}

func TestCloseNilAndDouble(t *testing.T) {
	t.Parallel()
	var nilStore *sqlite.Store
	if err := nilStore.Close(); err != nil {
		t.Fatalf("nil close = %v", err)
	}
	s := openMigrated(t)
	if err := s.Close(); err != nil {
		t.Fatalf("first close = %v", err)
	}
	// Second close is handled by the shared cleanup; ensure it does not panic.
	_ = s.Close()
}

func TestConcurrentCreateUpdate(t *testing.T) {
	t.Parallel()
	s := openMigrated(t)
	ctx := context.Background()
	const n = 20
	errCh := make(chan error, n)
	for i := range n {
		go func(i int) {
			p := sampleProcess(t, "c")
			p.Name = "proc" + string(rune('a'+i%26)) + string(rune('0'+i/26))
			p.ProjectID = "conc"
			if err := s.Create(ctx, p); err != nil {
				errCh <- err
				return
			}
			p.Status = domain.StatusStopping
			errCh <- s.Update(ctx, p)
		}(i)
	}
	for range n {
		if err := <-errCh; err != nil {
			t.Fatalf("concurrent op: %v", err)
		}
	}
}
