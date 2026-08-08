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

package authz_test

import (
	"strings"
	"testing"

	"github.com/scrothers/pmmcp/internal/authz"
)

func TestRoleCaps(t *testing.T) {
	t.Parallel()
	if !authz.Allow(authz.Principal{Role: authz.RoleAgent}, authz.CapProcessStart) {
		t.Fatal("agent should start")
	}
	if authz.Allow(authz.Principal{Role: authz.RoleReadonly}, authz.CapProcessStop) {
		t.Fatal("readonly must not stop")
	}
	if authz.Allow(authz.Principal{Role: authz.RoleAgent}, authz.CapSandboxRelax) {
		t.Fatal("agent must not relax sandbox")
	}
	if err := authz.Require(authz.Principal{Role: authz.RoleFull}, authz.CapDaemonReload); err != nil {
		t.Fatal(err)
	}
	if err := authz.Require(authz.Principal{Role: authz.RoleLogs}, authz.CapProcessStart); err == nil {
		t.Fatal("expected deny")
	}
}

func TestSameUID(t *testing.T) {
	t.Parallel()
	if !authz.SameUID(authz.Principal{UID: "1000"}, "1000") {
		t.Fatal("want same")
	}
	if authz.SameUID(authz.Principal{UID: "1000"}, "0") {
		t.Fatal("want different")
	}
	if authz.SameUID(authz.Principal{UID: ""}, "") {
		t.Fatal("empty uid must never match")
	}
}

func TestCurrentUserDefaultsToAgent(t *testing.T) {
	t.Parallel()
	p, err := authz.CurrentUser("", "sess-1")
	if err != nil {
		t.Fatal(err)
	}
	if p.Role != authz.RoleAgent {
		t.Fatalf("empty role defaulted to %q, want agent", p.Role)
	}
	if p.Session != "sess-1" || p.UID == "" {
		t.Fatalf("principal missing fields: %+v", p)
	}
	// An explicit role is preserved.
	p, err = authz.CurrentUser(authz.RoleReadonly, "sess-2")
	if err != nil {
		t.Fatal(err)
	}
	if p.Role != authz.RoleReadonly {
		t.Fatalf("role = %q, want readonly", p.Role)
	}
}

func TestRequireMessage(t *testing.T) {
	t.Parallel()
	err := authz.Require(authz.Principal{Role: authz.RoleReadonly}, authz.CapProcessStop)
	if err == nil {
		t.Fatal("want deny")
	}
	msg := err.Error()
	if !strings.Contains(msg, "permission_denied") || !strings.Contains(msg, string(authz.CapProcessStop)) {
		t.Fatalf("message %q missing code or capability", msg)
	}
}
