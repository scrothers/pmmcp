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
	"testing"

	"github.com/scrothers/pmmcp/internal/authz"
)

// allCaps is every capability in the vocabulary.
var allCaps = []authz.Capability{
	authz.CapProcessStart, authz.CapProcessStop, authz.CapProcessRead,
	authz.CapProcessList, authz.CapProcessRemove, authz.CapProcessRestart,
	authz.CapLogsRead, authz.CapLogsExport, authz.CapEventsRead, authz.CapAuditRead,
	authz.CapDeclareApply, authz.CapDaemonInfo, authz.CapDaemonReload,
	authz.CapSandboxRelax, authz.CapSecretsReadValues, authz.CapSecretSet,
	authz.CapWebhookManage, authz.CapGroupManage, authz.CapProfileManage,
	authz.CapWatchSet, authz.CapSessionShare, authz.CapSessionEnd,
}

// TestCapabilityMatrix asserts every role×capability pair against the frozen
// Capability matrix. The expected set per role is the source of truth; anything not
// listed must be denied (default deny).
func TestCapabilityMatrix(t *testing.T) {
	t.Parallel()

	allowed := map[authz.Role]map[authz.Capability]bool{
		authz.RoleReadonly: setOf(
			authz.CapProcessList, authz.CapProcessRead, authz.CapEventsRead,
			authz.CapDaemonInfo, authz.CapSessionEnd,
		),
		authz.RoleLogs: setOf(
			authz.CapProcessList, authz.CapProcessRead, authz.CapEventsRead,
			authz.CapDaemonInfo, authz.CapSessionEnd, authz.CapLogsRead,
		),
		authz.RoleAgent: setOf(
			authz.CapProcessList, authz.CapProcessRead, authz.CapEventsRead,
			authz.CapDaemonInfo, authz.CapSessionEnd, authz.CapLogsRead,
			authz.CapProcessStart, authz.CapProcessStop, authz.CapProcessRestart,
			authz.CapProcessRemove, authz.CapDeclareApply, authz.CapGroupManage,
			authz.CapProfileManage, authz.CapWatchSet, authz.CapAuditRead,
		),
		authz.RoleOperator: setOf(
			authz.CapProcessList, authz.CapProcessRead, authz.CapEventsRead,
			authz.CapDaemonInfo, authz.CapSessionEnd, authz.CapLogsRead,
			authz.CapProcessStart, authz.CapProcessStop, authz.CapProcessRestart,
			authz.CapProcessRemove, authz.CapDeclareApply, authz.CapGroupManage,
			authz.CapProfileManage, authz.CapWatchSet, authz.CapAuditRead,
			authz.CapLogsExport, authz.CapSandboxRelax, authz.CapDaemonReload,
			authz.CapWebhookManage, authz.CapSecretSet, authz.CapSessionShare,
		),
		authz.RoleFull: setOf(allCaps...),
	}

	for _, role := range []authz.Role{
		authz.RoleReadonly, authz.RoleLogs, authz.RoleAgent, authz.RoleOperator, authz.RoleFull,
	} {
		want := allowed[role]
		for _, c := range allCaps {
			p := authz.Principal{UID: "1", Role: role}
			if got := authz.Allow(p, c); got != want[c] {
				t.Errorf("role=%s cap=%s got=%v want=%v", role, c, got, want[c])
			}
		}
	}
}

// TestOperatorNotFull pins the one matrix distinction: read_values is full-only.
func TestOperatorNotFull(t *testing.T) {
	t.Parallel()
	op := authz.Principal{Role: authz.RoleOperator}
	full := authz.Principal{Role: authz.RoleFull}
	if authz.Allow(op, authz.CapSecretsReadValues) {
		t.Fatal("operator must not read secret values")
	}
	if !authz.Allow(full, authz.CapSecretsReadValues) {
		t.Fatal("full must read secret values")
	}
}

// TestReadonlyExcludesMutations asserts readonly/logs cannot mutate.
func TestReadonlyExcludesMutations(t *testing.T) {
	t.Parallel()
	mutations := []authz.Capability{
		authz.CapProcessStart, authz.CapProcessStop, authz.CapProcessRestart,
		authz.CapProcessRemove, authz.CapDeclareApply, authz.CapSecretSet,
		authz.CapWebhookManage, authz.CapSandboxRelax, authz.CapDaemonReload,
		authz.CapLogsExport, authz.CapSessionShare, authz.CapSecretsReadValues,
	}
	for _, role := range []authz.Role{authz.RoleReadonly, authz.RoleLogs} {
		for _, c := range mutations {
			if authz.Allow(authz.Principal{Role: role}, c) {
				t.Errorf("role=%s must not have %s", role, c)
			}
		}
	}
}

// TestUnknownRoleDenied asserts an unknown/empty role gets nothing.
func TestUnknownRoleDenied(t *testing.T) {
	t.Parallel()
	for _, c := range allCaps {
		if authz.Allow(authz.Principal{Role: "bogus"}, c) {
			t.Errorf("unknown role granted %s", c)
		}
		if authz.Allow(authz.Principal{Role: ""}, c) {
			t.Errorf("empty role granted %s", c)
		}
	}
}

func setOf(caps ...authz.Capability) map[authz.Capability]bool {
	m := make(map[authz.Capability]bool, len(caps))
	for _, c := range caps {
		m[c] = true
	}
	return m
}
