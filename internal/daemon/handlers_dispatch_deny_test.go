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

package daemon_test

import (
	"context"
	"strings"
	"testing"

	"github.com/scrothers/pmmcp/internal/api"
)

// TestDispatchExtraDenyPaths covers the dispatchExtra case arms whose
// capability-check-then-dispatch pair had never been exercised on the deny
// side: process.ports, webhook.create/update/delete, metrics.snapshot,
// logs.export, logs.ship, logs.subscribe, events.subscribe. Each requires a
// capability a readonly-role client lacks, so the request must be denied
// before ever reaching the underlying handler.
func TestDispatchExtraDenyPaths(t *testing.T) {
	cases := []struct {
		name    string
		method  string
		payload any
		// role denied: "readonly" already holds process:read/daemon:info/
		// events:read (the read-only floor), so process.ports, metrics, and
		// events.subscribe need a role with an *empty* capability set — an
		// unrecognized role string, per authz.Caps's default-deny fallback —
		// to actually exercise their deny branch.
		role string
	}{
		{"ports", api.MethodPorts, api.IDPayload{ID: "proc-does-not-exist"}, "unrecognized-role"},
		{"webhook_create", api.MethodWebhookCreate, api.WebhookPayload{URL: "https://example.com/hook"}, "readonly"},
		{"webhook_update", api.MethodWebhookUpdate, api.WebhookPayload{ID: "hook-does-not-exist"}, "readonly"},
		{"webhook_delete", api.MethodWebhookDelete, api.WebhookPayload{ID: "hook-does-not-exist"}, "readonly"},
		{"webhook_test", api.MethodWebhookTest, api.WebhookPayload{ID: "hook-does-not-exist"}, "readonly"},
		{"metrics", api.MethodMetrics, nil, "unrecognized-role"},
		{"logs_export", api.MethodLogsExport, api.LogsPayload{ID: "proc-does-not-exist"}, "readonly"},
		{"logs_ship", api.MethodLogsShip, api.LogsShipPayload{SinkPath: "/tmp/wherever"}, "readonly"},
		{"logs_subscribe", api.MethodLogsSubscribe, api.SubPayload{ID: "proc-does-not-exist"}, "readonly"},
		{"events_subscribe", api.MethodEventsSub, api.SubPayload{}, "unrecognized-role"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c, _ := newTestDaemon(t, nil)
			c.SetSession("sess-deny-"+tc.name, tc.role)
			var out map[string]any
			err := c.Call(context.Background(), tc.method, tc.payload, &out)
			if err == nil || !strings.Contains(err.Error(), "permission") {
				t.Fatalf("%s as %s: want permission_denied, got %v", tc.method, tc.role, err)
			}
		})
	}
}
