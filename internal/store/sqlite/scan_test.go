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

	"github.com/scrothers/pmmcp/internal/store"
)

func TestDBAccessor(t *testing.T) {
	t.Parallel()
	s := openMigrated(t)
	if s.DB() == nil {
		t.Fatal("DB() returned nil")
	}
	if err := s.DB().PingContext(context.Background()); err != nil {
		t.Fatalf("ping via DB(): %v", err)
	}
}

func TestRuntimeDefault(t *testing.T) {
	t.Parallel()
	s := openMigrated(t)
	ctx := context.Background()
	p := sampleProcess(t, "web")
	p.Runtime = "" // empty normalizes to "local"
	if err := s.Create(ctx, p); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Runtime != "local" {
		t.Fatalf("runtime = %q, want local", got.Runtime)
	}
}

func TestUpdateDuplicateLiveNameConflicts(t *testing.T) {
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
	// Renaming b onto a's live name violates the partial unique index.
	b.Name = "web"
	if err := s.Update(ctx, b); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("update dup name = %v, want ErrConflict", err)
	}
}

func TestScanCorruptRows(t *testing.T) {
	t.Parallel()
	s := openMigrated(t)
	ctx := context.Background()

	cases := []struct {
		name string
		col  string
		val  string
	}{
		{"command", "command_json", "{not json"},
		{"env_keys", "env_keys_json", "{not json"},
		{"created_at", "created_at", "not-a-time"},
		{"updated_at", "updated_at", "not-a-time"},
		{"started_at", "started_at", "not-a-time"},
		{"exited_at", "exited_at", "not-a-time"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := sampleProcess(t, "corrupt-"+tc.name)
			if err := s.Create(ctx, p); err != nil {
				t.Fatal(err)
			}
			if _, err := s.DB().ExecContext(ctx, "UPDATE processes SET "+tc.col+" = ? WHERE id = ?", tc.val, p.ID); err != nil {
				t.Fatal(err)
			}
			if _, err := s.Get(ctx, p.ID); err == nil {
				t.Fatalf("expected scan error for corrupt %s", tc.col)
			}
		})
	}
}
