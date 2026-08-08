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

package profile_test

import (
	"context"
	"crypto/rand"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/scrothers/pmmcp/internal/domain"
	"github.com/scrothers/pmmcp/internal/profile"
)

// failingReader always fails, used to force crypto/rand into an error path
// deterministically instead of relying on real entropy exhaustion.
type failingReader struct{}

func (failingReader) Read([]byte) (int, error) {
	return 0, errors.New("failingReader: forced failure")
}

// TestCreatePropagatesIDGenerationError swaps crypto/rand.Reader with a
// reader that always fails so Create's id.New error path is exercised
// deterministically. It intentionally does not call t.Parallel(): Go runs
// all non-parallel tests in a file to completion before any t.Parallel()
// test in that file resumes, so the global crypto/rand.Reader is restored
// before other tests touch it.
func TestCreatePropagatesIDGenerationError(t *testing.T) {
	orig := rand.Reader
	rand.Reader = failingReader{}
	defer func() { rand.Reader = orig }()

	ctx := context.Background()
	s := profile.NewStore()
	p, err := s.Create(ctx, profile.Profile{Name: "dev", ProjectID: "proj-a"})
	if err == nil {
		t.Fatal("expected error when id generation fails")
	}
	if p.ID != "" || p.Name != "" || p.ProjectID != "" || p.Env != nil {
		t.Fatalf("expected zero-value profile on error, got %+v", p)
	}
}

func TestCRUD(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := profile.NewStore()

	p, err := s.Create(ctx, profile.Profile{
		Name:      "dev",
		ProjectID: "proj-a",
		Env:       map[string]string{"LOG_LEVEL": "debug"},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !strings.HasPrefix(p.ID, "prof-") {
		t.Fatalf("ID = %q, want prof- prefix", p.ID)
	}
	if p.Name != "dev" || p.ProjectID != "proj-a" {
		t.Fatalf("got %+v", p)
	}
	if p.Env["LOG_LEVEL"] != "debug" {
		t.Fatalf("env = %v", p.Env)
	}

	got, err := s.Get(ctx, p.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != p.ID {
		t.Fatalf("Get ID = %q", got.ID)
	}

	updated, err := s.Update(ctx, profile.Profile{
		ID:  p.ID,
		Env: map[string]string{"LOG_LEVEL": "info"},
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Env["LOG_LEVEL"] != "info" {
		t.Fatalf("env after update = %v", updated.Env)
	}

	list, err := s.List(ctx, "proj-a")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("List len = %d, want 1", len(list))
	}

	if err := s.Delete(ctx, p.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, err = s.Get(ctx, p.ID)
	if err == nil {
		t.Fatal("Get after delete: want error")
	}
	var de *domain.Error
	if !errors.As(err, &de) || de.Code != domain.CodeNotFound {
		t.Fatalf("Get after delete: %v", err)
	}
}

func TestCreateConflictAndDefaultName(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := profile.NewStore()

	a, err := s.Create(ctx, profile.Profile{ProjectID: "proj-x"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if a.Name != profile.DefaultName {
		t.Fatalf("Name = %q, want %q", a.Name, profile.DefaultName)
	}
	_, err = s.Create(ctx, profile.Profile{ProjectID: "proj-x", Name: "default"})
	if err == nil {
		t.Fatal("second Create: want conflict")
	}
	var de *domain.Error
	if !errors.As(err, &de) || de.Code != domain.CodeConflict {
		t.Fatalf("want conflict, got %v", err)
	}
}

func TestUseAndActive(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := profile.NewStore()

	if got := s.Active("sess-1"); got != profile.DefaultName {
		t.Fatalf("Active unset = %q", got)
	}
	if err := s.Use(ctx, "sess-1", "test"); err != nil {
		t.Fatalf("Use: %v", err)
	}
	if got := s.Active("sess-1"); got != "test" {
		t.Fatalf("Active = %q, want test", got)
	}
	if err := s.Use(ctx, "sess-1", ""); err != nil {
		t.Fatalf("Use empty: %v", err)
	}
	if got := s.Active("sess-1"); got != profile.DefaultName {
		t.Fatalf("Active after empty = %q", got)
	}
}

func TestInvalidName(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := profile.NewStore()
	_, err := s.Create(ctx, profile.Profile{Name: "Bad Name", ProjectID: "p"})
	if err == nil {
		t.Fatal("want invalid name error")
	}
}

func TestUpdateRename(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := profile.NewStore()
	p, err := s.Create(ctx, profile.Profile{Name: "old", ProjectID: "proj-a"})
	if err != nil {
		t.Fatal(err)
	}
	renamed, err := s.Update(ctx, profile.Profile{ID: p.ID, Name: "new"})
	if err != nil {
		t.Fatalf("rename: %v", err)
	}
	if renamed.Name != "new" {
		t.Fatalf("name = %q, want new", renamed.Name)
	}
	// The old key is freed: a new profile can take the old name.
	if _, err := s.Create(ctx, profile.Profile{Name: "old", ProjectID: "proj-a"}); err != nil {
		t.Fatalf("old name should be reusable after rename: %v", err)
	}
	// Renaming onto an existing name conflicts.
	_, err = s.Update(ctx, profile.Profile{ID: p.ID, Name: "old"})
	var de *domain.Error
	if !errors.As(err, &de) || de.Code != domain.CodeConflict {
		t.Fatalf("rename onto existing: want conflict, got %v", err)
	}
	// Invalid new name is rejected.
	_, err = s.Update(ctx, profile.Profile{ID: p.ID, Name: "Bad Name"})
	if !errors.As(err, &de) || de.Code != domain.CodeInvalidArgument {
		t.Fatalf("invalid rename: want invalid_argument, got %v", err)
	}
}

func TestUpdateRejectsProjectChange(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := profile.NewStore()
	p, err := s.Create(ctx, profile.Profile{Name: "dev", ProjectID: "proj-a"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Update(ctx, profile.Profile{ID: p.ID, ProjectID: "proj-b"})
	var de *domain.Error
	if !errors.As(err, &de) || de.Code != domain.CodeInvalidArgument {
		t.Fatalf("want invalid_argument for project change, got %v", err)
	}
	// Same ProjectID passes (idempotent).
	if _, err := s.Update(ctx, profile.Profile{ID: p.ID, ProjectID: "proj-a", Env: map[string]string{"X": "1"}}); err != nil {
		t.Fatalf("same project_id should succeed: %v", err)
	}
}

func TestListAllProjects(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := profile.NewStore()
	if _, err := s.Create(ctx, profile.Profile{Name: "a", ProjectID: "proj-a"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Create(ctx, profile.Profile{Name: "b", ProjectID: "proj-b"}); err != nil {
		t.Fatal(err)
	}
	all, err := s.List(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("List(all) len = %d, want 2", len(all))
	}
	scoped, err := s.List(ctx, "proj-a")
	if err != nil {
		t.Fatal(err)
	}
	if len(scoped) != 1 {
		t.Fatalf("List(proj-a) len = %d, want 1", len(scoped))
	}
}

func TestRemoveSession(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := profile.NewStore()
	if err := s.Use(ctx, "sess-1", "dev"); err != nil {
		t.Fatal(err)
	}
	if got := s.Active("sess-1"); got != "dev" {
		t.Fatalf("Active = %q", got)
	}
	s.RemoveSession("sess-1")
	if got := s.Active("sess-1"); got != profile.DefaultName {
		t.Fatalf("after remove Active = %q, want default", got)
	}
	// Removing an unknown session is a no-op.
	s.RemoveSession("sess-unknown")
}

func TestCanceledContext(t *testing.T) {
	t.Parallel()
	s := profile.NewStore()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.Create(ctx, profile.Profile{Name: "x", ProjectID: "p"}); err == nil {
		t.Fatal("Create: want canceled error")
	}
	if _, err := s.Get(ctx, "prof-x"); err == nil {
		t.Fatal("Get: want canceled error")
	}
	if _, err := s.Update(ctx, profile.Profile{ID: "prof-x"}); err == nil {
		t.Fatal("Update: want canceled error")
	}
	if err := s.Delete(ctx, "prof-x"); err == nil {
		t.Fatal("Delete: want canceled error")
	}
	if _, err := s.List(ctx, ""); err == nil {
		t.Fatal("List: want canceled error")
	}
	if err := s.Use(ctx, "sess-1", "dev"); err == nil {
		t.Fatal("Use: want canceled error")
	}
}

func TestCreateRequiresProjectID(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := profile.NewStore()
	_, err := s.Create(ctx, profile.Profile{Name: "dev"})
	var de *domain.Error
	if !errors.As(err, &de) || de.Code != domain.CodeInvalidArgument {
		t.Fatalf("want invalid_argument for missing project_id, got %v", err)
	}
}

func TestUpdateRequiresID(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := profile.NewStore()
	_, err := s.Update(ctx, profile.Profile{Name: "dev"})
	var de *domain.Error
	if !errors.As(err, &de) || de.Code != domain.CodeInvalidArgument {
		t.Fatalf("want invalid_argument for missing id, got %v", err)
	}
}

func TestUpdateNotFound(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := profile.NewStore()
	_, err := s.Update(ctx, profile.Profile{ID: "prof-missing"})
	var de *domain.Error
	if !errors.As(err, &de) || de.Code != domain.CodeNotFound {
		t.Fatalf("want not_found, got %v", err)
	}
}

func TestDeleteNotFound(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := profile.NewStore()
	err := s.Delete(ctx, "prof-missing")
	var de *domain.Error
	if !errors.As(err, &de) || de.Code != domain.CodeNotFound {
		t.Fatalf("want not_found, got %v", err)
	}
}

func TestUseRequiresSession(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := profile.NewStore()
	err := s.Use(ctx, "", "dev")
	var de *domain.Error
	if !errors.As(err, &de) || de.Code != domain.CodeInvalidArgument {
		t.Fatalf("want invalid_argument for missing session, got %v", err)
	}
}

func TestUseInvalidName(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := profile.NewStore()
	err := s.Use(ctx, "sess-1", "Bad Name")
	var de *domain.Error
	if !errors.As(err, &de) || de.Code != domain.CodeInvalidArgument {
		t.Fatalf("want invalid_argument for invalid name, got %v", err)
	}
}

func TestConcurrentAccess(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := profile.NewStore()
	var wg sync.WaitGroup
	for i := range 30 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			name := "p" + string(rune('a'+n%20))
			_, _ = s.Create(ctx, profile.Profile{Name: name, ProjectID: "proj"})
			_ = s.Use(ctx, "sess", name)
			_, _ = s.List(ctx, "proj")
			_ = s.Active("sess")
			s.RemoveSession("sess")
		}(i)
	}
	wg.Wait()
}
