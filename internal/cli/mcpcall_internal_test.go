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
	"context"
	"testing"

	"github.com/scrothers/pmmcp/internal/api"
)

// TestMCPCallTypedErrorBranches drives every mcpCall typed-payload switch arm
// through a daemon that errors, covering the `if err := c.Call(...); err !=
// nil { return "", err }` branch for each one (the success branches are
// already covered by TestMCPCallToolPaths in mcp_internal_test.go).
func TestMCPCallTypedErrorBranches(t *testing.T) {
	cases := []struct {
		tool   string
		method string
		args   map[string]any
	}{
		{"pm_start", api.MethodStart, map[string]any{"name": "api", "command": []any{"./bin/api"}}},
		{"pm_stop", api.MethodStop, map[string]any{"name": "api"}},
		{"pm_list", api.MethodList, map[string]any{"project": "p"}},
		{"pm_logs", api.MethodLogs, map[string]any{"name": "api"}},
		{"pm_daemon_info", api.MethodDaemonInfo, map[string]any{}},
		{"pm_whoami", api.MethodWhoami, map[string]any{}},
		{"pm_events", api.MethodEvents, map[string]any{}},
	}
	for _, tc := range cases {
		t.Run(tc.tool, func(t *testing.T) {
			endpoint := startScriptedDaemon(t, map[string]scriptedResponse{
				tc.method: {errMsg: "boom", errCode: "internal"},
			})
			if _, err := mcpCall(context.Background(), endpoint, tc.tool, tc.args); err == nil {
				t.Errorf("mcpCall(%s) = nil error, want error", tc.tool)
			}
		})
	}
}

// TestMCPCallGenericBranches covers the generic (non-typed) path's call-error
// and out==nil branches, using a tool not in mcpCall's specialized switch.
func TestMCPCallGenericBranches(t *testing.T) {
	t.Run("call error", func(t *testing.T) {
		endpoint := startScriptedDaemon(t, map[string]scriptedResponse{
			api.MethodValidate: {errMsg: "boom", errCode: "internal"},
		})
		if _, err := mcpCall(context.Background(), endpoint, "pm_validate", nil); err == nil {
			t.Fatal("want call error")
		}
	})
	t.Run("nil result", func(t *testing.T) {
		endpoint := startScriptedDaemon(t, map[string]scriptedResponse{
			api.MethodValidate: {payload: nil},
		})
		out, err := mcpCall(context.Background(), endpoint, "pm_validate", nil)
		if err != nil {
			t.Fatalf("mcpCall: %v", err)
		}
		if out != "{}" {
			t.Errorf("mcpCall nil-payload result = %q, want {}", out)
		}
	})
}
