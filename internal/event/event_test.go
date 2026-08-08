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
	"strings"
	"testing"

	"github.com/scrothers/pmmcp/internal/event"
)

func TestBusAppendQuery(t *testing.T) {
	t.Parallel()
	b := event.NewBus(100)
	ctx := context.Background()
	e, err := b.Append(ctx, event.Event{Type: "process.started", ProcessID: "proc-x", Message: "hi"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(e.ID, "evt-") {
		t.Fatalf("id = %q", e.ID)
	}
	if e.At.IsZero() {
		t.Fatal("zero time")
	}
	got := b.Query(ctx, "proc-x", 10)
	if len(got) != 1 || got[0].Type != "process.started" {
		t.Fatalf("%+v", got)
	}
	if len(b.Query(ctx, "other", 10)) != 0 {
		t.Fatal("filter failed")
	}
}
