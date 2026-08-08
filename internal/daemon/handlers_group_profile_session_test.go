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
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scrothers/pmmcp/internal/api"
	"github.com/scrothers/pmmcp/internal/config"
)

// --- unimplemented method ---
//
// Note: this exercises handle()'s isKnownMethod gate (server.go), not
// dispatchExtra's own `default:` arm — a method absent from api.AllMethods
// never reaches dispatchExtra at all. dispatchExtra's default is dead code:
// every method in api.AllMethods is handled either by handle()'s core switch
// or by one of dispatchExtra's own cases, so nothing reaches its default.

func TestDispatchExtraUnknownMethod(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, nil)
	c.SetSession("sess-full", "full")
	ctx := context.Background()
	var out map[string]any
	err := c.Call(ctx, "totally.unknown.method", nil, &out)
	if err == nil || !strings.Contains(err.Error(), "unimplemented") {
		t.Fatalf("unknown method: want unimplemented error, got %v", err)
	}
}

// --- deny paths reachable only via an unrecognized role (every named role
// carries process:read/daemon:info/session:end, so a genuine deny for those
// capabilities requires a role string that matches none of the known packs,
// which authz.Caps maps to an empty capability set). ---

func denyBogus(t *testing.T, method string, payload any) {
	t.Helper()
	c, _ := newTestDaemon(t, nil)
	c.SetSession("sess-bogus", "not-a-real-role")
	ctx := context.Background()
	var out map[string]any
	err := c.Call(ctx, method, payload, &out)
	if err == nil || !strings.Contains(err.Error(), "permission") {
		t.Fatalf("%s with unrecognized role: want permission_denied, got %v", method, err)
	}
}

func TestDoRunDeniedForUnrecognizedRole(t *testing.T) {
	t.Parallel()
	denyBogus(t, api.MethodRun, api.RunPayload{StartPayload: api.StartPayload{Name: "x", Command: []string{"sleep", "1"}}})
}

func TestDoWaitDeniedForUnrecognizedRole(t *testing.T) {
	t.Parallel()
	denyBogus(t, api.MethodWait, api.IDPayload{ID: "x"})
}

func TestDoHealthCheckDeniedForUnrecognizedRole(t *testing.T) {
	t.Parallel()
	denyBogus(t, api.MethodHealthCheck, api.IDPayload{ID: "x"})
}

func TestDoProjectListDeniedForUnrecognizedRole(t *testing.T) {
	t.Parallel()
	denyBogus(t, api.MethodProjectList, nil)
}

func TestDoProfileListDeniedForUnrecognizedRole(t *testing.T) {
	t.Parallel()
	denyBogus(t, api.MethodProfileList, api.ProfilePayload{})
}

func TestDoProfileGetDeniedForUnrecognizedRole(t *testing.T) {
	t.Parallel()
	denyBogus(t, api.MethodProfileGet, api.ProfilePayload{ID: "x"})
}

func TestSessionEndDeniedForUnrecognizedRole(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, nil)
	c.SetSession("sess-bogus-end", "not-a-real-role")
	ctx := context.Background()
	var out map[string]any
	err := c.Call(ctx, api.MethodSessionEnd, map[string]any{"id": "sess-bogus-end"}, &out)
	if err == nil || !strings.Contains(err.Error(), "permission") {
		t.Fatalf("session.end with unrecognized role: want permission_denied, got %v", err)
	}
}

func TestUnshareDeniedForAgent(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, nil)
	c.SetSession("sess-agent-unshare", "agent")
	ctx := context.Background()
	var out map[string]any
	err := c.Call(ctx, api.MethodUnshare, api.SharePayload{Target: "p", ToSession: "s", Cap: "process:stop"}, &out)
	if err == nil || !strings.Contains(err.Error(), "permission") {
		t.Fatalf("agent unshare: want permission_denied, got %v", err)
	}
}

// --- daemon.reload ---

func TestDaemonReloadLogLevels(t *testing.T) {
	cases := []string{"debug", "warn", "warning", "error", "info", "unknown-level"}
	for _, lvl := range cases {
		t.Run(lvl, func(t *testing.T) {
			c, _ := newTestDaemon(t, nil)
			c.SetSession("sess-full", "full")
			ctx := context.Background()
			dir := t.TempDir()
			cfgPath := filepath.Join(dir, "reload.toml")
			body := "[log]\nlevel = \"" + lvl + "\"\n[sandbox]\ndefault = \"strict\"\n"
			if err := os.WriteFile(cfgPath, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PMMCP_CONFIG", cfgPath)
			var out map[string]any
			if err := c.Call(ctx, api.MethodDaemonReload, nil, &out); err != nil {
				t.Fatalf("reload: %v", err)
			}
			if out["status"] != "ok" {
				t.Fatalf("status = %v, want ok", out["status"])
			}
			if !auditHas(t, c, func(r auditRow) bool { return r.Action == "daemon.reload" }) {
				t.Fatal("no daemon.reload audit row")
			}
		})
	}
}

func TestDaemonReloadBadConfigPath(t *testing.T) {
	c, _ := newTestDaemon(t, nil)
	c.SetSession("sess-full", "full")
	ctx := context.Background()
	t.Setenv("PMMCP_CONFIG", filepath.Join(t.TempDir(), "does-not-exist.toml"))
	var out map[string]any
	err := c.Call(ctx, api.MethodDaemonReload, nil, &out)
	if err == nil {
		t.Fatal("reload with bad config path: want error, got nil")
	}
	if !auditHas(t, c, func(r auditRow) bool {
		return r.Action == "daemon.reload" && strings.Contains(r.Capability, "")
	}) {
		t.Fatal("no daemon.reload audit row for failed reload")
	}
}

func TestDaemonReloadDeniedForReadonly(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, nil)
	c.SetSession("sess-ro", "readonly")
	ctx := context.Background()
	var out map[string]any
	err := c.Call(ctx, api.MethodDaemonReload, nil, &out)
	if err == nil || !strings.Contains(err.Error(), "permission") {
		t.Fatalf("readonly daemon.reload: want permission_denied, got %v", err)
	}
}

// --- project.list ---

func TestProjectListEmpty(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, nil)
	c.SetSession("sess-ro", "readonly")
	ctx := context.Background()
	var out api.ProjectListResult
	if err := c.Call(ctx, api.MethodProjectList, nil, &out); err != nil {
		t.Fatalf("project.list: %v", err)
	}
}

// --- process.update ---

func TestDoUpdateBadPayload(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, func(cfg *config.Config) { cfg.Sandbox.Default = "off" })
	c.SetSession("sess-full", "full")
	ctx := context.Background()
	var out map[string]any
	// A JSON array where an object is expected fails json.Unmarshal.
	err := c.Call(ctx, api.MethodUpdate, []string{"not", "an", "object"}, &out)
	if err == nil || !strings.Contains(err.Error(), "bad update payload") {
		t.Fatalf("update with array payload: want bad update payload error, got %v", err)
	}
}

func TestDoUpdateUnknownID(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, func(cfg *config.Config) { cfg.Sandbox.Default = "off" })
	c.SetSession("sess-full", "full")
	ctx := context.Background()
	var out map[string]any
	err := c.Call(ctx, api.MethodUpdate, api.UpdatePayload{ID: "proc-does-not-exist"}, &out)
	if err == nil {
		t.Fatal("update unknown id: want error, got nil")
	}
}

func TestDoUpdateInvalidCommand(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, func(cfg *config.Config) { cfg.Sandbox.Default = "off" })
	c.SetSession("sess-full", "full")
	ctx := context.Background()
	var start api.StartResult
	if err := c.Call(ctx, api.MethodStart, api.StartPayload{
		Name: "upd1", Command: []string{"sleep", "1"}, Sandbox: "off",
	}, &start); err != nil {
		t.Fatalf("start: %v", err)
	}
	var out map[string]any
	err := c.Call(ctx, api.MethodUpdate, api.UpdatePayload{ID: start.ID, Command: []string{""}}, &out)
	if err == nil {
		t.Fatal("update with invalid (empty-arg) command: want error, got nil")
	}
}

func TestDoUpdateFullFieldsAndRestart(t *testing.T) {
	t.Parallel()
	c, mgr := newTestDaemon(t, func(cfg *config.Config) { cfg.Sandbox.Default = "off" })
	c.SetSession("sess-full", "full")
	ctx := context.Background()
	var start api.StartResult
	if err := c.Call(ctx, api.MethodStart, api.StartPayload{
		Name: "upd2", Command: []string{"sleep", "1"}, Sandbox: "off",
	}, &start); err != nil {
		t.Fatalf("start: %v", err)
	}
	var out api.ProcessView
	err := c.Call(ctx, api.MethodUpdate, api.UpdatePayload{
		ID: start.ID, Command: []string{"sleep", "2"}, Cwd: "/tmp",
		Env: map[string]string{"FOO": "bar"}, Restart: true,
	}, &out)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if !auditHas(t, c, func(r auditRow) bool { return r.Action == "process.update" && r.Target == start.ID }) {
		t.Fatal("no process.update audit row")
	}
	_ = mgr
}

func TestDoUpdateDeniedForReadonly(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, func(cfg *config.Config) { cfg.Sandbox.Default = "off" })
	c.SetSession("sess-ro", "readonly")
	ctx := context.Background()
	var out map[string]any
	err := c.Call(ctx, api.MethodUpdate, api.UpdatePayload{ID: "whatever"}, &out)
	if err == nil || !strings.Contains(err.Error(), "permission") {
		t.Fatalf("readonly update: want permission_denied, got %v", err)
	}
}

// --- process.run ---

func TestDoRunBadPayload(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, func(cfg *config.Config) { cfg.Sandbox.Default = "off" })
	c.SetSession("sess-full", "full")
	ctx := context.Background()
	var out map[string]any
	err := c.Call(ctx, api.MethodRun, []string{"bad"}, &out)
	if err == nil || !strings.Contains(err.Error(), "bad run payload") {
		t.Fatalf("run with array payload: want bad run payload error, got %v", err)
	}
}

func TestDoRunStartFails(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, func(cfg *config.Config) { cfg.Sandbox.Default = "off" })
	c.SetSession("sess-full", "full")
	ctx := context.Background()
	var out map[string]any
	err := c.Call(ctx, api.MethodRun, api.RunPayload{
		StartPayload: api.StartPayload{Name: "badrun", Command: nil, Sandbox: "off"},
	}, &out)
	if err == nil {
		t.Fatal("run with empty command: want error, got nil")
	}
}

func TestDoRunWithWaitAndTimeout(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, func(cfg *config.Config) { cfg.Sandbox.Default = "off" })
	c.SetSession("sess-full", "full")
	ctx := context.Background()
	var out api.WaitResult
	err := c.Call(ctx, api.MethodRun, api.RunPayload{
		StartPayload: api.StartPayload{Name: "runwait", Command: []string{"/bin/echo", "hi"}, Sandbox: "off"},
		Wait:         true, TimeoutSec: 2,
	}, &out)
	if err != nil {
		t.Fatalf("run with wait: %v", err)
	}
}

// --- process.wait ---

func TestDoWaitUnknownID(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, nil)
	c.SetSession("sess-full", "full")
	ctx := context.Background()
	var out api.WaitResult
	err := c.Call(ctx, api.MethodWait, api.IDPayload{ID: "does-not-exist"}, &out)
	if err == nil {
		t.Fatal("wait on unknown id: want error, got nil")
	}
}

func TestDoWaitTimeout(t *testing.T) {
	t.Parallel()
	// Needs the real process manager: fakeMgr.Wait returns immediately
	// regardless of ctx, so it can never exercise the DeadlineExceeded path.
	ctx, _, c, _ := startTestDaemon(t)
	c.SetSession("sess-full", "full")
	var start api.StartResult
	if err := c.Call(ctx, api.MethodStart, api.StartPayload{
		Name: "waitp", Command: []string{"sleep", "60"}, Sandbox: "off",
	}, &start); err != nil {
		t.Fatalf("start: %v", err)
	}
	var out api.WaitResult
	if err := c.Call(ctx, api.MethodWait, api.IDPayload{ID: start.ID, TimeoutSec: 1}, &out); err != nil {
		t.Fatalf("wait: %v", err)
	}
	if !out.TimedOut {
		t.Fatalf("wait result = %+v, want TimedOut=true", out)
	}
	_ = c.Call(ctx, api.MethodStop, api.IDPayload{ID: start.ID, Force: true}, &map[string]any{})
}

// --- process.enable / process.disable ---

func TestDoEnableUnknownID(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, nil)
	c.SetSession("sess-full", "full")
	ctx := context.Background()
	var out map[string]any
	if err := c.Call(ctx, api.MethodEnable, api.IDPayload{ID: "nope"}, &out); err == nil {
		t.Fatal("enable unknown id: want error, got nil")
	}
}

func TestDoDisableUnknownID(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, nil)
	c.SetSession("sess-full", "full")
	ctx := context.Background()
	var out map[string]any
	if err := c.Call(ctx, api.MethodDisable, api.IDPayload{ID: "nope"}, &out); err == nil {
		t.Fatal("disable unknown id: want error, got nil")
	}
}

func TestDoDisableWhileRunningStopsProcess(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, func(cfg *config.Config) { cfg.Sandbox.Default = "off" })
	c.SetSession("sess-full", "full")
	ctx := context.Background()
	var start api.StartResult
	if err := c.Call(ctx, api.MethodStart, api.StartPayload{
		Name: "dis1", Command: []string{"sleep", "60"}, Sandbox: "off",
	}, &start); err != nil {
		t.Fatalf("start: %v", err)
	}
	var out api.ProcessView
	if err := c.Call(ctx, api.MethodDisable, api.IDPayload{ID: start.ID}, &out); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if !auditHas(t, c, func(r auditRow) bool { return r.Action == "process.disable" && r.Target == start.ID }) {
		t.Fatal("no process.disable audit row")
	}
}

func TestDoEnableDeniedForReadonly(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, nil)
	c.SetSession("sess-ro", "readonly")
	ctx := context.Background()
	var out map[string]any
	if err := c.Call(ctx, api.MethodEnable, api.IDPayload{ID: "x"}, &out); err == nil || !strings.Contains(err.Error(), "permission") {
		t.Fatalf("readonly enable: want permission_denied, got %v", err)
	}
}

func TestDoDisableDeniedForReadonly(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, nil)
	c.SetSession("sess-ro", "readonly")
	ctx := context.Background()
	var out map[string]any
	if err := c.Call(ctx, api.MethodDisable, api.IDPayload{ID: "x"}, &out); err == nil || !strings.Contains(err.Error(), "permission") {
		t.Fatalf("readonly disable: want permission_denied, got %v", err)
	}
}

// --- process.health_check ---

func TestDoHealthCheckUnknownID(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, nil)
	c.SetSession("sess-full", "full")
	ctx := context.Background()
	var out api.HealthCheckResult
	if err := c.Call(ctx, api.MethodHealthCheck, api.IDPayload{ID: "nope"}, &out); err == nil {
		t.Fatal("health_check unknown id: want error, got nil")
	}
}

func TestDoHealthCheckNotRunning(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, func(cfg *config.Config) { cfg.Sandbox.Default = "off" })
	c.SetSession("sess-full", "full")
	ctx := context.Background()
	var start api.StartResult
	if err := c.Call(ctx, api.MethodStart, api.StartPayload{
		Name: "hc1", Command: []string{"sleep", "1"}, Sandbox: "off",
	}, &start); err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := c.Call(ctx, api.MethodStop, api.IDPayload{ID: start.ID, Force: true}, &map[string]any{}); err != nil {
		t.Fatalf("stop: %v", err)
	}
	var out api.HealthCheckResult
	if err := c.Call(ctx, api.MethodHealthCheck, api.IDPayload{ID: start.ID}, &out); err != nil {
		t.Fatalf("health_check: %v", err)
	}
	if out.OK {
		t.Fatalf("health_check on stopped process: OK=true, want false: %+v", out)
	}
}

func TestDoHealthCheckDeniedForReadonly(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, nil)
	c.SetSession("sess-ro", "readonly")
	ctx := context.Background()
	// readonly HAS process:read, so health_check should actually be allowed;
	// verify it doesn't spuriously deny for an unknown id (it should surface
	// resolveID's not-found, not a permission error).
	var out api.HealthCheckResult
	err := c.Call(ctx, api.MethodHealthCheck, api.IDPayload{ID: "nope"}, &out)
	if err == nil {
		t.Fatal("health_check unknown id (readonly): want error, got nil")
	}
	if strings.Contains(err.Error(), "permission") {
		t.Fatalf("readonly has process:read, should not be denied: %v", err)
	}
}

// --- group.create / group.remove / group.status / group lifecycle ---

func TestGroupCreateBadPayload(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, nil)
	c.SetSession("sess-full", "full")
	ctx := context.Background()
	var out map[string]any
	err := c.Call(ctx, api.MethodGroupCreate, []string{"bad"}, &out)
	if err == nil || !strings.Contains(err.Error(), "bad group payload") {
		t.Fatalf("group create with array payload: want bad group payload error, got %v", err)
	}
}

func TestGroupCreateDuplicateID(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, nil)
	c.SetSession("sess-full", "full")
	ctx := context.Background()
	var g api.GroupView
	if err := c.Call(ctx, api.MethodGroupCreate, api.GroupPayload{
		Name: "one", Members: []api.GroupMemberPayload{{Name: "a"}},
	}, &g); err != nil {
		t.Fatalf("group create: %v", err)
	}
	err := c.Call(ctx, api.MethodGroupCreate, api.GroupPayload{
		ID: g.ID, Name: "two", Members: []api.GroupMemberPayload{{Name: "b"}},
	}, &g)
	if err == nil {
		t.Fatal("group create duplicate id: want error, got nil")
	}
}

func TestGroupCreateCycle(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, nil)
	c.SetSession("sess-full", "full")
	ctx := context.Background()
	var g api.GroupView
	err := c.Call(ctx, api.MethodGroupCreate, api.GroupPayload{
		Name: "cyclic",
		Members: []api.GroupMemberPayload{
			{Name: "a", DependsOn: []string{"a"}},
		},
	}, &g)
	if err == nil {
		t.Fatal("group create with self-cycle: want error, got nil")
	}
}

func TestGroupRemoveEmptyID(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, nil)
	c.SetSession("sess-full", "full")
	ctx := context.Background()
	var out map[string]any
	err := c.Call(ctx, api.MethodGroupRemove, api.GroupPayload{}, &out)
	if err == nil || !strings.Contains(err.Error(), "id required") {
		t.Fatalf("group remove without id: want 'id required' error, got %v", err)
	}
}

func TestGroupRemoveNotFound(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, nil)
	c.SetSession("sess-full", "full")
	ctx := context.Background()
	var out map[string]any
	err := c.Call(ctx, api.MethodGroupRemove, api.GroupPayload{ID: "grp-nope"}, &out)
	if err == nil {
		t.Fatal("group remove unknown id: want error, got nil")
	}
}

func TestGroupRemoveDeniedForReadonly(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, nil)
	c.SetSession("sess-ro", "readonly")
	ctx := context.Background()
	var out map[string]any
	err := c.Call(ctx, api.MethodGroupRemove, api.GroupPayload{ID: "grp-x"}, &out)
	if err == nil || !strings.Contains(err.Error(), "permission") {
		t.Fatalf("readonly group remove: want permission_denied, got %v", err)
	}
}

func TestGroupStatusNotFound(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, nil)
	c.SetSession("sess-full", "full")
	ctx := context.Background()
	var out api.GroupView
	err := c.Call(ctx, api.MethodGroupStatus, api.GroupPayload{ID: "grp-nope"}, &out)
	if err == nil {
		t.Fatal("group status unknown id: want error, got nil")
	}
}

func TestGroupStatusByNameCrossProject(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, nil)
	c.SetSession("sess-full", "full")
	ctx := context.Background()
	var g api.GroupView
	if err := c.Call(ctx, api.MethodGroupCreate, api.GroupPayload{
		Name: "byname", ProjectID: "proj-a", Members: []api.GroupMemberPayload{{Name: "svc"}},
	}, &g); err != nil {
		t.Fatalf("group create: %v", err)
	}
	// Look up by name under a *different* project filter — resolveGroup falls
	// back to scanning all groups when the project-scoped lookup misses.
	var status api.GroupView
	if err := c.Call(ctx, api.MethodGroupStatus, api.GroupPayload{Name: "byname", ProjectID: "proj-b"}, &status); err != nil {
		t.Fatalf("group status by name cross-project: %v", err)
	}
	if status.ID != g.ID {
		t.Fatalf("status.ID = %q, want %q", status.ID, g.ID)
	}
}

func TestGroupStatusByNameNotFound(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, nil)
	c.SetSession("sess-full", "full")
	ctx := context.Background()
	var out api.GroupView
	err := c.Call(ctx, api.MethodGroupStatus, api.GroupPayload{}, &out)
	if err == nil || !strings.Contains(err.Error(), "group id or name required") {
		t.Fatalf("group status with no id/name: want error, got %v", err)
	}
	err = c.Call(ctx, api.MethodGroupStatus, api.GroupPayload{Name: "nonexistent"}, &out)
	if err == nil {
		t.Fatal("group status by unknown name: want error, got nil")
	}
}

func TestGroupLifecycleEmptyGroupStillChecksCapability(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, nil)
	c.SetSession("sess-full", "full")
	ctx := context.Background()
	var g api.GroupView
	if err := c.Call(ctx, api.MethodGroupCreate, api.GroupPayload{Name: "empty"}, &g); err != nil {
		t.Fatalf("group create: %v", err)
	}
	// Empty group: the capability check must still run even though the
	// member loop is a no-op.
	c.SetSession("sess-ro", "readonly")
	var out map[string]any
	err := c.Call(ctx, api.MethodGroupStop, api.GroupPayload{ID: g.ID}, &out)
	if err == nil || !strings.Contains(err.Error(), "permission") {
		t.Fatalf("readonly group.stop on empty group: want permission_denied, got %v", err)
	}
}

func TestGroupLifecycleStartStopRestart(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, func(cfg *config.Config) { cfg.Sandbox.Default = "off" })
	c.SetSession("sess-full", "full")
	ctx := context.Background()
	var start api.StartResult
	if err := c.Call(ctx, api.MethodStart, api.StartPayload{
		Name: "member1", Command: []string{"sleep", "60"}, Sandbox: "off",
	}, &start); err != nil {
		t.Fatalf("start: %v", err)
	}
	var g api.GroupView
	if err := c.Call(ctx, api.MethodGroupCreate, api.GroupPayload{
		Name: "lifecycle", Members: []api.GroupMemberPayload{{Name: "member1"}},
	}, &g); err != nil {
		t.Fatalf("group create: %v", err)
	}
	var out map[string]any
	if err := c.Call(ctx, api.MethodGroupStop, api.GroupPayload{ID: g.ID}, &out); err != nil {
		t.Fatalf("group stop: %v", err)
	}
	if err := c.Call(ctx, api.MethodGroupStart, api.GroupPayload{ID: g.ID}, &out); err != nil {
		t.Fatalf("group start: %v", err)
	}
	if err := c.Call(ctx, api.MethodGroupRestart, api.GroupPayload{ID: g.ID}, &out); err != nil {
		t.Fatalf("group restart: %v", err)
	}
	if !auditHas(t, c, func(r auditRow) bool { return r.Action == "group.restart" && r.Target == g.ID }) {
		t.Fatal("no group.restart audit row")
	}
}

func TestGroupLifecycleUnknownGroup(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, nil)
	c.SetSession("sess-full", "full")
	ctx := context.Background()
	var out map[string]any
	err := c.Call(ctx, api.MethodGroupStop, api.GroupPayload{ID: "grp-nope"}, &out)
	if err == nil {
		t.Fatal("group.stop unknown group: want error, got nil")
	}
}

// --- profile.* ---

func TestProfileGetMissingID(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, nil)
	c.SetSession("sess-full", "full")
	ctx := context.Background()
	var out api.ProfileView
	err := c.Call(ctx, api.MethodProfileGet, api.ProfilePayload{}, &out)
	if err == nil || !strings.Contains(err.Error(), "id required") {
		t.Fatalf("profile.get without id: want error, got %v", err)
	}
}

func TestProfileGetNotFound(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, nil)
	c.SetSession("sess-full", "full")
	ctx := context.Background()
	var out api.ProfileView
	err := c.Call(ctx, api.MethodProfileGet, api.ProfilePayload{ID: "prof-nope"}, &out)
	if err == nil {
		t.Fatal("profile.get unknown id: want error, got nil")
	}
}

func TestProfileCreateBadPayload(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, nil)
	c.SetSession("sess-full", "full")
	ctx := context.Background()
	var out map[string]any
	err := c.Call(ctx, api.MethodProfileCreate, []string{"bad"}, &out)
	if err == nil || !strings.Contains(err.Error(), "bad profile payload") {
		t.Fatalf("profile.create with array payload: want error, got %v", err)
	}
}

func TestProfileUpdateBadPayload(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, nil)
	c.SetSession("sess-full", "full")
	ctx := context.Background()
	var out map[string]any
	err := c.Call(ctx, api.MethodProfileUpdate, []string{"bad"}, &out)
	if err == nil || !strings.Contains(err.Error(), "bad profile payload") {
		t.Fatalf("profile.update with array payload: want error, got %v", err)
	}
}

func TestProfileUpdateNotFound(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, nil)
	c.SetSession("sess-full", "full")
	ctx := context.Background()
	var out api.ProfileView
	err := c.Call(ctx, api.MethodProfileUpdate, api.ProfilePayload{ID: "prof-nope", Name: "x"}, &out)
	if err == nil {
		t.Fatal("profile.update unknown id: want error, got nil")
	}
}

func TestProfileUpdateSuccess(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, nil)
	c.SetSession("sess-full", "full")
	ctx := context.Background()
	var pr api.ProfileView
	if err := c.Call(ctx, api.MethodProfileCreate, api.ProfilePayload{Name: "orig", ProjectID: "/tmp/proj-a", Env: map[string]string{"A": "1"}}, &pr); err != nil {
		t.Fatalf("create: %v", err)
	}
	var updated api.ProfileView
	if err := c.Call(ctx, api.MethodProfileUpdate, api.ProfilePayload{ID: pr.ID, Name: "renamed", Env: map[string]string{"B": "2"}}, &updated); err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Name != "renamed" {
		t.Fatalf("updated name = %q, want renamed", updated.Name)
	}
	if !auditHas(t, c, func(r auditRow) bool { return r.Action == "profile.update" && r.Target == pr.ID }) {
		t.Fatal("no profile.update audit row")
	}
}

func TestProfileDeleteMissingID(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, nil)
	c.SetSession("sess-full", "full")
	ctx := context.Background()
	var out map[string]any
	err := c.Call(ctx, api.MethodProfileDelete, api.ProfilePayload{}, &out)
	if err == nil || !strings.Contains(err.Error(), "id required") {
		t.Fatalf("profile.delete without id: want error, got %v", err)
	}
}

func TestProfileDeleteNotFound(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, nil)
	c.SetSession("sess-full", "full")
	ctx := context.Background()
	var out map[string]any
	err := c.Call(ctx, api.MethodProfileDelete, api.ProfilePayload{ID: "prof-nope"}, &out)
	if err == nil {
		t.Fatal("profile.delete unknown id: want error, got nil")
	}
}

func TestProfileDeleteSuccess(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, nil)
	c.SetSession("sess-full", "full")
	ctx := context.Background()
	var pr api.ProfileView
	if err := c.Call(ctx, api.MethodProfileCreate, api.ProfilePayload{Name: "todelete", ProjectID: "/tmp/proj-b"}, &pr); err != nil {
		t.Fatalf("create: %v", err)
	}
	var out map[string]any
	if err := c.Call(ctx, api.MethodProfileDelete, api.ProfilePayload{ID: pr.ID}, &out); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if !auditHas(t, c, func(r auditRow) bool { return r.Action == "profile.delete" && r.Target == pr.ID }) {
		t.Fatal("no profile.delete audit row")
	}
}

func TestProfileUseOpensNewSessionWhenNoneGiven(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, nil)
	c.SetSession("", "full")
	ctx := context.Background()
	if _, err := (func() (any, error) {
		var out map[string]any
		err := c.Call(ctx, api.MethodProfileCreate, api.ProfilePayload{Name: "useme", ProjectID: "/tmp/proj-c"}, &out)
		return out, err
	})(); err != nil {
		t.Fatalf("create: %v", err)
	}
	var out map[string]any
	// No request session and no payload session: doProfileUse opens a fresh one.
	if err := c.Call(ctx, api.MethodProfileUse, api.ProfilePayload{Name: "useme", ProjectID: "/tmp/proj-c"}, &out); err != nil {
		t.Fatalf("profile.use: %v", err)
	}
	if out["session"] == "" || out["session"] == nil {
		t.Fatalf("profile.use result missing session: %+v", out)
	}
}

func TestProfileUseWithPayloadSession(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, nil)
	c.SetSession("sess-use", "full")
	ctx := context.Background()
	var out map[string]any
	if err := c.Call(ctx, api.MethodProfileCreate, api.ProfilePayload{Name: "useme2", ProjectID: "/tmp/proj-d"}, &out); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := c.Call(ctx, api.MethodProfileUse, api.ProfilePayload{Name: "useme2", Session: "explicit-session"}, &out); err != nil {
		t.Fatalf("profile.use: %v", err)
	}
	if out["session"] != "explicit-session" {
		t.Fatalf("session = %v, want explicit-session", out["session"])
	}
}

func TestProfileListRedactsValuesForNonFullRole(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, nil)
	c.SetSession("sess-full", "full")
	ctx := context.Background()
	var pr api.ProfileView
	if err := c.Call(ctx, api.MethodProfileCreate, api.ProfilePayload{
		Name: "redact-me", ProjectID: "/tmp/proj-e", Env: map[string]string{"SECRET": "hunter2"},
	}, &pr); err != nil {
		t.Fatalf("create: %v", err)
	}
	// agent role has process:read but not secrets:read_values.
	c.SetSession("sess-agent", "agent")
	var list []api.ProfileView
	if err := c.Call(ctx, api.MethodProfileList, api.ProfilePayload{}, &list); err != nil {
		t.Fatalf("list: %v", err)
	}
	found := false
	for _, p := range list {
		if p.ID == pr.ID {
			found = true
			if p.Env["SECRET"] != "[redacted]" {
				t.Fatalf("agent role saw unredacted secret: %+v", p.Env)
			}
		}
	}
	if !found {
		t.Fatal("created profile not found in list")
	}

	c.SetSession("sess-full2", "full")
	var got api.ProfileView
	if err := c.Call(ctx, api.MethodProfileGet, api.ProfilePayload{ID: pr.ID}, &got); err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Env["SECRET"] != "hunter2" {
		t.Fatalf("full role should see raw value, got %q", got.Env["SECRET"])
	}
}

func TestProfileCreateDeniedForReadonly(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, nil)
	c.SetSession("sess-ro", "readonly")
	ctx := context.Background()
	var out map[string]any
	err := c.Call(ctx, api.MethodProfileCreate, api.ProfilePayload{Name: "x"}, &out)
	if err == nil || !strings.Contains(err.Error(), "permission") {
		t.Fatalf("readonly profile.create: want permission_denied, got %v", err)
	}
}

// --- session.info / session.end ---

func TestSessionInfoOpensNewWhenNoSessionGiven(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, nil)
	c.SetSession("", "agent")
	ctx := context.Background()
	var info api.SessionInfoResult
	if err := c.Call(ctx, api.MethodSessionInfo, nil, &info); err != nil {
		t.Fatalf("session.info: %v", err)
	}
	if info.ID == "" {
		t.Fatal("session.info: empty session id")
	}
}

func TestSessionEndMissingID(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, nil)
	c.SetSession("", "full")
	ctx := context.Background()
	var out map[string]any
	err := c.Call(ctx, api.MethodSessionEnd, map[string]any{}, &out)
	if err == nil || !strings.Contains(err.Error(), "session required") {
		t.Fatalf("session.end with no id: want error, got %v", err)
	}
}

func TestSessionEndByIDField(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, nil)
	c.SetSession("sess-ended-by-field", "full")
	ctx := context.Background()
	// Open the session first via session.info.
	if err := c.Call(ctx, api.MethodSessionInfo, nil, &api.SessionInfoResult{}); err != nil {
		t.Fatalf("session.info: %v", err)
	}
	var out map[string]any
	if err := c.Call(ctx, api.MethodSessionEnd, map[string]any{"id": "sess-ended-by-field"}, &out); err != nil {
		t.Fatalf("session.end: %v", err)
	}
	if !auditHas(t, c, func(r auditRow) bool { return r.Action == "session.end" }) {
		t.Fatal("no session.end audit row")
	}
}

func TestSessionEndUnknownFallsBackToEnsureThenEnd(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, nil)
	c.SetSession("brand-new-harness-id", "full")
	ctx := context.Background()
	// The session was never opened via session.info; session.end must still
	// succeed by treating the harness id as an ensureSession fallback.
	var out map[string]any
	if err := c.Call(ctx, api.MethodSessionEnd, map[string]any{"id": "brand-new-harness-id"}, &out); err != nil {
		t.Fatalf("session.end (never-opened session): %v", err)
	}
}

func TestSessionEndDeniedForReadonlyWithoutSOD(t *testing.T) {
	t.Parallel()
	// readonly HAS session:end (self-management) so a session with no
	// stop-on-disconnect processes should succeed even for readonly.
	c, _ := newTestDaemon(t, nil)
	c.SetSession("sess-ro-end", "readonly")
	ctx := context.Background()
	var out map[string]any
	if err := c.Call(ctx, api.MethodSessionEnd, map[string]any{"id": "sess-ro-end"}, &out); err != nil {
		t.Fatalf("readonly session.end without SOD processes should succeed: %v", err)
	}
}

// --- session.share / session.unshare ---

func TestShareMissingFields(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, nil)
	c.SetSession("sess-full", "full")
	ctx := context.Background()
	var out map[string]any
	err := c.Call(ctx, api.MethodShare, api.SharePayload{}, &out)
	if err == nil || !strings.Contains(err.Error(), "target and to_session required") {
		t.Fatalf("share with no target/to_session: want error, got %v", err)
	}
}

func TestShareBadPayload(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, nil)
	c.SetSession("sess-full", "full")
	ctx := context.Background()
	var out map[string]any
	err := c.Call(ctx, api.MethodShare, []string{"bad"}, &out)
	if err == nil || !strings.Contains(err.Error(), "bad share payload") {
		t.Fatalf("share with array payload: want error, got %v", err)
	}
}

func TestShareGrantAndUnshare(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, nil)
	c.SetSession("sess-full", "full")
	ctx := context.Background()
	var out map[string]any
	if err := c.Call(ctx, api.MethodShare, api.SharePayload{
		Target: "proc-shared", ToSession: "sess-other", Cap: "process:stop",
	}, &out); err != nil {
		t.Fatalf("share: %v", err)
	}
	if out["shared"] != "true" {
		t.Fatalf("share result: %+v", out)
	}
	if !auditHas(t, c, func(r auditRow) bool { return r.Action == "session.share" }) {
		t.Fatal("no session.share audit row")
	}
	var out2 map[string]any
	if err := c.Call(ctx, api.MethodUnshare, api.SharePayload{
		Target: "proc-shared", ToSession: "sess-other", Cap: "process:stop",
	}, &out2); err != nil {
		t.Fatalf("unshare: %v", err)
	}
	if !auditHas(t, c, func(r auditRow) bool { return r.Action == "session.unshare" }) {
		t.Fatal("no session.unshare audit row")
	}
}

func TestShareDeniedForAgent(t *testing.T) {
	t.Parallel()
	// session:share is operator+/full only — agent lacks it.
	c, _ := newTestDaemon(t, nil)
	c.SetSession("sess-agent", "agent")
	ctx := context.Background()
	var out map[string]any
	err := c.Call(ctx, api.MethodShare, api.SharePayload{Target: "p", ToSession: "s", Cap: "process:stop"}, &out)
	if err == nil || !strings.Contains(err.Error(), "permission") {
		t.Fatalf("agent share: want permission_denied, got %v", err)
	}
}

// --- doGroupCreate's generic error fallback (neither ErrExists nor ErrCycle) ---

func TestGroupCreateUnknownDependency(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, nil)
	c.SetSession("sess-full", "full")
	ctx := context.Background()
	var out map[string]any
	err := c.Call(ctx, api.MethodGroupCreate, api.GroupPayload{
		Name: "baddep",
		Members: []api.GroupMemberPayload{
			{Name: "a", DependsOn: []string{"ghost"}},
		},
	}, &out)
	if err == nil || !strings.Contains(err.Error(), "unknown member") {
		t.Fatalf("group create depending on unknown member: want error, got %v", err)
	}
}

func TestGroupRemoveSuccess(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, nil)
	c.SetSession("sess-full", "full")
	ctx := context.Background()
	var g api.GroupView
	if err := c.Call(ctx, api.MethodGroupCreate, api.GroupPayload{Name: "removeme"}, &g); err != nil {
		t.Fatalf("create: %v", err)
	}
	var out map[string]string
	if err := c.Call(ctx, api.MethodGroupRemove, api.GroupPayload{ID: g.ID}, &out); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if out["removed"] != "true" {
		t.Fatalf("remove result: %+v", out)
	}
	if !auditHas(t, c, func(r auditRow) bool { return r.Action == "group.remove" && r.Target == g.ID }) {
		t.Fatal("no group.remove audit row")
	}
}

func TestGroupStatusDegradedPhase(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, func(cfg *config.Config) { cfg.Sandbox.Default = "off" })
	c.SetSession("sess-full", "full")
	ctx := context.Background()
	var start api.StartResult
	if err := c.Call(ctx, api.MethodStart, api.StartPayload{
		Name: "up-member", Command: []string{"sleep", "60"}, Sandbox: "off",
	}, &start); err != nil {
		t.Fatalf("start: %v", err)
	}
	var g api.GroupView
	if err := c.Call(ctx, api.MethodGroupCreate, api.GroupPayload{
		Name: "degraded",
		Members: []api.GroupMemberPayload{
			{Name: "up-member"}, {Name: "never-started"},
		},
	}, &g); err != nil {
		t.Fatalf("create: %v", err)
	}
	var status api.GroupView
	if err := c.Call(ctx, api.MethodGroupStatus, api.GroupPayload{ID: g.ID}, &status); err != nil {
		t.Fatalf("status: %v", err)
	}
	if status.Phase != "degraded" {
		t.Fatalf("phase = %q, want degraded: %+v", status.Phase, status)
	}
}

func TestGroupLifecycleMemberFailureMarksEntryError(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, func(cfg *config.Config) { cfg.Sandbox.Default = "off" })
	c.SetSession("sess-full", "full")
	ctx := context.Background()
	var g api.GroupView
	if err := c.Call(ctx, api.MethodGroupCreate, api.GroupPayload{
		Name: "willfail",
		Members: []api.GroupMemberPayload{
			{Name: "never-existed-anywhere"},
		},
	}, &g); err != nil {
		t.Fatalf("create: %v", err)
	}
	var out map[string]any
	if err := c.Call(ctx, api.MethodGroupStart, api.GroupPayload{ID: g.ID}, &out); err != nil {
		t.Fatalf("group.start (should succeed overall with a per-member error entry): %v", err)
	}
	members, ok := out["members"].([]any)
	if !ok || len(members) != 1 {
		t.Fatalf("members: %+v", out)
	}
	entry, ok := members[0].(map[string]any)
	if !ok || entry["status"] != "error" {
		t.Fatalf("expected member entry with status=error, got %+v", entry)
	}
}

// --- profile.create legacy "project" field + duplicate-name conflict ---

func TestProfileCreateLegacyProjectField(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, nil)
	c.SetSession("sess-full", "full")
	ctx := context.Background()
	var pr api.ProfileView
	if err := c.Call(ctx, api.MethodProfileCreate, api.ProfilePayload{
		Name: "legacy", Project: "/tmp/legacy-proj",
	}, &pr); err != nil {
		t.Fatalf("create with legacy project field: %v", err)
	}
	if pr.ID == "" {
		t.Fatal("empty profile id")
	}
}

func TestProfileCreateDuplicateNameConflict(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, nil)
	c.SetSession("sess-full", "full")
	ctx := context.Background()
	var pr api.ProfileView
	if err := c.Call(ctx, api.MethodProfileCreate, api.ProfilePayload{
		Name: "dupe", ProjectID: "/tmp/proj-dupe",
	}, &pr); err != nil {
		t.Fatalf("create: %v", err)
	}
	var out map[string]any
	err := c.Call(ctx, api.MethodProfileCreate, api.ProfilePayload{
		Name: "dupe", ProjectID: "/tmp/proj-dupe",
	}, &out)
	if err == nil {
		t.Fatal("duplicate profile create: want error, got nil")
	}
}

func TestProfileUseInvalidNameErrors(t *testing.T) {
	t.Parallel()
	// profile.Store.Use does not check existence (a session may select a
	// profile before it's created), but it does validate the name format.
	c, _ := newTestDaemon(t, nil)
	c.SetSession("sess-use-err", "full")
	ctx := context.Background()
	var out map[string]any
	err := c.Call(ctx, api.MethodProfileUse, api.ProfilePayload{Name: "Not A Valid Name!"}, &out)
	if err == nil {
		t.Fatal("profile.use with an invalid name format: want error, got nil")
	}
}

// --- health_check: HTTP URL probe path + recovery from unhealthy ---

func TestDoHealthCheckURLProbeAndRecovery(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, func(cfg *config.Config) { cfg.Sandbox.Default = "off" })
	c.SetSession("sess-full", "full")
	ctx := context.Background()

	healthy := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if healthy {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	}))
	t.Cleanup(srv.Close)

	var start api.StartResult
	if err := c.Call(ctx, api.MethodStart, api.StartPayload{
		Name: "urlhealth", Command: []string{"sleep", "60"}, Sandbox: "off", HealthURL: srv.URL,
	}, &start); err != nil {
		t.Fatalf("start: %v", err)
	}

	var bad api.HealthCheckResult
	if err := c.Call(ctx, api.MethodHealthCheck, api.IDPayload{ID: start.ID}, &bad); err != nil {
		t.Fatalf("health_check (unhealthy): %v", err)
	}
	if bad.OK {
		t.Fatalf("expected unhealthy result, got %+v", bad)
	}

	healthy = true
	var good api.HealthCheckResult
	if err := c.Call(ctx, api.MethodHealthCheck, api.IDPayload{ID: start.ID}, &good); err != nil {
		t.Fatalf("health_check (recovered): %v", err)
	}
	if !good.OK || good.Status != "running" {
		t.Fatalf("expected recovered running status, got %+v", good)
	}
}

// --- session.info / session.end resolving an already-open session by its
// internal id (as opposed to a harness id, which never matches the internal
// sess- ULID key that Registry.Get looks up by). ---

func TestSessionInfoFindsExistingByInternalID(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, nil)
	c.SetSession("harness-a", "full")
	ctx := context.Background()
	var first api.SessionInfoResult
	if err := c.Call(ctx, api.MethodSessionInfo, nil, &first); err != nil {
		t.Fatalf("session.info (open): %v", err)
	}
	if first.ID == "" {
		t.Fatal("empty session id")
	}
	// Re-request using the *internal* id this time: ensureSession's
	// Registry.Get(req.Session) branch (as opposed to Open) is only reachable
	// when the asserted session field is already the internal sess- id.
	c.SetSession(first.ID, "full")
	var second api.SessionInfoResult
	if err := c.Call(ctx, api.MethodSessionInfo, nil, &second); err != nil {
		t.Fatalf("session.info (by internal id): %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("second.ID = %q, want %q (same identity)", second.ID, first.ID)
	}
}

func TestSessionEndFindsExistingByInternalID(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, nil)
	c.SetSession("harness-b", "full")
	ctx := context.Background()
	var info api.SessionInfoResult
	if err := c.Call(ctx, api.MethodSessionInfo, nil, &info); err != nil {
		t.Fatalf("session.info: %v", err)
	}
	c.SetSession(info.ID, "full")
	var out map[string]any
	if err := c.Call(ctx, api.MethodSessionEnd, map[string]any{}, &out); err != nil {
		t.Fatalf("session.end (by internal id): %v", err)
	}
}

func TestSessionEndWhitespaceIDStillSucceeds(t *testing.T) {
	t.Parallel()
	// A whitespace-only id is non-empty (passes doSessionEnd's required
	// check) but trims to empty in sodIDsForSession's own key-set builder.
	// It doesn't reach sodIDsForSession's len(keySet)==0 guard in practice:
	// the ensureSession fallback always contributes a real sess- ULID to
	// sessionKeys before that helper is ever called (see FABLE_REVIEW.md).
	c, _ := newTestDaemon(t, nil)
	c.SetSession("sess-ws", "full")
	ctx := context.Background()
	var out map[string]any
	if err := c.Call(ctx, api.MethodSessionEnd, map[string]any{"id": "   "}, &out); err != nil {
		t.Fatalf("session.end with whitespace id: %v", err)
	}
}

// --- profile.update / profile.delete / profile.use deny paths (dispatchExtra
// case arms 113-127 gate each on CapProfileManage; only profile.create's arm
// had a deny test) ---

func TestProfileUpdateDeniedForReadonly(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, nil)
	c.SetSession("sess-ro", "readonly")
	ctx := context.Background()
	var out map[string]any
	err := c.Call(ctx, api.MethodProfileUpdate, api.ProfilePayload{ID: "x", Name: "y"}, &out)
	if err == nil || !strings.Contains(err.Error(), "permission") {
		t.Fatalf("readonly profile.update: want permission_denied, got %v", err)
	}
}

func TestProfileDeleteDeniedForReadonly(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, nil)
	c.SetSession("sess-ro", "readonly")
	ctx := context.Background()
	var out map[string]any
	err := c.Call(ctx, api.MethodProfileDelete, api.ProfilePayload{ID: "x"}, &out)
	if err == nil || !strings.Contains(err.Error(), "permission") {
		t.Fatalf("readonly profile.delete: want permission_denied, got %v", err)
	}
}

func TestProfileUseDeniedForReadonly(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, nil)
	c.SetSession("sess-ro", "readonly")
	ctx := context.Background()
	var out map[string]any
	err := c.Call(ctx, api.MethodProfileUse, api.ProfilePayload{Name: "x"}, &out)
	if err == nil || !strings.Contains(err.Error(), "permission") {
		t.Fatalf("readonly profile.use: want permission_denied, got %v", err)
	}
}
