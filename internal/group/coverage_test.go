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

package group_test

import (
	"crypto/rand"
	"errors"
	"testing"

	"github.com/scrothers/pmmcp/internal/group"
)

// TestCreateEmptyName covers the empty-name validation error.
func TestCreateEmptyName(t *testing.T) {
	t.Parallel()
	r := group.NewRegistry()
	if _, err := r.Create(group.Group{Name: ""}); err == nil {
		t.Fatal("expected error for empty name")
	}
}

// TestListFiltersByProject covers the projectID-mismatch continue branch in
// List.
func TestListFiltersByProject(t *testing.T) {
	t.Parallel()
	r := group.NewRegistry()
	if _, err := r.Create(group.Group{Name: "a", ProjectID: "proj-a"}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Create(group.Group{Name: "b", ProjectID: "proj-b"}); err != nil {
		t.Fatal(err)
	}
	got := r.List("proj-a")
	if len(got) != 1 || got[0].Name != "a" {
		t.Fatalf("List(proj-a) = %+v, want only group a", got)
	}
}

// TestStartOrderNotFound covers the not-found branch in StartOrder.
func TestStartOrderNotFound(t *testing.T) {
	t.Parallel()
	r := group.NewRegistry()
	if _, err := r.StartOrder("grp-missing"); !errors.Is(err, group.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

// TestStopOrderNotFound covers StopOrder's propagation of StartOrder's error.
func TestStopOrderNotFound(t *testing.T) {
	t.Parallel()
	r := group.NewRegistry()
	if _, err := r.StopOrder("grp-missing"); !errors.Is(err, group.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

// brokenReader always fails, simulating crypto/rand exhaustion.
type brokenReader struct{}

func (brokenReader) Read([]byte) (int, error) { return 0, errors.New("entropy unavailable") }

// TestCreateIDGenerationFailure covers the id.New error branch in Create by
// swapping crypto/rand.Reader for a reader that always fails. This mutates
// process-global state, so the test must not run in parallel — Go's testing
// package runs all non-parallel tests to completion (including their
// t.Cleanup) before any t.Parallel() test resumes, so the swap cannot race
// with the package's other (parallel) tests.
func TestCreateIDGenerationFailure(t *testing.T) {
	prev := rand.Reader
	rand.Reader = brokenReader{}
	t.Cleanup(func() { rand.Reader = prev })

	r := group.NewRegistry()
	if _, err := r.Create(group.Group{Name: "x"}); err == nil {
		t.Fatal("expected error when ID generation fails")
	}
}
