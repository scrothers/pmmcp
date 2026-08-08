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
	"testing"

	"github.com/scrothers/pmmcp/internal/audit"
)

func TestMemRingEviction(t *testing.T) {
	t.Parallel()
	l := audit.New(3)
	ctx := context.Background()
	for i := range 5 {
		if _, err := l.Append(ctx, audit.Record{Action: "a", Target: "t", Detail: string(rune('a' + i))}); err != nil {
			t.Fatal(err)
		}
	}
	got := l.Query(ctx, "t", 100)
	if len(got) != 3 || got[0].Detail != "c" || got[2].Detail != "e" {
		t.Fatalf("ring = %+v", got)
	}
}

func TestMemFilterAndDefault(t *testing.T) {
	t.Parallel()
	l := audit.New(0) // defaults to DefaultMaxRecords
	ctx := context.Background()
	_, _ = l.Append(ctx, audit.Record{Action: "process.start", Actor: "alice", Target: "proc-1"})
	_, _ = l.Append(ctx, audit.Record{Action: "process.stop", Actor: "bob", Target: "proc-1"})

	if got := l.QueryFilter(ctx, audit.Filter{Actor: "alice"}, 100); len(got) != 1 {
		t.Fatalf("actor filter = %d, want 1", len(got))
	}
	if got := l.QueryFilter(ctx, audit.Filter{Actor: "nobody"}, 100); len(got) != 0 {
		t.Fatalf("miss = %d, want 0", len(got))
	}
	// Seq is assigned in memory too.
	if got := l.Query(ctx, "proc-1", 1); len(got) != 1 || got[0].Seq == 0 {
		t.Fatalf("seq = %+v", got)
	}
}
