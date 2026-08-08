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
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/scrothers/pmmcp/internal/api"
	"github.com/scrothers/pmmcp/internal/daemon"
	"github.com/scrothers/pmmcp/internal/domain"
	"github.com/scrothers/pmmcp/internal/ipc"
	"github.com/scrothers/pmmcp/internal/store"
	"github.com/scrothers/pmmcp/internal/store/sqlite"
)

// secondListFailsStore wraps a real store.ProcessStore, delegating every call
// except it forces the *second* List call to fail. resolveID's name-fallback
// path calls List twice (project-scoped, then global); this exercises the
// second call's distinct error-handling branch without touching the first.
type secondListFailsStore struct {
	*sqlite.Store
	calls int
}

func (f *secondListFailsStore) List(ctx context.Context, filter store.ProcessFilter) ([]*domain.Process, error) {
	f.calls++
	if f.calls >= 2 {
		return nil, errors.New("forced second List failure")
	}
	return f.Store.List(ctx, filter)
}

func newSecondListFailsStore(t *testing.T) *secondListFailsStore {
	t.Helper()
	dir := t.TempDir()
	second, err := sqlite.Open(filepath.Join(dir, "secondlist.db"))
	if err != nil {
		t.Fatalf("open backing store: %v", err)
	}
	if err := second.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate backing store: %v", err)
	}
	t.Cleanup(func() { _ = second.Close() })
	return &secondListFailsStore{Store: second}
}

func TestResolveIDSecondListCallFails(t *testing.T) {
	t.Parallel()
	fs := newSecondListFailsStore(t)
	c, _ := newTestDaemonOpts(t, nil, func(o *daemon.Options) { o.Store = fs })
	ctx := context.Background()
	// No id, a name that resolves to nothing: byName misses, the first
	// (project-scoped) List call genuinely finds nothing (real store, real
	// empty result), then the second (global) List call is the one our fake
	// forces to fail.
	var out api.ProcessView
	err := c.Call(ctx, api.MethodStatus, api.IDPayload{Name: "no-such-process", Project: "no-such-project"}, &out)
	if err == nil {
		t.Fatal("process.status resolving an unknown name under a forced second-List failure: want error, got nil")
	}
}

// TestListenAndServeGracefulStopFallsBackAfterTimeout proves the bounded
// gs.Stop() fallback: a long-running unary RPC (process.wait on a process
// that won't exit on its own) is not tied to the run context the way
// streaming RPCs are (that fix is stream-specific — see FABLE_REVIEW.md), so
// it keeps GracefulStop blocked past its budget (injected here as 300ms;
// 5s in production), and Stop() must force the drain rather than let
// ListenAndServe hang for the process's full lifetime.
func TestListenAndServeGracefulStopFallsBackAfterTimeout(t *testing.T) {
	t.Parallel()
	const budget = 300 * time.Millisecond
	dir := t.TempDir()
	cfg := newTestConfig(t, dir)
	ctx, cancel := context.WithCancel(context.Background())
	srv, err := daemon.New(ctx, daemon.Options{
		Config: cfg, DBPath: filepath.Join(dir, "db.sqlite"),
		GracefulStopTimeout: budget,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	serveErrCh := make(chan error, 1)
	go func() { serveErrCh <- srv.ListenAndServe(ctx) }()

	var c *ipc.Client
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		c, err = ipc.Dial(context.Background(), cfg.IPC.Endpoint)
		if err == nil {
			break
		}
		time.Sleep(15 * time.Millisecond)
	}
	if c == nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	var start api.StartResult
	if err := c.Call(context.Background(), api.MethodStart, api.StartPayload{
		Name: "long-wait", Command: []string{"sleep", "30"}, Sandbox: "off",
	}, &start); err != nil {
		t.Fatalf("start: %v", err)
	}

	waitDone := make(chan struct{})
	go func() {
		var out api.WaitResult
		_ = c.Call(context.Background(), api.MethodWait, api.IDPayload{ID: start.ID}, &out)
		close(waitDone)
	}()
	// Let the wait RPC actually reach the server and start blocking in
	// mgr.Wait before we trigger shutdown.
	time.Sleep(300 * time.Millisecond)

	shutdownStart := time.Now()
	cancel()
	select {
	case <-serveErrCh:
		elapsed := time.Since(shutdownStart)
		if elapsed < budget-budget/4 {
			t.Fatalf("ListenAndServe returned after %v, want ~%v (the in-flight wait RPC should have blocked GracefulStop)", elapsed, budget)
		}
		if elapsed > 10*budget {
			t.Fatalf("ListenAndServe returned after %v, want ~%v (the Stop() fallback should have forced the drain)", elapsed, budget)
		}
	case <-time.After(10 * budget):
		t.Fatalf("ListenAndServe did not return within %v of the %v Stop() fallback deadline", 10*budget, budget)
	}
	// gs.Stop() above only tore down the gRPC transport; "long-wait" (sleep
	// 30) is still running and, with the IPC connection now dead, unreachable
	// through the client. Kill it directly by PID — Server.Close (below)
	// doesn't stop managed children either, and an orphaned sleep still
	// holding stderr.log open races t.TempDir's cleanup on Windows.
	if p, err := os.FindProcess(start.PID); err == nil {
		_ = p.Kill()
	}
	<-waitDone
	_ = srv.Close()
}
