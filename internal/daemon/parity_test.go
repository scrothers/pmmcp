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
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/scrothers/pmmcp/internal/api"
	"github.com/scrothers/pmmcp/internal/config"
	"github.com/scrothers/pmmcp/internal/daemon"
	"github.com/scrothers/pmmcp/internal/ipc"
	"github.com/scrothers/pmmcp/internal/testsock"
)

// TestAllMethodsDispatched ensures every api method is handled (not "unknown method").
func TestAllMethodsDispatched(t *testing.T) {
	t.Parallel()
	ctx, cancel, c, dir, startID := bootParityDaemon(t)
	defer cancel()
	defer c.Close()

	for _, m := range api.AllMethods {
		if m == api.MethodHello {
			continue
		}
		payload := parityPayload(m, startID, dir)
		err := c.Call(ctx, m, payload, &map[string]any{})
		if err != nil && (strings.Contains(err.Error(), "unknown method") || strings.Contains(err.Error(), "unimplemented")) {
			t.Errorf("method %s unknown: %v", m, err)
		}
	}
	_ = c.Call(ctx, api.MethodStop, map[string]any{"id": startID, "timeout_sec": 2}, &map[string]any{})
}

// TestCoreMethodsSucceed requires real success on the product path for key tools.
func TestCoreMethodsSucceed(t *testing.T) {
	t.Parallel()
	ctx, cancel, c, dir, startID := bootParityDaemon(t)
	defer cancel()
	defer c.Close()

	mustOK := []struct {
		method  string
		payload any
	}{
		{api.MethodList, api.ListPayload{}},
		{api.MethodStatus, api.IDPayload{ID: startID}},
		{api.MethodLogs, api.LogsPayload{ID: startID, Lines: 10}},
		{api.MethodGrep, api.LogsPayload{ID: startID, Pattern: ".*", Lines: 10}},
		{api.MethodErrors, api.LogsPayload{ID: startID, Lines: 10}},
		{api.MethodEvents, api.EventsPayload{Limit: 10}},
		{api.MethodAudit, map[string]any{"limit": 10}},
		{api.MethodDaemonInfo, nil},
		{api.MethodWhoami, nil},
		{api.MethodProjectCurrent, map[string]any{"cwd": dir}},
		{api.MethodProjectList, nil},
		{api.MethodSandboxProfiles, nil},
		{api.MethodRuntimeInfo, nil},
		{api.MethodMetrics, nil},
		{api.MethodPorts, api.IDPayload{ID: startID}},
		{api.MethodHealthCheck, api.IDPayload{ID: startID}},
		{api.MethodEnable, api.IDPayload{ID: startID}},
		{api.MethodGroupCreate, map[string]any{"name": "pg", "members": []map[string]any{{"name": "parity"}}}},
		{api.MethodGroupList, nil},
		{api.MethodGroupStatus, map[string]any{"name": "pg"}},
		{api.MethodProfileCreate, map[string]any{"name": "default", "project_id": "proj-x"}},
		{api.MethodProfileList, map[string]any{"project_id": "proj-x"}},
		{api.MethodValidate, map[string]any{"yaml": "apiVersion: pmmcp.dev/v1alpha1\nkind: Project\nservices: {}\n"}},
		{api.MethodWebhookCreate, map[string]any{"id": "wh-core", "url": "https://example.com/h"}},
		{api.MethodWebhookList, nil},
		{api.MethodLogsExport, api.IDPayload{ID: startID}},
		{api.MethodSecretSet, map[string]any{"name": "k", "value": "v"}},
		{api.MethodSecretList, nil},
		{api.MethodWatchSet, map[string]any{"id": startID, "path": dir}},
		{api.MethodWatchStatus, nil},
	}
	for _, tc := range mustOK {
		var out any
		if err := c.Call(ctx, tc.method, tc.payload, &out); err != nil {
			t.Errorf("%s must succeed: %v", tc.method, err)
		}
	}
	_ = c.Call(ctx, api.MethodStop, map[string]any{"id": startID, "timeout_sec": 2}, &map[string]any{})
}

func bootParityDaemon(t *testing.T) (context.Context, context.CancelFunc, *ipc.Client, string, string) {
	t.Helper()
	dir := t.TempDir()
	sock := testsock.Path(t)
	cfg, err := config.Load(config.LoadOptions{
		GOOS: "linux", Home: dir,
		LookupEnv: func(string) (string, bool) { return "", false },
	})
	if err != nil {
		t.Fatal(err)
	}
	cfg.StateDir = filepath.Join(dir, "state")
	cfg.IPC.Endpoint = sock
	cfg.Sandbox.Default = "off"
	cfg.Relaunch.Enabled = false
	cfg.Webhook.Allowlist = []string{"*.example.com"}

	ctx, cancel := context.WithCancel(context.Background())
	srv, err := daemon.New(ctx, daemon.Options{Config: cfg, DBPath: filepath.Join(dir, "t.db")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Close() })
	go func() { _ = srv.ListenAndServe(ctx) }()

	var c *ipc.Client
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		c, err = ipc.Dial(ctx, sock)
		if err == nil {
			break
		}
		time.Sleep(15 * time.Millisecond)
	}
	if c == nil {
		cancel()
		t.Fatal(err)
	}

	var start api.StartResult
	if err := c.Call(ctx, api.MethodStart, api.StartPayload{
		Name: "parity", Command: []string{"sleep", "60"}, Sandbox: "off",
		HealthURL: "", AutoRestart: true, Ports: []string{"8080"},
	}, &start); err != nil {
		cancel()
		t.Fatalf("seed start: %v", err)
	}
	return ctx, cancel, c, dir, start.ID
}

func parityPayload(m, startID, dir string) any {
	switch m {
	case api.MethodStart, api.MethodRun:
		return api.StartPayload{Name: "x-" + strings.ReplaceAll(m, ".", "-"), Command: []string{"true"}, Sandbox: "off"}
	case api.MethodStop, api.MethodRestart, api.MethodRemove, api.MethodStatus, api.MethodWait,
		api.MethodEnable, api.MethodDisable, api.MethodHealthCheck, api.MethodUpdate,
		api.MethodLogs, api.MethodGrep, api.MethodErrors, api.MethodLogsExport,
		api.MethodPorts, api.MethodLogsSubscribe, api.MethodLogsUnsub, api.MethodLogsShip:
		return map[string]any{"id": startID, "name": "parity", "pattern": "e", "path": filepath.Join(dir, "ship.tgz")}
	case api.MethodGroupCreate:
		return map[string]any{"name": "g1", "members": []map[string]any{{"name": "parity"}}}
	case api.MethodGroupStart, api.MethodGroupStop, api.MethodGroupRestart, api.MethodGroupStatus, api.MethodGroupRemove:
		return map[string]any{"name": "g1"}
	case api.MethodProfileCreate:
		return map[string]any{"name": "default", "project_id": "proj-test"}
	case api.MethodProfileGet, api.MethodProfileUpdate, api.MethodProfileDelete, api.MethodProfileUse:
		return map[string]any{"name": "default", "project_id": "proj-test"}
	case api.MethodValidate, api.MethodDiff, api.MethodApply, api.MethodDeclareShow:
		return map[string]any{"yaml": "apiVersion: pmmcp.dev/v1alpha1\nkind: Project\nservices: {}\n"}
	case api.MethodImport:
		return map[string]any{"format": "procfile", "data": "web: echo hi\n"}
	case api.MethodWebhookCreate, api.MethodWebhookUpdate:
		return map[string]any{"id": "wh1", "url": "https://example.com/hook"}
	case api.MethodWebhookDelete, api.MethodWebhookTest:
		return map[string]any{"id": "wh1"}
	case api.MethodShare, api.MethodUnshare:
		return map[string]any{"target": startID, "session": "sess-other", "cap": "process:read"}
	case api.MethodWatchSet:
		return map[string]any{"id": startID, "path": dir}
	case api.MethodSecretSet, api.MethodSecretRefCheck:
		return map[string]any{"name": "db", "value": "x", "path": filepath.Join(dir, "ref")}
	default:
		return map[string]any{}
	}
}
