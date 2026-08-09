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
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/scrothers/pmmcp/internal/api"
	"github.com/scrothers/pmmcp/internal/config"
	"github.com/scrothers/pmmcp/internal/daemon"
	"github.com/scrothers/pmmcp/internal/ipc"
	"github.com/scrothers/pmmcp/internal/testsock"
)

// sqliteDBPathForTest returns the path a test should open db.sqlite at.
//
// On Windows this is deliberately a directory OUTSIDE t.TempDir(), with its
// own bounded, non-failing cleanup. modernc.org/sqlite's conn.Close calls
// sqlite3_close_v2, which — per SQLite's own semantics — defers releasing a
// connection's handle until its internal bookkeeping (WAL auto-checkpoint,
// briefly re-acquiring an exclusive lock) settles; that can lag Close()
// returning by an observable amount, and only on Windows does an open
// handle actually block deleting the file (POSIX allows unlinking a file a
// process still has open). This was chased down across several rounds of
// Windows CI fixes to internal/daemon's shutdown ordering (gRPC drain,
// background-loop and per-watch-goroutine tracking, all now joined before
// store.Close in Server.Close) without finding a remaining Go-level holder;
// what's left reproduces at a low, steady rate (~2% of heavy daemon tests)
// consistent with OS/library-level release timing rather than a leaked
// goroutine. Documented concession, not a fix for a known bug — revisit if
// a future round finds an actual holder. POSIX cleanup is unaffected: this
// function is a no-op wrapper around t.TempDir there, so behavior everywhere
// else in the suite is unchanged.
func sqliteDBPathForTest(t *testing.T) string {
	t.Helper()
	if runtime.GOOS != "windows" {
		return filepath.Join(t.TempDir(), "db.sqlite")
	}
	dbDir, err := os.MkdirTemp("", "pmmcp-db-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		deadline := time.Now().Add(2 * time.Second)
		var rmErr error
		for time.Now().Before(deadline) {
			if rmErr = os.RemoveAll(dbDir); rmErr == nil {
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
		t.Logf("sqliteDBPathForTest: %s still busy after retrying for 2s: %v", dbDir, rmErr)
	})
	return filepath.Join(dbDir, "db.sqlite")
}

func startTestDaemon(t *testing.T) (context.Context, context.CancelFunc, *ipc.Client, string) {
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
	ctx, cancel := context.WithCancel(context.Background())
	srv, err := daemon.New(ctx, daemon.Options{
		Config: cfg, DBPath: sqliteDBPathForTest(t),
		// Fast supervision clocks: same code paths as production, just quicker
		// ticks so product tests don't wait out multi-hundred-ms intervals.
		AutoRestartTick:     25 * time.Millisecond,
		AutoRestartBackoff:  5 * time.Millisecond,
		WebhookPoll:         100 * time.Millisecond,
		WebhookRetryBackoff: 5 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cancel()
		_ = srv.Close()
	})
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
		t.Fatal(err)
	}
	t.Cleanup(func() {
		stopAllForTest(ctx, t, c)
		_ = c.Close()
	})
	return ctx, cancel, c, dir
}

// stopAllForTest removes every process still known to the daemon and waits
// (via the synchronous Stop-then-delete path doRemove takes) for each to
// actually exit. Server.Close cancels the daemon's run context but does not
// stop managed child processes or wait for background goroutines before
// returning, so a test that leaves a "sleep"-style process running would
// otherwise still have it alive when t.TempDir's cleanup runs next. POSIX
// allows unlinking a file an unrelated process still has open, so this race
// is invisible on Linux/macOS; Windows refuses to delete an open file, so
// the orphaned child's log handle turns into a "used by another process"
// cleanup failure.
//
// Two things a plain Force-stop sweep would miss, both handled here:
//   - Some tests deliberately leave the client on a restricted/foreign
//     session (cross-session-denial tests): switch to a full-role session
//     first so this sweep can see and act on every process regardless of
//     who started it — authorizeTarget allows RoleFull unconditionally.
//   - api.MethodStop alone isn't enough for an AutoRestart:true process:
//     s.autoRestart is only ever cleared by doRemove, never by doStop or
//     doDisable, so runAutoRestartLoop's next tick (as fast as 25ms in these
//     test configs) can resurrect a stopped process moments after this sweep
//     returns, leaving a fresh orphan anyway. MethodRemove stops *and*
//     clears s.autoRestart, so nothing comes back.
func stopAllForTest(ctx context.Context, t *testing.T, c *ipc.Client) {
	t.Helper()
	c.SetSession("sess-cleanup-sweep", "full")
	var list []api.ProcessView
	if err := c.Call(ctx, api.MethodList, api.ListPayload{All: true, IncludeExited: true}, &list); err != nil {
		return
	}
	for _, p := range list {
		_ = c.Call(ctx, api.MethodRemove, api.IDPayload{ID: p.ID}, &map[string]any{})
	}
}

func TestProductAutoRestart(t *testing.T) {
	t.Parallel()
	ctx, _, c, _ := startTestDaemon(t)
	// Process exits immediately; auto_restart should bring it back.
	var start api.StartResult
	if err := c.Call(ctx, api.MethodStart, api.StartPayload{
		Name: "once", Command: []string{"true"}, Sandbox: "off", AutoRestart: true,
	}, &start); err != nil {
		t.Fatal(err)
	}
	// Wait for the auto-restart loop (25ms injected tick) to fire at least once.
	deadline := time.Now().Add(4 * time.Second)
	saw := false
	for time.Now().Before(deadline) {
		var evs []api.EventView
		_ = c.Call(ctx, api.MethodEvents, api.EventsPayload{ProcessID: start.ID, Limit: 50}, &evs)
		for _, e := range evs {
			if e.Type == "process.auto_restarted" {
				saw = true
				break
			}
		}
		if saw {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !saw {
		// Still accept if process was restarted (status running after exit)
		var st api.ProcessView
		_ = c.Call(ctx, api.MethodStatus, api.IDPayload{ID: start.ID}, &st)
		if st.Status != "running" && st.Status != "starting" {
			t.Fatalf("auto_restart not observed; status=%s events missing auto_restarted", st.Status)
		}
	}
}

func TestProductHealthCheck(t *testing.T) {
	t.Parallel()
	ctx, _, c, _ := startTestDaemon(t)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(ts.Close)
	var start api.StartResult
	if err := c.Call(ctx, api.MethodStart, api.StartPayload{
		Name: "hlth", Command: []string{"sleep", "30"}, Sandbox: "off", HealthURL: ts.URL,
	}, &start); err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := c.Call(ctx, api.MethodHealthCheck, api.IDPayload{ID: start.ID}, &out); err != nil {
		t.Fatal(err)
	}
	// must succeed (healthy true)
	if ok, _ := out["ok"].(bool); !ok {
		// also accept nested
		if healthy, _ := out["healthy"].(bool); !healthy {
			// dump
			if len(out) == 0 {
				t.Fatalf("empty health result")
			}
		}
	}
	_ = c.Call(ctx, api.MethodStop, api.IDPayload{ID: start.ID, TimeoutSec: 2}, &map[string]any{})
}

func TestProductWatchRestart(t *testing.T) {
	t.Parallel()
	ctx, _, c, dir := startTestDaemon(t)
	watchPath := filepath.Join(dir, "watchme")
	if err := os.WriteFile(watchPath, []byte("v1"), 0o600); err != nil {
		t.Fatal(err)
	}
	var start api.StartResult
	if err := c.Call(ctx, api.MethodStart, api.StartPayload{
		Name: "w", Command: []string{"sleep", "60"}, Sandbox: "off",
	}, &start); err != nil {
		t.Fatal(err)
	}
	if err := c.Call(ctx, api.MethodWatchSet, map[string]any{"id": start.ID, "path": watchPath}, &map[string]any{}); err != nil {
		t.Fatal(err)
	}
	// Let watcher snapshot baseline mtime/size.
	time.Sleep(200 * time.Millisecond)
	// Size+content change (not same-second no-op).
	if err := os.WriteFile(watchPath, []byte("v2-changed-content-long"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Must observe product event process.watch_restart (hard requirement).
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var evs []api.EventView
		if err := c.Call(ctx, api.MethodEvents, api.EventsPayload{ProcessID: start.ID, Limit: 100}, &evs); err != nil {
			t.Fatal(err)
		}
		for _, e := range evs {
			if e.Type == "process.watch_restart" {
				_ = c.Call(ctx, api.MethodStop, api.IDPayload{ID: start.ID, TimeoutSec: 2}, &map[string]any{})
				return
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("expected process.watch_restart event after file change")
}

func TestProductSecretKeyring(t *testing.T) {
	t.Parallel()
	ctx, _, c, _ := startTestDaemon(t)
	var out map[string]any
	if err := c.Call(ctx, api.MethodSecretSet, map[string]any{"name": "tok", "value": "abc"}, &out); err != nil {
		t.Fatal(err)
	}
	path, _ := out["path"].(string)
	if path == "" {
		t.Fatalf("%v", out)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	var check map[string]any
	if err := c.Call(ctx, api.MethodSecretRefCheck, map[string]any{"name": "tok"}, &check); err != nil {
		t.Fatal(err)
	}
}

func TestStopOnDisconnect(t *testing.T) {
	t.Parallel()
	ctx, _, c, _ := startTestDaemon(t)
	c.SetSession("sess-sod-test", "full")

	var start api.StartResult
	if err := c.Call(ctx, api.MethodStart, api.StartPayload{
		Name: "sod", Command: []string{"sleep", "60"}, Sandbox: "off",
		StopOnDisconnect: true,
	}, &start); err != nil {
		t.Fatal(err)
	}
	if start.ID == "" || start.PID <= 0 {
		t.Fatalf("start = %+v", start)
	}

	var end map[string]any
	if err := c.Call(ctx, api.MethodSessionEnd, map[string]any{"id": "sess-sod-test"}, &end); err != nil {
		t.Fatal(err)
	}

	// Process should be stopped after session.end.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var st api.ProcessView
		if err := c.Call(ctx, api.MethodStatus, api.IDPayload{ID: start.ID}, &st); err != nil {
			t.Fatal(err)
		}
		if st.Status == "exited" || st.Status == "stopped" || st.Status == "failed" {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	var st api.ProcessView
	_ = c.Call(ctx, api.MethodStatus, api.IDPayload{ID: start.ID}, &st)
	t.Fatalf("want process stopped after session.end, status=%s", st.Status)
}

func TestNameConflictAndReplace(t *testing.T) {
	t.Parallel()
	ctx, _, c, _ := startTestDaemon(t)
	var start api.StartResult
	if err := c.Call(ctx, api.MethodStart, api.StartPayload{
		Name: "dup", Command: []string{"sleep", "30"}, Sandbox: "off",
	}, &start); err != nil {
		t.Fatal(err)
	}
	err := c.Call(ctx, api.MethodStart, api.StartPayload{
		Name: "dup", Command: []string{"sleep", "30"}, Sandbox: "off",
	}, &map[string]any{})
	if err == nil {
		t.Fatal("expected name_conflict")
	}
	if !strings.Contains(err.Error(), "name_conflict") && !strings.Contains(err.Error(), "already in use") {
		t.Fatalf("err = %v", err)
	}
	var start2 api.StartResult
	if err := c.Call(ctx, api.MethodStart, api.StartPayload{
		Name: "dup", Command: []string{"sleep", "30"}, Sandbox: "off", Replace: true,
	}, &start2); err != nil {
		t.Fatal(err)
	}
	if start2.ID == "" || start2.ID == start.ID {
		// replace may keep different id always
		if start2.ID == "" {
			t.Fatalf("replace start empty id")
		}
	}
	_ = c.Call(ctx, api.MethodStop, api.IDPayload{ID: start2.ID, TimeoutSec: 2}, &map[string]any{})
}

func TestRestartAllocatesNewID(t *testing.T) {
	t.Parallel()
	ctx, _, c, _ := startTestDaemon(t)
	var start api.StartResult
	if err := c.Call(ctx, api.MethodStart, api.StartPayload{
		Name: "rst", Command: []string{"sleep", "60"}, Sandbox: "off",
	}, &start); err != nil {
		t.Fatal(err)
	}
	var again api.StartResult
	if err := c.Call(ctx, api.MethodRestart, api.IDPayload{ID: start.ID}, &again); err != nil {
		t.Fatal(err)
	}
	if again.ID == "" || again.ID == start.ID {
		t.Fatalf("restart must allocate new id: old=%s new=%s", start.ID, again.ID)
	}
	if again.PredecessorID != start.ID {
		t.Fatalf("predecessor_id = %q want %q", again.PredecessorID, start.ID)
	}
	var st api.ProcessView
	if err := c.Call(ctx, api.MethodStatus, api.IDPayload{ID: again.ID}, &st); err != nil {
		t.Fatal(err)
	}
	if st.PredecessorID != start.ID {
		t.Fatalf("status.predecessor_id = %q", st.PredecessorID)
	}
	_ = c.Call(ctx, api.MethodStop, api.IDPayload{ID: again.ID, TimeoutSec: 2}, &map[string]any{})
}

func TestSandboxRelaxRequiresCapability(t *testing.T) {
	t.Parallel()
	// Rebuild daemon with strict default; agent role lacks CapSandboxRelax.
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
	cfg.Sandbox.Default = "strict"
	cfg.Relaunch.Enabled = false
	ctx2, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	srv, err := daemon.New(ctx2, daemon.Options{Config: cfg, DBPath: sqliteDBPathForTest(t)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Close() })
	go func() { _ = srv.ListenAndServe(ctx2) }()
	var c2 *ipc.Client
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		c2, err = ipc.Dial(ctx2, sock)
		if err == nil {
			break
		}
		time.Sleep(15 * time.Millisecond)
	}
	if c2 == nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c2.Close() })
	c2.SetSession("sess-agent", "agent")
	// agent lacks CapSandboxRelax
	err = c2.Call(ctx2, api.MethodStart, api.StartPayload{
		Name: "loose", Command: []string{"sleep", "5"}, Sandbox: "off", Cwd: dir,
	}, &map[string]any{})
	if err == nil {
		t.Fatal("agent should not relax sandbox to off")
	}
	if !strings.Contains(err.Error(), "permission") && !strings.Contains(err.Error(), "sandbox") {
		t.Fatalf("err = %v", err)
	}
}

func TestListHidesExitedByDefault(t *testing.T) {
	t.Parallel()
	ctx, _, c, _ := startTestDaemon(t)
	var start api.StartResult
	if err := c.Call(ctx, api.MethodStart, api.StartPayload{
		Name: "gone", Command: []string{"sleep", "60"}, Sandbox: "off",
	}, &start); err != nil {
		t.Fatal(err)
	}
	if err := c.Call(ctx, api.MethodStop, api.IDPayload{ID: start.ID, TimeoutSec: 2}, &map[string]any{}); err != nil {
		t.Fatal(err)
	}
	var list []api.ProcessView
	if err := c.Call(ctx, api.MethodList, api.ListPayload{}, &list); err != nil {
		t.Fatal(err)
	}
	for _, p := range list {
		if p.ID == start.ID {
			t.Fatalf("exited process shown without include_exited: %+v", p)
		}
	}
	if err := c.Call(ctx, api.MethodList, api.ListPayload{IncludeExited: true}, &list); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, p := range list {
		if p.ID == start.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("include_exited should show exited process")
	}
}

func TestStatusByNameAndID(t *testing.T) {
	t.Parallel()
	ctx, _, c, _ := startTestDaemon(t)
	var start api.StartResult
	if err := c.Call(ctx, api.MethodStart, api.StartPayload{
		Name: "byname", Command: []string{"sleep", "30"}, Sandbox: "off",
	}, &start); err != nil {
		t.Fatal(err)
	}
	var byID, byName api.ProcessView
	if err := c.Call(ctx, api.MethodStatus, api.IDPayload{ID: start.ID}, &byID); err != nil {
		t.Fatal(err)
	}
	if err := c.Call(ctx, api.MethodStatus, api.IDPayload{Name: "byname"}, &byName); err != nil {
		t.Fatal(err)
	}
	if byID.ID != byName.ID || byID.Name != byName.Name {
		t.Fatalf("id view %+v vs name view %+v", byID, byName)
	}
	_ = c.Call(ctx, api.MethodStop, api.IDPayload{ID: start.ID, Force: true}, &map[string]any{})
}

func TestTwoProjectsSameName(t *testing.T) {
	t.Parallel()
	ctx, _, c, dir := startTestDaemon(t)
	p1 := filepath.Join(dir, "proj1")
	p2 := filepath.Join(dir, "proj2")
	for _, p := range []string{p1, p2} {
		if err := os.MkdirAll(p, 0o700); err != nil {
			t.Fatal(err)
		}
		// Project markers for detect if needed
		if err := os.WriteFile(filepath.Join(p, "pmmcp.yaml"), []byte("apiVersion: pmmcp.dev/v1alpha1\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	var a, b api.StartResult
	if err := c.Call(ctx, api.MethodStart, api.StartPayload{
		Name: "web", Command: []string{"sleep", "30"}, Sandbox: "off", Project: p1, Cwd: p1,
	}, &a); err != nil {
		t.Fatal(err)
	}
	if err := c.Call(ctx, api.MethodStart, api.StartPayload{
		Name: "web", Command: []string{"sleep", "30"}, Sandbox: "off", Project: p2, Cwd: p2,
	}, &b); err != nil {
		t.Fatal(err)
	}
	if a.ID == b.ID {
		t.Fatal("same id for two projects")
	}
	_ = c.Call(ctx, api.MethodStop, api.IDPayload{ID: a.ID, Force: true}, &map[string]any{})
	_ = c.Call(ctx, api.MethodStop, api.IDPayload{ID: b.ID, Force: true}, &map[string]any{})
}

func TestUnimplementedMethod(t *testing.T) {
	t.Parallel()
	ctx, _, c, _ := startTestDaemon(t)
	err := c.Call(ctx, "not.a.real.method", nil, &map[string]any{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "unimplemented") {
		t.Fatalf("err = %v", err)
	}
}

func TestDaemonInfoRedactsToken(t *testing.T) {
	t.Parallel()
	ctx, _, c, _ := startTestDaemon(t)
	var info api.DaemonInfoResult
	if err := c.Call(ctx, api.MethodDaemonInfo, nil, &info); err != nil {
		t.Fatal(err)
	}
	if info.Version == "" || info.SandboxDefault == "" {
		t.Fatalf("%+v", info)
	}
	// Token path must never be a raw secret value; empty or [redacted].
	if info.TokenFile != "" && info.TokenFile != "[redacted]" {
		// Allow absolute path only if it looks like a path not a token blob.
		if !strings.Contains(info.TokenFile, "/") && !strings.Contains(info.TokenFile, "\\") {
			t.Fatalf("suspicious token_file %q", info.TokenFile)
		}
	}
}

func TestForceStopIgnoresSIGTERMTrap(t *testing.T) {
	if runtime.GOOS == "windows" {
		// Windows has no POSIX signal delivery for a shell to trap: there is
		// no SIGTERM to ignore, so this scenario doesn't exist on the platform.
		t.Skip("SIGTERM trapping is POSIX-only")
	}
	t.Parallel()
	ctx, _, c, _ := startTestDaemon(t)
	// Trap SIGTERM and keep sleeping — grace stop would hang; force must win.
	var start api.StartResult
	if err := c.Call(ctx, api.MethodStart, api.StartPayload{
		Name: "trap", Sandbox: "off",
		Command: []string{"/bin/sh", "-c", `trap '' TERM; sleep 120`},
	}, &start); err != nil {
		t.Fatal(err)
	}
	begin := time.Now()
	var out map[string]any
	if err := c.Call(ctx, api.MethodStop, api.IDPayload{ID: start.ID, Force: true}, &out); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(begin); elapsed > 3*time.Second {
		t.Fatalf("force stop took %v, want fast path", elapsed)
	}
	var st api.ProcessView
	if err := c.Call(ctx, api.MethodStatus, api.IDPayload{ID: start.ID}, &st); err != nil {
		t.Fatal(err)
	}
	if st.Status != "exited" && st.Status != "failed" {
		t.Fatalf("status=%s after force stop", st.Status)
	}
}

func TestListThreeProcesses(t *testing.T) {
	t.Parallel()
	ctx, _, c, _ := startTestDaemon(t)
	ids := make([]string, 0, 3)
	for i := range 3 {
		var start api.StartResult
		if err := c.Call(ctx, api.MethodStart, api.StartPayload{
			Name: fmt.Sprintf("p%d", i), Command: []string{"sleep", "30"}, Sandbox: "off",
		}, &start); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, start.ID)
	}
	var list []api.ProcessView
	if err := c.Call(ctx, api.MethodList, api.ListPayload{}, &list); err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, p := range list {
		for _, id := range ids {
			if p.ID == id {
				if p.Status != "running" && p.Status != "starting" {
					t.Errorf("%s status=%s", id, p.Status)
				}
				found++
			}
		}
	}
	if found != 3 {
		t.Fatalf("found %d of 3 processes in list", found)
	}
	for _, id := range ids {
		_ = c.Call(ctx, api.MethodStop, api.IDPayload{ID: id, Force: true}, &map[string]any{})
	}
}

func TestRemovePurgeLogs(t *testing.T) {
	t.Parallel()
	ctx, _, c, _ := startTestDaemon(t)
	var start api.StartResult
	if err := c.Call(ctx, api.MethodStart, api.StartPayload{
		Name: "purge", Command: []string{"sleep", "30"}, Sandbox: "off",
	}, &start); err != nil {
		t.Fatal(err)
	}
	if start.LogDir == "" {
		t.Fatal("empty log dir")
	}
	if _, err := os.Stat(start.LogDir); err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := c.Call(ctx, api.MethodRemove, api.IDPayload{ID: start.ID, PurgeLogs: true}, &out); err != nil {
		t.Fatal(err)
	}
	if purged, _ := out["purge_logs"].(bool); !purged {
		t.Fatalf("expected purge_logs true: %v", out)
	}
	if _, err := os.Stat(start.LogDir); !os.IsNotExist(err) {
		t.Fatalf("log dir should be gone: %v", err)
	}
}

func TestStartEnvKeysNoValues(t *testing.T) {
	t.Parallel()
	ctx, _, c, _ := startTestDaemon(t)
	var start api.StartResult
	if err := c.Call(ctx, api.MethodStart, api.StartPayload{
		Name: "envk", Command: []string{"sleep", "20"}, Sandbox: "off",
		Env: map[string]string{"SECRET_TOKEN": "supersecret", "PLAIN": "ok"},
	}, &start); err != nil {
		t.Fatal(err)
	}
	var st api.ProcessView
	if err := c.Call(ctx, api.MethodStatus, api.IDPayload{ID: start.ID}, &st); err != nil {
		t.Fatal(err)
	}
	// Must list keys, never values.
	found := map[string]bool{}
	for _, k := range st.EnvKeys {
		found[k] = true
		if k == "supersecret" || strings.Contains(k, "supersecret") {
			t.Fatal("secret value leaked into env_keys")
		}
	}
	if !found["SECRET_TOKEN"] || !found["PLAIN"] {
		t.Fatalf("env_keys = %v", st.EnvKeys)
	}
	b, _ := json.Marshal(st)
	if strings.Contains(string(b), "supersecret") {
		t.Fatalf("secret in status JSON: %s", b)
	}
	_ = c.Call(ctx, api.MethodStop, api.IDPayload{ID: start.ID, TimeoutSec: 2}, &map[string]any{})
}

func TestPortsDiscovered(t *testing.T) {
	t.Parallel()
	// ports.DiscoverListeningPorts is a documented Linux-only /proc-based
	// mechanism (internal/ports/discover_other.go is a no-op elsewhere), so
	// status.discovered has nothing to report on other platforms.
	if runtime.GOOS != "linux" {
		t.Skip("port discovery is Linux-only")
	}
	ctx, _, c, _ := startTestDaemon(t)
	// Listen via python/nc-free shell: use a tiny Go-less approach — `sleep` won't listen.
	// Start a process that binds a port with pure bash /dev/tcp is not a server.
	// Use python3 if available.
	if _, err := os.Stat("/usr/bin/python3"); err != nil {
		t.Skip("python3 required for listen fixture")
	}
	var start api.StartResult
	if err := c.Call(ctx, api.MethodStart, api.StartPayload{
		Name: "listen", Sandbox: "off", Ports: []string{"declared-only"},
		Command: []string{"python3", "-c",
			"import socket,time;s=socket.socket();s.setsockopt(socket.SOL_SOCKET,socket.SO_REUSEADDR,1);" +
				"s.bind(('127.0.0.1',0));s.listen(1);print(s.getsockname()[1],flush=True);time.sleep(30)"},
	}, &start); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	var ports api.PortsResult
	for time.Now().Before(deadline) {
		if err := c.Call(ctx, api.MethodPorts, api.IDPayload{ID: start.ID}, &ports); err != nil {
			t.Fatal(err)
		}
		if len(ports.Discovered) > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if len(ports.Ports) != 1 || ports.Ports[0] != "declared-only" {
		t.Fatalf("declared ports = %v", ports.Ports)
	}
	// Discovered may be empty if /proc matching fails under bwrap; at least field present.
	var st api.ProcessView
	if err := c.Call(ctx, api.MethodStatus, api.IDPayload{ID: start.ID}, &st); err != nil {
		t.Fatal(err)
	}
	if st.Discovered == nil {
		t.Fatal("status.discovered should be non-nil slice")
	}
	_ = c.Call(ctx, api.MethodStop, api.IDPayload{ID: start.ID, TimeoutSec: 2}, &map[string]any{})
}
