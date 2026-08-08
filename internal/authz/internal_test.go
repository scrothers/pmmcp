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

package authz

import (
	"errors"
	"os/user"
	"testing"
)

// TestCurrentUserFromPropagatesLookupError exercises CurrentUser's error
// path by injecting a failing lookup directly into currentUserFrom, rather
// than swapping a package-level mutable global.
func TestCurrentUserFromPropagatesLookupError(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("lookup failed")
	lookup := func() (*user.User, error) { return nil, wantErr }

	p, err := currentUserFrom(lookup, RoleFull, "sess-1")
	if err == nil {
		t.Fatal("expected error when lookup fails")
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want wrapping %v", err, wantErr)
	}
	if p != (Principal{}) {
		t.Fatalf("expected zero-value principal on error, got %+v", p)
	}
}

func TestCurrentUserFromSuccess(t *testing.T) {
	t.Parallel()
	u := &user.User{Uid: "1000", Username: "steven"}
	lookup := func() (*user.User, error) { return u, nil }

	p, err := currentUserFrom(lookup, "", "sess-2")
	if err != nil {
		t.Fatalf("currentUserFrom: %v", err)
	}
	if p.Role != RoleAgent {
		t.Fatalf("empty role defaulted to %q, want agent", p.Role)
	}
	if p.UID != "1000" || p.Username != "steven" || p.Session != "sess-2" {
		t.Fatalf("principal = %+v", p)
	}

	p, err = currentUserFrom(lookup, RoleReadonly, "sess-3")
	if err != nil {
		t.Fatalf("currentUserFrom: %v", err)
	}
	if p.Role != RoleReadonly {
		t.Fatalf("role = %q, want readonly", p.Role)
	}
}
