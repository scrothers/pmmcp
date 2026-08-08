// Copyright 2026 Steven Crothers
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cli

import "strings"

// IntentionalCLIOmissions documents MCP tools without a dedicated CLI verb
// . These are streaming tools reachable only over MCP or
// gRPC; a synchronous CLI verb is not meaningful for them.
var IntentionalCLIOmissions = map[string]string{
	"pm_events_subscribe":     "streaming; use MCP or gRPC SubscribeEvents",
	"pm_events_unsubscribe":   "streaming companion to subscribe",
	"pm_events_subscriptions": "streaming companion to subscribe",
	"pm_logs_subscribe":       "streaming; use MCP or gRPC SubscribeLogs",
	"pm_logs_unsubscribe":     "streaming companion to subscribe",
}

// cliVerbs maps each non-omitted catalog tool to its CLI invocation (without the
// leading `pmmcp`). It is the authoritative catalog→CLI coverage map; a tool
// added to ToolMethod without an entry here (and not in IntentionalCLIOmissions)
// fails TestEveryToolIsDispatchable.
var cliVerbs = map[string]string{
	"pm_whoami":           "whoami",
	"pm_daemon_info":      "daemon-info",
	"pm_daemon_reload":    "reload",
	"pm_project_current":  "project current",
	"pm_project_list":     "project list",
	"pm_start":            "start",
	"pm_stop":             "stop",
	"pm_restart":          "restart",
	"pm_update":           "update",
	"pm_remove":           "remove",
	"pm_list":             "list",
	"pm_status":           "status",
	"pm_run":              "run",
	"pm_wait":             "wait",
	"pm_enable":           "enable",
	"pm_disable":          "disable",
	"pm_health_check":     "health",
	"pm_group_create":     "group create",
	"pm_group_remove":     "group remove",
	"pm_group_list":       "group list",
	"pm_group_status":     "group status",
	"pm_group_start":      "group start",
	"pm_group_stop":       "group stop",
	"pm_group_restart":    "group restart",
	"pm_profile_list":     "profile list",
	"pm_profile_get":      "profile get",
	"pm_profile_create":   "profile create",
	"pm_profile_update":   "profile update",
	"pm_profile_delete":   "profile delete",
	"pm_profile_use":      "profile use",
	"pm_session_info":     "session info",
	"pm_session_end":      "session end",
	"pm_share":            "share",
	"pm_unshare":          "unshare",
	"pm_logs":             "logs",
	"pm_grep":             "grep",
	"pm_errors":           "errors",
	"pm_logs_export":      "logs export",
	"pm_logs_ship":        "logs ship",
	"pm_events":           "events",
	"pm_audit_query":      "audit",
	"pm_metrics_snapshot": "metrics",
	"pm_validate":         "validate",
	"pm_diff":             "diff",
	"pm_apply":            "apply",
	"pm_declare_show":     "declare show",
	"pm_ports":            "ports",
	"pm_runtime_info":     "runtime",
	"pm_sandbox_profiles": "sandbox-profiles",
	"pm_secret_list":      "secret list",
	"pm_secret_ref_check": "secret check",
	"pm_secret_set":       "secret set",
	"pm_watch_set":        "watch set",
	"pm_watch_status":     "watch status",
	"pm_webhook_create":   "webhook create",
	"pm_webhook_update":   "webhook update",
	"pm_webhook_delete":   "webhook delete",
	"pm_webhook_list":     "webhook list",
	"pm_webhook_test":     "webhook test",
	"pm_import":           "import",
}

// CommandForTool returns the CLI invocation for a catalog tool, or "" if the
// tool is an intentional omission or has no mapping.
func CommandForTool(tool string) string {
	if _, ok := IntentionalCLIOmissions[tool]; ok {
		return ""
	}
	return cliVerbs[tool]
}

// Dispatchable reports whether verb resolves to a concrete command in the cobra
// tree. It walks the real tree built by NewRootCmd, so it cannot drift from the
// commands the binary actually routes: verb must consume all of its fields to
// reach a non-root command.
func Dispatchable(verb string) bool {
	fields := strings.Fields(verb)
	if len(fields) == 0 {
		return false
	}
	root := NewRootCmd()
	cmd, rest, err := root.Find(fields)
	if err != nil || cmd == root || len(rest) != 0 {
		return false
	}
	return true
}
