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

package cli

import (
	"sort"

	"github.com/scrothers/pmmcp/internal/api"
	"github.com/scrothers/pmmcp/internal/prompts"
)

// ToolMethod maps MCP tool name -> api.Method constant.
// Every pm_* tool from the MCP tools catalog is registered here.
var ToolMethod = map[string]string{
	// Meta / daemon
	"pm_whoami":          api.MethodWhoami,
	"pm_daemon_info":     api.MethodDaemonInfo,
	"pm_daemon_reload":   api.MethodDaemonReload,
	"pm_project_current": api.MethodProjectCurrent,
	"pm_project_list":    api.MethodProjectList,

	// Process lifecycle
	"pm_start":        api.MethodStart,
	"pm_stop":         api.MethodStop,
	"pm_restart":      api.MethodRestart,
	"pm_update":       api.MethodUpdate,
	"pm_remove":       api.MethodRemove,
	"pm_list":         api.MethodList,
	"pm_status":       api.MethodStatus,
	"pm_run":          api.MethodRun,
	"pm_wait":         api.MethodWait,
	"pm_enable":       api.MethodEnable,
	"pm_disable":      api.MethodDisable,
	"pm_health_check": api.MethodHealthCheck,

	// Groups
	"pm_group_create":  api.MethodGroupCreate,
	"pm_group_remove":  api.MethodGroupRemove,
	"pm_group_list":    api.MethodGroupList,
	"pm_group_status":  api.MethodGroupStatus,
	"pm_group_start":   api.MethodGroupStart,
	"pm_group_stop":    api.MethodGroupStop,
	"pm_group_restart": api.MethodGroupRestart,

	// Profiles
	"pm_profile_list":   api.MethodProfileList,
	"pm_profile_get":    api.MethodProfileGet,
	"pm_profile_create": api.MethodProfileCreate,
	"pm_profile_update": api.MethodProfileUpdate,
	"pm_profile_delete": api.MethodProfileDelete,
	"pm_profile_use":    api.MethodProfileUse,

	// Session / share
	"pm_session_info": api.MethodSessionInfo,
	"pm_session_end":  api.MethodSessionEnd,
	"pm_share":        api.MethodShare,
	"pm_unshare":      api.MethodUnshare,

	// Logs
	"pm_logs":             api.MethodLogs,
	"pm_grep":             api.MethodGrep,
	"pm_errors":           api.MethodErrors,
	"pm_logs_export":      api.MethodLogsExport,
	"pm_logs_ship":        api.MethodLogsShip,
	"pm_logs_subscribe":   api.MethodLogsSubscribe,
	"pm_logs_unsubscribe": api.MethodLogsUnsub,

	// Events / audit / metrics
	"pm_events":               api.MethodEvents,
	"pm_events_subscribe":     api.MethodEventsSub,
	"pm_events_unsubscribe":   api.MethodEventsUnsub,
	"pm_events_subscriptions": api.MethodEventsSubs,
	"pm_audit_query":          api.MethodAudit,
	"pm_metrics_snapshot":     api.MethodMetrics,

	// Declare
	"pm_validate":     api.MethodValidate,
	"pm_diff":         api.MethodDiff,
	"pm_apply":        api.MethodApply,
	"pm_declare_show": api.MethodDeclareShow,

	// Ports / runtime / sandbox
	"pm_ports":            api.MethodPorts,
	"pm_runtime_info":     api.MethodRuntimeInfo,
	"pm_sandbox_profiles": api.MethodSandboxProfiles,

	// Secrets
	"pm_secret_list":      api.MethodSecretList,
	"pm_secret_ref_check": api.MethodSecretRefCheck,
	"pm_secret_set":       api.MethodSecretSet,

	// Watch / webhooks
	"pm_watch_set":      api.MethodWatchSet,
	"pm_watch_status":   api.MethodWatchStatus,
	"pm_webhook_create": api.MethodWebhookCreate,
	"pm_webhook_update": api.MethodWebhookUpdate,
	"pm_webhook_delete": api.MethodWebhookDelete,
	"pm_webhook_list":   api.MethodWebhookList,
	"pm_webhook_test":   api.MethodWebhookTest,

	// Import
	"pm_import": api.MethodImport,
}

// ToolDescription maps tool names to short purpose strings for tools/list.
// Values are owned by package prompts (lines.toml); this is a copy at init.
var ToolDescription = prompts.ToolDescriptions()

// ToolNames returns sorted tool names for tools/list.
func ToolNames() []string {
	names := make([]string, 0, len(ToolMethod))
	for n := range ToolMethod {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// MethodSet returns the set of IPC methods referenced by ToolMethod.
// CatalogParityTest helpers used from test.
func MethodSet() map[string]struct{} {
	m := make(map[string]struct{}, len(ToolMethod))
	for _, method := range ToolMethod {
		m[method] = struct{}{}
	}
	return m
}

// AllMethodsSet returns api.AllMethods as a set for parity checks.
func AllMethodsSet() map[string]struct{} {
	m := make(map[string]struct{}, len(api.AllMethods))
	for _, method := range api.AllMethods {
		m[method] = struct{}{}
	}
	return m
}

// ReverseToolMethod maps api.Method -> first tool name (for debugging/tests).
func ReverseToolMethod() map[string]string {
	rev := make(map[string]string, len(ToolMethod))
	for tool, method := range ToolMethod {
		if _, ok := rev[method]; !ok {
			rev[method] = tool
		}
	}
	return rev
}
