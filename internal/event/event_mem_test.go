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

package event_test

import (
	"context"
	"testing"

	"github.com/scrothers/pmmcp/internal/event"
)

func TestMemRingEviction(t *testing.T) {
	t.Parallel()
	b := event.NewBus(3)
	ctx := context.Background()
	for i := range 5 {
		if _, err := b.Append(ctx, event.Event{Type: "t", Message: string(rune('a' + i))}); err != nil {
			t.Fatal(err)
		}
	}
	got := b.Query(ctx, "", 100)
	if len(got) != 3 || got[0].Message != "c" || got[2].Message != "e" {
		t.Fatalf("ring = %+v", got)
	}
}

func TestMemQuerySince(t *testing.T) {
	t.Parallel()
	b := event.NewBus(100)
	ctx := context.Background()
	seqs := make([]int64, 0, 4)
	for range 4 {
		e, err := b.Append(ctx, event.Event{Type: "t"})
		if err != nil {
			t.Fatal(err)
		}
		seqs = append(seqs, e.Seq)
	}
	tail := b.QuerySince(ctx, seqs[1], "", 100)
	if len(tail) != 2 || tail[0].Seq != seqs[2] {
		t.Fatalf("since = %+v", tail)
	}
	// Limit is honored.
	if got := b.QuerySince(ctx, 0, "", 1); len(got) != 1 {
		t.Fatalf("limit = %d", len(got))
	}
}

func TestMemQueryLimitAndDefault(t *testing.T) {
	t.Parallel()
	b := event.NewBus(0) // defaults to DefaultMaxEvents
	ctx := context.Background()
	for range 3 {
		if _, err := b.Append(ctx, event.Event{Type: "t", ProcessID: "p"}); err != nil {
			t.Fatal(err)
		}
	}
	if got := b.Query(ctx, "p", 2); len(got) != 2 {
		t.Fatalf("limit = %d, want 2", len(got))
	}
	if got := b.Query(ctx, "", 0); len(got) != 3 { // 0 → default 100
		t.Fatalf("default limit = %d", len(got))
	}
}
