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
	"crypto/rand"
	"errors"
	"testing"
	"time"

	"github.com/scrothers/pmmcp/internal/api"
	"github.com/scrothers/pmmcp/internal/daemon"
	"github.com/scrothers/pmmcp/internal/domain"
	"github.com/scrothers/pmmcp/internal/store/sqlite"
)

// doProfileList's s.profiles.List ctx-cancellation error branch is covered
// whitebox in handlers_a_internal_test.go: a real gRPC round trip can't
// reliably deliver an already-cancelled context to the handler (grpc-go
// short-circuits a known-past deadline client-side before any network call).

// --- doProfileUse / doSessionInfo / doSessionEnd: session.Registry.Open's
// id.New failure, forced via a failing crypto/rand.Reader (not parallel-safe:
// mutates the package-level rand.Reader). ---

// failingRandReader always fails Read, used to force crypto/rand-backed ID
// generation (ulid.New, via internal/id.New) to return an error
// deterministically without touching production code.
type failingRandReader struct{}

func (failingRandReader) Read([]byte) (int, error) { return 0, errors.New("failingRandReader: boom") }

func withFailingRand(t *testing.T) {
	t.Helper()
	orig := rand.Reader
	rand.Reader = failingRandReader{}
	t.Cleanup(func() { rand.Reader = orig })
}

func TestDoProfileUseSessionOpenFailure(t *testing.T) {
	ctx, _, c, _ := startTestDaemon(t)
	withFailingRand(t)
	var out map[string]string
	err := c.Call(ctx, api.MethodProfileUse, api.ProfilePayload{Name: "default"}, &out)
	if err == nil {
		t.Fatal("profile.use with a failing session id generator: want error, got nil")
	}
}

func TestDoSessionInfoEnsureSessionFailure(t *testing.T) {
	ctx, _, c, _ := startTestDaemon(t)
	withFailingRand(t)
	var out api.SessionInfoResult
	err := c.Call(ctx, api.MethodSessionInfo, nil, &out)
	if err == nil {
		t.Fatal("session.info with a failing session id generator: want error, got nil")
	}
}

func TestDoSessionEndEnsureSessionFailure(t *testing.T) {
	ctx, _, c, _ := startTestDaemon(t)
	withFailingRand(t)
	var out map[string]any
	// No session on the wire and an id that was never opened: session.End
	// fails, falling into ensureSession's Open path, which fails under the
	// broken rand.Reader.
	err := c.Call(ctx, api.MethodSessionEnd, map[string]string{"id": "does-not-exist"}, &out)
	if err == nil {
		t.Fatal("session.end with an unresolvable id and a failing session id generator: want error, got nil")
	}
	var derr *domain.Error
	if !errors.As(err, &derr) || derr.Code != domain.CodeNotFound {
		t.Fatalf("err = %v, want not_found (doSessionEnd maps ensureSession failure to session-not-found)", err)
	}
}

// --- doWait / doHealthCheck: the process.Manager and the store's process
// record diverging (boot-relaunch reconcile scenario: a record survives in
// the store but the manager no longer tracks its id). ---

func TestDoWaitManagerForgotProcess(t *testing.T) {
	t.Parallel()
	c, mgr := newTestDaemon(t, nil)
	ctx := context.Background()
	var start api.StartResult
	if err := c.Call(ctx, api.MethodStart, api.StartPayload{
		Name: "wait-forgotten", Command: []string{"sleep", "5"}, Sandbox: "off",
	}, &start); err != nil {
		t.Fatalf("start: %v", err)
	}
	mgr.mu.Lock()
	delete(mgr.handles, start.ID)
	mgr.mu.Unlock()

	var out api.WaitResult
	if err := c.Call(ctx, api.MethodWait, api.IDPayload{ID: start.ID}, &out); err == nil {
		t.Fatal("process.wait after the manager forgot the process: want error, got nil")
	}
}

func TestDoWaitEmptyManagerStatusDefaultsToExited(t *testing.T) {
	t.Parallel()
	c, mgr := newTestDaemon(t, nil)
	ctx := context.Background()
	var start api.StartResult
	if err := c.Call(ctx, api.MethodStart, api.StartPayload{
		Name: "wait-empty-status", Command: []string{"sleep", "5"}, Sandbox: "off",
	}, &start); err != nil {
		t.Fatalf("start: %v", err)
	}
	mgr.mu.Lock()
	mgr.handles[start.ID].Status = ""
	mgr.mu.Unlock()

	var out api.WaitResult
	if err := c.Call(ctx, api.MethodWait, api.IDPayload{ID: start.ID}, &out); err != nil {
		t.Fatalf("process.wait: %v", err)
	}
	if out.Status != string(domain.StatusExited) {
		t.Fatalf("Status = %q, want %q", out.Status, domain.StatusExited)
	}
}

func TestDoHealthCheckManagerForgotProcess(t *testing.T) {
	t.Parallel()
	c, mgr := newTestDaemon(t, nil)
	ctx := context.Background()
	var start api.StartResult
	if err := c.Call(ctx, api.MethodStart, api.StartPayload{
		Name: "health-forgotten", Command: []string{"sleep", "5"}, Sandbox: "off",
	}, &start); err != nil {
		t.Fatalf("start: %v", err)
	}
	mgr.mu.Lock()
	delete(mgr.handles, start.ID)
	mgr.mu.Unlock()

	var out api.HealthCheckResult
	if err := c.Call(ctx, api.MethodHealthCheck, api.IDPayload{ID: start.ID}, &out); err != nil {
		t.Fatalf("health_check: %v", err)
	}
	if out.OK || out.Message != "process not found in manager" {
		t.Fatalf("out = %+v, want not-ok / %q", out, "process not found in manager")
	}
}

// --- doUpdate / doEnable: store.Update failure via the Options.Store seam. ---

// failingUpdateStore wraps a real store.ProcessStore, forcing Update to fail
// while delegating every other method, to exercise doUpdate/doEnable's
// store.Update error-handling branches without a real database fault.
type failingUpdateStore struct {
	*sqlite.Store
	err error
}

func (f *failingUpdateStore) Update(context.Context, *domain.Process) error { return f.err }

func newFailingUpdateStore(t *testing.T) *failingUpdateStore {
	t.Helper()
	dir := t.TempDir()
	backing, err := sqlite.Open(dir + "/failstore.db")
	if err != nil {
		t.Fatalf("open backing store: %v", err)
	}
	if err := backing.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate backing store: %v", err)
	}
	t.Cleanup(func() { _ = backing.Close() })
	return &failingUpdateStore{Store: backing, err: errors.New("forced store.Update failure")}
}

func TestDoUpdateStoreUpdateFails(t *testing.T) {
	t.Parallel()
	fs := newFailingUpdateStore(t)
	c, _ := newTestDaemonOpts(t, nil, func(o *daemon.Options) { o.Store = fs })
	ctx := context.Background()
	var start api.StartResult
	if err := c.Call(ctx, api.MethodStart, api.StartPayload{
		Name: "upd-fail", Command: []string{"sleep", "5"}, Sandbox: "off",
	}, &start); err != nil {
		t.Fatalf("start: %v", err)
	}
	var out map[string]any
	err := c.Call(ctx, api.MethodUpdate, api.UpdatePayload{ID: start.ID, Cwd: "/tmp"}, &out)
	if err == nil {
		t.Fatal("process.update with a failing store: want error, got nil")
	}
}

func TestDoEnableDisableStoreUpdateFails(t *testing.T) {
	t.Parallel()
	fs := newFailingUpdateStore(t)
	c, _ := newTestDaemonOpts(t, nil, func(o *daemon.Options) { o.Store = fs })
	ctx := context.Background()
	var start api.StartResult
	if err := c.Call(ctx, api.MethodStart, api.StartPayload{
		Name: "enable-fail", Command: []string{"sleep", "5"}, Sandbox: "off",
	}, &start); err != nil {
		t.Fatalf("start: %v", err)
	}
	var enableOut map[string]any
	if err := c.Call(ctx, api.MethodEnable, api.IDPayload{ID: start.ID}, &enableOut); err == nil {
		t.Fatal("process.enable with a failing store: want error, got nil")
	}
	var disableOut map[string]any
	if err := c.Call(ctx, api.MethodDisable, api.IDPayload{ID: start.ID}, &disableOut); err == nil {
		t.Fatal("process.disable with a failing store: want error, got nil")
	}
}

// --- stopSODForSession: mgr.Stop failing skips bookkeeping for that id
// ("id in store but the manager rejects the stop" — a real reconcile edge,
// not a floor). ---

type stopErrMgr struct{ *fakeMgr }

func (m *stopErrMgr) Stop(context.Context, string, time.Duration) error {
	return errors.New("stop failed")
}

func TestStopSODForSessionStopErrorSkipsBookkeeping(t *testing.T) {
	t.Parallel()
	sm := &stopErrMgr{fakeMgr: newFakeMgr()}
	c, _ := newTestDaemonOpts(t, nil, func(o *daemon.Options) { o.Manager = sm })
	ctx := context.Background()
	c.SetSession("sess-sod-errmgr", "full")

	var start api.StartResult
	if err := c.Call(ctx, api.MethodStart, api.StartPayload{
		Name: "sod-errmgr", Command: []string{"sleep", "5"}, Sandbox: "off", StopOnDisconnect: true,
	}, &start); err != nil {
		t.Fatalf("start: %v", err)
	}

	var out map[string]any
	if err := c.Call(ctx, api.MethodSessionEnd, map[string]string{}, &out); err != nil {
		t.Fatalf("session.end: %v", err)
	}
	if n, _ := out["stopped"].(float64); n != 0 {
		t.Fatalf("stopped = %v, want 0 (mgr.Stop always errors, so bookkeeping is skipped)", out["stopped"])
	}
}
