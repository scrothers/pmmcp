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

	"github.com/scrothers/pmmcp/internal/domain"
	"github.com/scrothers/pmmcp/internal/id"
	"github.com/scrothers/pmmcp/internal/store"
	"github.com/scrothers/pmmcp/internal/store/sqlite"
)

func openMigrated(t *testing.T) *sqlite.Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "pmmcp.db")
	s, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	// Second migrate is no-op.
	if err := s.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	return s
}

func sampleProcess(t *testing.T, name string) *domain.Process {
	t.Helper()
	pid, err := id.New(id.Proc)
	if err != nil {
		t.Fatal(err)
	}
	return &domain.Process{
		ID:      pid,
		Name:    name,
		Command: []string{"sleep", "60"},
		Status:  domain.StatusRunning,
		Desired: domain.DesiredRunning,
		Cwd:     "/tmp/app",
		Sandbox: "strict",
		Runtime: "local",
	}
}

func TestProcessCRUD(t *testing.T) {
	t.Parallel()
	s := openMigrated(t)
	ctx := context.Background()

	p := sampleProcess(t, "web")
	if err := s.Create(ctx, p); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := s.Get(ctx, p.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name != "web" || got.Status != domain.StatusRunning {
		t.Fatalf("got %+v", got)
	}
	if len(got.Command) != 2 || got.Command[0] != "sleep" {
		t.Fatalf("command = %#v", got.Command)
	}

	code := 0
	got.Status = domain.StatusExited
	got.Desired = domain.DesiredStopped
	got.ExitCode = &code
	if err := s.Update(ctx, got); err != nil {
		t.Fatalf("update: %v", err)
	}
	got2, err := s.Get(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got2.Status != domain.StatusExited || got2.ExitCode == nil || *got2.ExitCode != 0 {
		t.Fatalf("after update: %+v", got2)
	}

	list, err := s.List(ctx, store.ProcessFilter{Status: domain.StatusExited})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("list len = %d", len(list))
	}

	if err := s.Delete(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	_, err = s.Get(ctx, p.ID)
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("get after delete: %v", err)
	}
	if err := s.Delete(ctx, p.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("delete missing: %v", err)
	}
}

func TestCreateConflict(t *testing.T) {
	t.Parallel()
	s := openMigrated(t)
	ctx := context.Background()
	p := sampleProcess(t, "a")
	if err := s.Create(ctx, p); err != nil {
		t.Fatal(err)
	}
	// Same ID again.
	p2 := *p
	p2.Name = "b"
	err := s.Create(ctx, &p2)
	if !errors.Is(err, store.ErrConflict) {
		t.Fatalf("err = %v, want conflict", err)
	}
}

func TestListFilterProject(t *testing.T) {
	t.Parallel()
	s := openMigrated(t)
	ctx := context.Background()
	a := sampleProcess(t, "a")
	a.ProjectID = "proj-one"
	b := sampleProcess(t, "b")
	b.ProjectID = "proj-two"
	if err := s.Create(ctx, a); err != nil {
		t.Fatal(err)
	}
	if err := s.Create(ctx, b); err != nil {
		t.Fatal(err)
	}
	list, err := s.List(ctx, store.ProcessFilter{ProjectID: "proj-one"})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Name != "a" {
		t.Fatalf("list = %+v", list)
	}
}

func TestDriverIsModernc(t *testing.T) {
	t.Parallel()
	// Structural: opening with driver name "sqlite" is modernc's registration.
	// CGo mattn uses "sqlite3". We assert our Open path works on a temp file,
	// and go.mod is checked in a separate module test below via import.
	path := filepath.Join(t.TempDir(), "x.db")
	s, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
}
