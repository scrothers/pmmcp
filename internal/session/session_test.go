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

package session_test

import (
	"crypto/rand"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/scrothers/pmmcp/internal/session"
)

// failingReader always fails, used to force crypto/rand into an error path
// deterministically instead of relying on real entropy exhaustion.
type failingReader struct{}

func (failingReader) Read([]byte) (int, error) {
	return 0, errors.New("failingReader: forced failure")
}

// TestOpenPropagatesIDGenerationError swaps crypto/rand.Reader with a reader
// that always fails so Open's id.New error path is exercised deterministically.
// It intentionally does not call t.Parallel(): Go runs all non-parallel tests
// in a file to completion before any t.Parallel() test in that file resumes,
// so the global crypto/rand.Reader is restored before other tests touch it.
func TestOpenPropagatesIDGenerationError(t *testing.T) {
	orig := rand.Reader
	rand.Reader = failingReader{}
	defer func() { rand.Reader = orig }()

	r := session.NewRegistry()
	s, err := r.Open("", "agent")
	if err == nil {
		t.Fatal("expected error when id generation fails")
	}
	if s != nil {
		t.Fatalf("expected nil session on error, got %+v", s)
	}
}

func TestOpenHarnessPreferred(t *testing.T) {
	t.Parallel()
	r := session.NewRegistry()
	s, err := r.Open("claude-conv-123", "agent")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(s.ID, "sess-") {
		t.Fatalf("id %q", s.ID)
	}
	if s.PrimaryID() != "claude-conv-123" {
		t.Fatalf("primary %q", s.PrimaryID())
	}
	// PrimaryID falls back to internal id when no harness id.
	anon, err := r.Open("", "agent")
	if err != nil {
		t.Fatal(err)
	}
	if anon.PrimaryID() != anon.ID {
		t.Fatalf("anon primary %q, want %q", anon.PrimaryID(), anon.ID)
	}
}

func TestOpenReusesHarnessSession(t *testing.T) {
	t.Parallel()
	r := session.NewRegistry()
	first, err := r.Open("claude-conv-1", "agent")
	if err != nil {
		t.Fatal(err)
	}
	second, err := r.Open("claude-conv-1", "operator")
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID {
		t.Fatalf("repeated harness id minted a new session: %q vs %q", first.ID, second.ID)
	}
	// First-open role wins; a later Open must not silently escalate.
	if second.Role != "agent" {
		t.Fatalf("role changed on reuse: %q", second.Role)
	}
	// Distinct harness ids get distinct sessions.
	other, err := r.Open("claude-conv-2", "agent")
	if err != nil {
		t.Fatal(err)
	}
	if other.ID == first.ID {
		t.Fatal("distinct harness ids collapsed to one session")
	}
}

func TestAnonymousSessionsAreDistinct(t *testing.T) {
	t.Parallel()
	r := session.NewRegistry()
	a, err := r.Open("", "agent")
	if err != nil {
		t.Fatal(err)
	}
	b, err := r.Open("", "agent")
	if err != nil {
		t.Fatal(err)
	}
	if a.ID == b.ID {
		t.Fatal("anonymous opens must not be reused")
	}
}

func TestGetAndGetByHarness(t *testing.T) {
	t.Parallel()
	r := session.NewRegistry()
	s, err := r.Open("harness-x", "agent")
	if err != nil {
		t.Fatal(err)
	}
	got, ok := r.Get(s.ID)
	if !ok || got.ID != s.ID {
		t.Fatalf("Get: %+v ok=%v", got, ok)
	}
	// Returned value is a copy: mutating it must not affect the registry.
	got.Role = "tampered"
	again, _ := r.Get(s.ID)
	if again.Role != "agent" {
		t.Fatalf("Get returned a live pointer; role mutated to %q", again.Role)
	}
	byH, ok := r.GetByHarness("harness-x")
	if !ok || byH.ID != s.ID {
		t.Fatalf("GetByHarness: %+v ok=%v", byH, ok)
	}
	if _, ok := r.GetByHarness(""); ok {
		t.Fatal("empty harness id must not match")
	}
	if _, ok := r.Get("sess-nope"); ok {
		t.Fatal("unknown id must not match")
	}
}

func TestEndDeletesSession(t *testing.T) {
	t.Parallel()
	r := session.NewRegistry()
	s, err := r.Open("harness-e", "agent")
	if err != nil {
		t.Fatal(err)
	}
	if !r.End(s.ID) {
		t.Fatal("End should report the session existed")
	}
	if _, ok := r.Get(s.ID); ok {
		t.Fatal("session should be gone after End")
	}
	if _, ok := r.GetByHarness("harness-e"); ok {
		t.Fatal("harness index should be cleared after End")
	}
	if r.End(s.ID) {
		t.Fatal("End on an already-ended session should be false")
	}
	if r.End("sess-unknown") {
		t.Fatal("End on unknown id should be false")
	}
	// After End the harness id is free to open a fresh session.
	again, err := r.Open("harness-e", "agent")
	if err != nil {
		t.Fatal(err)
	}
	if again.ID == s.ID {
		t.Fatal("reopened session should be a new identity")
	}
}

func TestConcurrentOpenGetEnd(t *testing.T) {
	t.Parallel()
	r := session.NewRegistry()
	var wg sync.WaitGroup
	for i := range 40 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			s, err := r.Open("shared-harness", "agent")
			if err != nil {
				t.Error(err)
				return
			}
			_, _ = r.Get(s.ID)
			_, _ = r.GetByHarness("shared-harness")
			if n%3 == 0 {
				r.End(s.ID)
			}
		}(i)
	}
	wg.Wait()
}
