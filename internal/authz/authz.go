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
	"fmt"
	"os/user"
)

// Capability is a fine-grained permission bit.
type Capability string

// Capabilities are the fine-grained permission bits referenced by the role
// packs and by the daemon's per-tool authz checks.
const (
	CapProcessStart   Capability = "process:start"
	CapProcessStop    Capability = "process:stop"
	CapProcessRead    Capability = "process:read"
	CapProcessList    Capability = "process:list"
	CapProcessRemove  Capability = "process:remove"
	CapProcessRestart Capability = "process:restart"
	CapLogsRead       Capability = "logs:read"
	CapLogsExport     Capability = "logs:export"
	CapEventsRead     Capability = "events:read"
	CapAuditRead      Capability = "audit:read"
	CapDeclareApply   Capability = "declare:apply"
	CapDaemonInfo     Capability = "daemon:info"
	CapDaemonReload   Capability = "daemon:configure"
	CapSandboxRelax   Capability = "sandbox:relax"

	// CapSecretsReadValues gates reading raw secret values (full role only);
	// CapSecretSet gates keyring writes via pm_secret_set ("authz heavy").
	CapSecretsReadValues Capability = "secrets:read_values"
	CapSecretSet         Capability = "secrets:write"

	// CapWebhookManage, CapGroupManage, CapProfileManage, and CapWatchSet gate
	// management surfaces kept away from the readonly and logs roles.
	CapWebhookManage Capability = "webhooks:manage"
	CapGroupManage   Capability = "group:manage"
	CapProfileManage Capability = "profile:manage"
	CapWatchSet      Capability = "watch:set"

	// CapSessionShare grants cross-session access (operator+); CapSessionEnd is
	// self-management, so any role may end its own session.
	CapSessionShare Capability = "session:share"
	CapSessionEnd   Capability = "session:end"
)

// Role is a named capability pack.
type Role string

// Roles are the named capability packs defined by the authz matrix.
const (
	RoleFull     Role = "full"
	RoleOperator Role = "operator"
	RoleAgent    Role = "agent"
	RoleReadonly Role = "readonly"
	RoleLogs     Role = "logs"
)

// Principal is an authenticated client.
type Principal struct {
	UID      string
	Username string
	Role     Role
	Session  string
}

// capsReadonly is the read-only floor: list/read and self session-end only.
var capsReadonly = []Capability{
	CapProcessList, CapProcessRead, CapEventsRead, CapDaemonInfo, CapSessionEnd,
}

// capsLogs adds log reading to the read-only floor.
var capsLogs = append(cloneCaps(capsReadonly), CapLogsRead)

// capsAgent adds process lifecycle, declare, and workspace management.
var capsAgent = append(cloneCaps(capsLogs),
	CapProcessStart, CapProcessStop, CapProcessRestart, CapProcessRemove,
	CapDeclareApply, CapGroupManage, CapProfileManage, CapWatchSet, CapAuditRead,
)

// capsOperator adds privileged operational surfaces (but not read_values).
var capsOperator = append(cloneCaps(capsAgent),
	CapLogsExport, CapSandboxRelax, CapDaemonReload,
	CapWebhookManage, CapSecretSet, CapSessionShare,
)

// capsFull is operator plus the ability to read raw secret values.
var capsFull = append(cloneCaps(capsOperator), CapSecretsReadValues)

func cloneCaps(in []Capability) []Capability {
	out := make([]Capability, len(in))
	copy(out, in)
	return out
}

// Caps returns the capability set for a role. Unknown roles get an empty
// set (default deny for unknown actions, per the authz matrix security notes).
func Caps(role Role) map[Capability]bool {
	var list []Capability
	switch role {
	case RoleFull:
		list = capsFull
	case RoleOperator:
		list = capsOperator
	case RoleAgent:
		list = capsAgent
	case RoleLogs:
		list = capsLogs
	case RoleReadonly:
		list = capsReadonly
	default:
		return map[Capability]bool{}
	}
	m := make(map[Capability]bool, len(list))
	for _, c := range list {
		m[c] = true
	}
	return m
}

// Allow reports whether p may exercise c.
func Allow(p Principal, c Capability) bool {
	return Caps(p.Role)[c]
}

// Require returns an error if not allowed.
func Require(p Principal, c Capability) error {
	if Allow(p, c) {
		return nil
	}
	return fmt.Errorf("authz: permission_denied: %s lacks %s", p.Role, c)
}

// CurrentUser builds a Principal for the OS user running the client (same-user model).
// An unset role defaults to RoleAgent — the harness default per the authz matrix —
// never RoleFull, so a client that never states a role is not silently privileged.
func CurrentUser(role Role, session string) (Principal, error) {
	return currentUserFrom(user.Current, role, session)
}

// currentUserFrom builds a Principal from a caller-supplied user lookup, so
// tests can exercise the lookup-failure branch without a mutable global.
func currentUserFrom(lookup func() (*user.User, error), role Role, session string) (Principal, error) {
	u, err := lookup()
	if err != nil {
		return Principal{}, fmt.Errorf("authz: current user: %w", err)
	}
	if role == "" {
		role = RoleAgent
	}
	return Principal{UID: u.Uid, Username: u.Username, Role: role, Session: session}, nil
}

// SameUID reports whether principal matches the daemon's OS user (local trust model).
func SameUID(p Principal, daemonUID string) bool {
	return p.UID != "" && p.UID == daemonUID
}
