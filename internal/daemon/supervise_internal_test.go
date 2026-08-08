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

package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/scrothers/pmmcp/internal/api"
	"github.com/scrothers/pmmcp/internal/config"
	"github.com/scrothers/pmmcp/internal/domain"
	"github.com/scrothers/pmmcp/internal/event"
	"github.com/scrothers/pmmcp/internal/store"
	"github.com/scrothers/pmmcp/internal/testsock"
	"github.com/scrothers/pmmcp/internal/webhook"
)

// stopAllSuperviseForTest force-stops every process this white-box server
// still has recorded and waits (via the short-timeout force path) for each
// to actually exit. Server.Close cancels the run context but never stops
// managed children or waits for background loops before returning, so an
// orphaned "sleep"-style process can still be alive — with its log file
// still open — when t.TempDir's cleanup runs right after. POSIX allows
// unlinking a file another process still has open; Windows does not, so
// there this turns into a "used by another process" cleanup failure.
func stopAllSuperviseForTest(ctx context.Context, t *testing.T, s *Server) {
	t.Helper()
	list, err := s.store.List(ctx, store.ProcessFilter{})
	if err != nil {
		return
	}
	for _, rec := range list {
		_ = s.mgr.Stop(ctx, rec.ID, time.Millisecond)
	}
}

// newSuperviseTestServer builds a real *Server (real local process manager, real
// SQLite-backed store/audit/events) for whitebox tests that need direct access
// to unexported methods (restartByID, probeRunningHealthy, startWatchForProcess,
// dispatchEventToHooks, deliverWithRetry) that the black-box daemon_test suite
// cannot reach through the IPC surface alone.
func newSuperviseTestServer(t *testing.T, tweak func(*config.Config)) (*Server, context.Context) {
	t.Helper()
	dir := t.TempDir()
	cfg, err := config.Load(config.LoadOptions{
		GOOS: "linux", Home: dir,
		LookupEnv: func(string) (string, bool) { return "", false },
	})
	if err != nil {
		t.Fatal(err)
	}
	cfg.StateDir = filepath.Join(dir, "state")
	cfg.IPC.Endpoint = testsock.Path(t)
	cfg.Sandbox.Default = "off"
	cfg.Relaunch.Enabled = false
	if tweak != nil {
		tweak(cfg)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	srv, err := New(ctx, Options{Config: cfg, DBPath: filepath.Join(dir, "db.sqlite")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Close() })
	t.Cleanup(func() { stopAllSuperviseForTest(ctx, t, srv) })
	return srv, ctx
}

// newSuperviseTestServerOpts is newSuperviseTestServer plus an Options mutator,
// for injecting the runAutoRestartLoop test seam (AutoRestartMax/Backoff) so a
// test can reach ShouldRestart's Max-exceeded branch in milliseconds instead of
// the production ~105s (20 restarts * 500ms backoff at the default policy).
func newSuperviseTestServerOpts(t *testing.T, tweak func(*config.Config), optsTweak func(*Options)) (*Server, context.Context) {
	t.Helper()
	dir := t.TempDir()
	cfg, err := config.Load(config.LoadOptions{
		GOOS: "linux", Home: dir,
		LookupEnv: func(string) (string, bool) { return "", false },
	})
	if err != nil {
		t.Fatal(err)
	}
	cfg.StateDir = filepath.Join(dir, "state")
	cfg.IPC.Endpoint = testsock.Path(t)
	cfg.Sandbox.Default = "off"
	cfg.Relaunch.Enabled = false
	if tweak != nil {
		tweak(cfg)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	opts := Options{Config: cfg, DBPath: filepath.Join(dir, "db.sqlite")}
	if optsTweak != nil {
		optsTweak(&opts)
	}
	srv, err := New(ctx, opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Close() })
	t.Cleanup(func() { stopAllSuperviseForTest(ctx, t, srv) })
	return srv, ctx
}

func startSuperviseProcess(ctx context.Context, t *testing.T, s *Server, payload api.StartPayload) api.StartResult {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	resp := s.doStart(ctx, s.principal("full", "sess-internal"), raw)
	if !resp.OK {
		t.Fatalf("doStart: %s: %s", resp.ErrorCode, resp.Error)
	}
	var out api.StartResult
	if err := json.Unmarshal(resp.Payload, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestProbeRunningHealthyProcessNotFound(t *testing.T) {
	t.Parallel()
	s, ctx := newSuperviseTestServer(t, nil)
	running, healthy := s.probeRunningHealthy(ctx, "does-not-exist", "")
	if running || healthy {
		t.Fatalf("probeRunningHealthy(unknown) = (%v,%v), want (false,false)", running, healthy)
	}
}

func TestProbeRunningHealthyTerminalStatus(t *testing.T) {
	t.Parallel()
	s, ctx := newSuperviseTestServer(t, nil)
	res := startSuperviseProcess(ctx, t, s, api.StartPayload{Name: "quick", Command: []string{"true"}, Sandbox: "off"})
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		h, err := s.mgr.Inspect(ctx, res.ID)
		if err == nil && h.Status != domain.StatusRunning && h.Status != domain.StatusStarting {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	running, healthy := s.probeRunningHealthy(ctx, res.ID, "")
	if running || healthy {
		t.Fatalf("probeRunningHealthy(exited) = (%v,%v), want (false,false)", running, healthy)
	}
}

func TestProbeRunningHealthyNoHealthURL(t *testing.T) {
	t.Parallel()
	s, ctx := newSuperviseTestServer(t, nil)
	res := startSuperviseProcess(ctx, t, s, api.StartPayload{Name: "svc", Command: []string{"sleep", "5"}, Sandbox: "off"})
	running, healthy := s.probeRunningHealthy(ctx, res.ID, "")
	if !running || !healthy {
		t.Fatalf("probeRunningHealthy(running, no url) = (%v,%v), want (true,true)", running, healthy)
	}
}

func TestProbeRunningHealthyHTTPSuccess(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	s, ctx := newSuperviseTestServer(t, nil)
	res := startSuperviseProcess(ctx, t, s, api.StartPayload{Name: "svc", Command: []string{"sleep", "5"}, Sandbox: "off"})
	running, healthy := s.probeRunningHealthy(ctx, res.ID, srv.URL)
	if !running || !healthy {
		t.Fatalf("probeRunningHealthy(healthy url) = (%v,%v), want (true,true)", running, healthy)
	}
}

func TestProbeRunningHealthyHTTPFailureMarksUnhealthy(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	s, ctx := newSuperviseTestServer(t, nil)
	res := startSuperviseProcess(ctx, t, s, api.StartPayload{Name: "svc", Command: []string{"sleep", "5"}, Sandbox: "off"})
	running, healthy := s.probeRunningHealthy(ctx, res.ID, srv.URL)
	if !running || healthy {
		t.Fatalf("probeRunningHealthy(failing url) = (%v,%v), want (true,false)", running, healthy)
	}
	rec, err := s.store.Get(ctx, res.ID)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Status != domain.StatusUnhealthy {
		t.Fatalf("store status = %q, want unhealthy", rec.Status)
	}
}

func TestRestartByIDSuccess(t *testing.T) {
	t.Parallel()
	s, ctx := newSuperviseTestServer(t, nil)
	res := startSuperviseProcess(ctx, t, s, api.StartPayload{Name: "svc", Command: []string{"sleep", "5"}, Sandbox: "off"})

	if err := s.restartByID(ctx, res.ID); err != nil {
		t.Fatalf("restartByID: %v", err)
	}
	rec, err := s.store.Get(ctx, res.ID)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Status != domain.StatusRunning || rec.Desired != domain.DesiredRunning {
		t.Fatalf("record after restart = %+v", rec)
	}
	if rec.StartedAt == nil {
		t.Fatal("StartedAt not set after restart")
	}
}

func TestRestartByIDStoreGetError(t *testing.T) {
	t.Parallel()
	s, ctx := newSuperviseTestServer(t, nil)
	if err := s.restartByID(ctx, "no-such-id"); err == nil {
		t.Fatal("restartByID on unknown id: want error, got nil")
	}
}

func TestRestartByIDStartError(t *testing.T) {
	t.Parallel()
	s, ctx := newSuperviseTestServer(t, nil)
	res := startSuperviseProcess(ctx, t, s, api.StartPayload{Name: "svc", Command: []string{"sleep", "5"}, Sandbox: "off"})

	rec, err := s.store.Get(ctx, res.ID)
	if err != nil {
		t.Fatal(err)
	}
	rec.Command = []string{"/no/such/binary-xyz"}
	if err := s.store.Update(ctx, rec); err != nil {
		t.Fatal(err)
	}
	if err := s.restartByID(ctx, res.ID); err == nil {
		t.Fatal("restartByID with an unstartable command: want error, got nil")
	}
}

func TestRunAutoRestartLoopRestartsExitedProcess(t *testing.T) {
	t.Parallel()
	s, ctx := newSuperviseTestServer(t, nil)
	s.autoRestartTick = 20 * time.Millisecond
	s.autoRestartBackoff = time.Millisecond
	res := startSuperviseProcess(ctx, t, s, api.StartPayload{
		// "sh" resolves via PATH on every CI platform (Git for Windows ships
		// sh.exe); a hardcoded /bin/sh path doesn't exist on Windows.
		Name: "flappy", Command: []string{"sh", "-c", "exit 1"}, Sandbox: "off", AutoRestart: true,
	})

	loopCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		s.runAutoRestartLoop(loopCtx)
		close(done)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		evs := s.events.Query(ctx, res.ID, 10)
		for _, e := range evs {
			if e.Type == "process.auto_restarted" {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("expected a process.auto_restarted event within the deadline")
}

func TestRunAutoRestartLoopBuildsHealthURLSnapshot(t *testing.T) {
	t.Parallel()
	// A process that is both auto-restart AND health-checked exercises the
	// loop's per-tick health-URL map snapshot (s.healthURL is non-empty),
	// not just the auto-restart-id snapshot.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	s, ctx := newSuperviseTestServer(t, nil)
	s.autoRestartTick = 20 * time.Millisecond
	res := startSuperviseProcess(ctx, t, s, api.StartPayload{
		Name: "healthy", Command: []string{"sleep", "5"}, Sandbox: "off",
		AutoRestart: true, HealthURL: srv.URL,
	})

	loopCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		s.runAutoRestartLoop(loopCtx)
		close(done)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})

	// The process stays running and healthy throughout: give the loop a few
	// ticks (20ms injected interval) to snapshot s.healthURL and probe it,
	// then confirm no restart happened (a healthy process must not be
	// restarted).
	time.Sleep(200 * time.Millisecond)
	evs := s.events.Query(ctx, res.ID, 10)
	for _, e := range evs {
		if e.Type == "process.auto_restarted" {
			t.Fatalf("a healthy process was restarted: %+v", e)
		}
	}
	rec, err := s.store.Get(ctx, res.ID)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Status != domain.StatusRunning {
		t.Fatalf("status = %q, want running", rec.Status)
	}
}

func TestRunAutoRestartLoopStopsRestartingAtMax(t *testing.T) {
	t.Parallel()
	// Relies on wall-clock ticks of the loop's (injected, 50ms) ticker to
	// observe two consecutive unhealthy ticks — an always-failing process
	// restarts once (count 0 < Max 1), then the second failure must hit the
	// ShouldRestart()-false branch (count 1 < Max 1 is false) and NOT
	// restart again.
	s, ctx := newSuperviseTestServerOpts(t, nil, func(o *Options) {
		o.AutoRestartMax = 1
		o.AutoRestartBackoff = time.Millisecond
		o.AutoRestartTick = 50 * time.Millisecond
	})
	res := startSuperviseProcess(ctx, t, s, api.StartPayload{
		Name: "always-fails", Command: []string{"sh", "-c", "exit 1"}, Sandbox: "off", AutoRestart: true,
	})

	loopCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		s.runAutoRestartLoop(loopCtx)
		close(done)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})

	// Two ticks (50ms each) covers: tick 1 restarts (count 0->1), tick 2
	// hits the Max-exceeded branch and stops. Sleep several extra ticks of
	// margin so the loop provably had chances to over-restart and didn't.
	time.Sleep(400 * time.Millisecond)

	restarts := 0
	for _, e := range s.events.Query(ctx, res.ID, 10) {
		if e.Type == "process.auto_restarted" {
			restarts++
		}
	}
	if restarts != 1 {
		t.Fatalf("process.auto_restarted count = %d, want exactly 1 (Max=1 must stop further restarts)", restarts)
	}
}

func TestRunAutoRestartLoopStopsOnContextDone(t *testing.T) {
	t.Parallel()
	s, ctx := newSuperviseTestServer(t, nil)
	loopCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		s.runAutoRestartLoop(loopCtx)
		close(done)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("runAutoRestartLoop did not return after ctx cancel")
	}
}

func TestStartWatchForProcessRestartsOnChange(t *testing.T) {
	t.Parallel()
	s, ctx := newSuperviseTestServer(t, nil)
	res := startSuperviseProcess(ctx, t, s, api.StartPayload{Name: "watched", Command: []string{"sleep", "5"}, Sandbox: "off"})

	watchDir := t.TempDir()
	watchFile := filepath.Join(watchDir, "trigger")
	if err := os.WriteFile(watchFile, []byte("v1"), 0o600); err != nil {
		t.Fatal(err)
	}

	watchCtx, cancel := context.WithCancel(ctx)
	t.Cleanup(cancel)
	if err := s.startWatchForProcess(watchCtx, res.ID, watchFile); err != nil {
		t.Fatalf("startWatchForProcess: %v", err)
	}

	if err := os.WriteFile(watchFile, []byte("v2"), 0o600); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		evs := s.events.Query(ctx, res.ID, 10)
		for _, e := range evs {
			if e.Type == "process.watch_restart" {
				return
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("expected a process.watch_restart event within the deadline")
}

func TestStartWatchForProcessRestartErrorEmitsEvent(t *testing.T) {
	t.Parallel()
	s, ctx := newSuperviseTestServer(t, nil)
	res := startSuperviseProcess(ctx, t, s, api.StartPayload{Name: "watched", Command: []string{"sleep", "5"}, Sandbox: "off"})

	// Corrupt the stored command so the watch-triggered restartByID fails.
	rec, err := s.store.Get(ctx, res.ID)
	if err != nil {
		t.Fatal(err)
	}
	rec.Command = []string{"/no/such/binary-xyz"}
	if err := s.store.Update(ctx, rec); err != nil {
		t.Fatal(err)
	}

	watchDir := t.TempDir()
	watchFile := filepath.Join(watchDir, "trigger")
	if err := os.WriteFile(watchFile, []byte("v1"), 0o600); err != nil {
		t.Fatal(err)
	}
	watchCtx, cancel := context.WithCancel(ctx)
	t.Cleanup(cancel)
	if err := s.startWatchForProcess(watchCtx, res.ID, watchFile); err != nil {
		t.Fatalf("startWatchForProcess: %v", err)
	}
	if err := os.WriteFile(watchFile, []byte("v2"), 0o600); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		evs := s.events.Query(ctx, res.ID, 10)
		for _, e := range evs {
			if e.Type == "process.watch_restart_error" {
				return
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("expected a process.watch_restart_error event within the deadline")
}

func TestStartWatchForProcessReplacesExistingWatcher(t *testing.T) {
	t.Parallel()
	s, ctx := newSuperviseTestServer(t, nil)
	res := startSuperviseProcess(ctx, t, s, api.StartPayload{Name: "watched", Command: []string{"sleep", "5"}, Sandbox: "off"})

	watchDir := t.TempDir()
	f1 := filepath.Join(watchDir, "one")
	f2 := filepath.Join(watchDir, "two")
	if err := os.WriteFile(f1, []byte("v1"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f2, []byte("v1"), 0o600); err != nil {
		t.Fatal(err)
	}

	watchCtx, cancel := context.WithCancel(ctx)
	t.Cleanup(cancel)
	if err := s.startWatchForProcess(watchCtx, res.ID, f1); err != nil {
		t.Fatalf("first startWatchForProcess: %v", err)
	}
	// Replacing the watcher for the same process id must close the old one
	// rather than leaking it (the old-watcher-close branch).
	if err := s.startWatchForProcess(watchCtx, res.ID, f2); err != nil {
		t.Fatalf("second startWatchForProcess: %v", err)
	}
	s.mu.Lock()
	got := s.watches[res.ID]
	s.mu.Unlock()
	if got != f2 {
		t.Fatalf("watches[id] = %q, want %q", got, f2)
	}
}

func TestStartWatchForProcessAddError(t *testing.T) {
	t.Parallel()
	s, ctx := newSuperviseTestServer(t, nil)
	res := startSuperviseProcess(ctx, t, s, api.StartPayload{Name: "watched", Command: []string{"sleep", "5"}, Sandbox: "off"})
	if err := s.startWatchForProcess(ctx, res.ID, filepath.Join(t.TempDir(), "does-not-exist")); err == nil {
		t.Fatal("startWatchForProcess on a nonexistent path: want error, got nil")
	}
}

func TestDefaultWebhookDeliver(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	// defaultWebhookDeliver is a thin pass-through to webhook.Deliverer; any
	// destination exercises the statement. A loopback URL is SSRF-blocked by
	// the production deliverer, which is the expected (and only reachable)
	// outcome in this sandbox — the point is executing the call, not a 2xx.
	err := defaultWebhookDeliver(ctx, webhook.Hook{ID: "h1", URL: "http://127.0.0.1:1/hook"}, webhook.Event{Type: "process.started"})
	if err == nil {
		t.Fatal("defaultWebhookDeliver to loopback: want an SSRF-blocked error, got nil")
	}
}

func TestDispatchEventToHooksMatchesFilter(t *testing.T) {
	t.Parallel()
	s, ctx := newSuperviseTestServer(t, nil)
	var delivered []string
	s.deliver = func(_ context.Context, h webhook.Hook, _ webhook.Event) error {
		delivered = append(delivered, h.ID)
		return nil
	}
	hooks := []webhook.Hook{
		{ID: "matches", Events: []string{"process.started"}},
		{ID: "no-match", Events: []string{"process.stopped"}},
		{ID: "wildcard"},
	}
	s.dispatchEventToHooks(ctx, hooks, event.Event{Type: "process.started", ProcessID: "p1"})
	if len(delivered) != 2 {
		t.Fatalf("delivered = %v, want exactly the matching + wildcard hooks", delivered)
	}
	for _, id := range delivered {
		if id == "no-match" {
			t.Fatalf("delivered to a non-matching hook: %v", delivered)
		}
	}
}

func TestDeliverWithRetrySucceedsFirstTry(t *testing.T) {
	t.Parallel()
	s, ctx := newSuperviseTestServer(t, nil)
	calls := 0
	s.deliver = func(context.Context, webhook.Hook, webhook.Event) error {
		calls++
		return nil
	}
	s.deliverWithRetry(ctx, webhook.Hook{ID: "h"}, webhook.Event{})
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
}

func TestDeliverWithRetrySucceedsAfterFailures(t *testing.T) {
	t.Parallel()
	s, ctx := newSuperviseTestServer(t, nil)
	const backoff = 20 * time.Millisecond
	s.webhookRetryBackoff = backoff
	calls := 0
	s.deliver = func(context.Context, webhook.Hook, webhook.Event) error {
		calls++
		if calls < 2 {
			return errors.New("transient")
		}
		return nil
	}
	start := time.Now()
	s.deliverWithRetry(ctx, webhook.Hook{ID: "h"}, webhook.Event{})
	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
	if time.Since(start) < backoff {
		t.Fatalf("deliverWithRetry returned too fast for a real backoff wait: %v", time.Since(start))
	}
}

func TestDeliverWithRetryGivesUpAfterMaxAttempts(t *testing.T) {
	t.Parallel()
	s, ctx := newSuperviseTestServer(t, nil)
	s.webhookRetryBackoff = time.Millisecond
	calls := 0
	s.deliver = func(context.Context, webhook.Hook, webhook.Event) error {
		calls++
		return errors.New("permanent")
	}
	s.deliverWithRetry(ctx, webhook.Hook{ID: "h"}, webhook.Event{})
	if calls != 3 {
		t.Fatalf("calls = %d, want 3 (maxAttempts)", calls)
	}
}

func TestDeliverWithRetryStopsWhenContextAlreadyDone(t *testing.T) {
	t.Parallel()
	s, _ := newSuperviseTestServer(t, nil)
	cctx, cancel := context.WithCancel(context.Background())
	calls := 0
	s.deliver = func(context.Context, webhook.Hook, webhook.Event) error {
		calls++
		cancel() // simulate the context ending during/around this attempt
		return errors.New("fails")
	}
	s.deliverWithRetry(cctx, webhook.Hook{ID: "h"}, webhook.Event{})
	if calls != 1 {
		t.Fatalf("calls = %d, want 1 (should stop once ctx is done)", calls)
	}
}

func TestDeliverWithRetryStopsDuringBackoffWait(t *testing.T) {
	t.Parallel()
	s, _ := newSuperviseTestServer(t, nil)
	cctx, cancel := context.WithCancel(context.Background())
	calls := 0
	s.deliver = func(context.Context, webhook.Hook, webhook.Event) error {
		calls++
		return errors.New("fails")
	}
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	start := time.Now()
	s.deliverWithRetry(cctx, webhook.Hook{ID: "h"}, webhook.Event{})
	if elapsed := time.Since(start); elapsed > 1*time.Second {
		t.Fatalf("deliverWithRetry took %v, want early return once ctx canceled mid-backoff", elapsed)
	}
	if calls < 1 {
		t.Fatalf("calls = %d, want at least 1", calls)
	}
}

func TestLatestEventSeqEmpty(t *testing.T) {
	t.Parallel()
	s, ctx := newSuperviseTestServer(t, nil)
	if got := s.latestEventSeq(ctx); got != 0 {
		t.Fatalf("latestEventSeq(empty) = %d, want 0", got)
	}
}

func TestLatestEventSeqAfterAppend(t *testing.T) {
	t.Parallel()
	s, ctx := newSuperviseTestServer(t, nil)
	ev, err := s.events.Append(ctx, event.Event{Type: "process.started", ProcessID: "p1"})
	if err != nil {
		t.Fatal(err)
	}
	if got := s.latestEventSeq(ctx); got != ev.Seq {
		t.Fatalf("latestEventSeq = %d, want %d", got, ev.Seq)
	}
}

func TestRunWebhookDispatchIdleAdvancesCursorWithoutHooks(t *testing.T) {
	t.Parallel()
	s, ctx := newSuperviseTestServer(t, nil)
	s.webhookPoll = 20 * time.Millisecond
	delivered := 0
	s.deliver = func(context.Context, webhook.Hook, webhook.Event) error {
		delivered++
		return nil
	}
	if _, err := s.events.Append(ctx, event.Event{Type: "process.started", ProcessID: "p1"}); err != nil {
		t.Fatal(err)
	}

	loopCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		s.runWebhookDispatch(loopCtx)
		close(done)
	}()
	time.Sleep(150 * time.Millisecond)
	cancel()
	<-done
	if delivered != 0 {
		t.Fatalf("delivered = %d with zero registered hooks, want 0", delivered)
	}
}

func TestRunWebhookDispatchDeliversNewEvents(t *testing.T) {
	t.Parallel()
	s, ctx := newSuperviseTestServer(t, func(cfg *config.Config) {
		cfg.Webhook.Allowlist = []string{"*.example.com"}
	})
	s.webhookPoll = 20 * time.Millisecond
	delivered := make(chan webhook.Hook, 4)
	s.deliver = func(_ context.Context, h webhook.Hook, _ webhook.Event) error {
		delivered <- h
		return nil
	}
	if err := s.hooks.Create(webhook.Hook{ID: "h1", URL: "https://hooks.example.com/x"}); err != nil {
		t.Fatal(err)
	}

	loopCtx, cancel := context.WithCancel(ctx)
	t.Cleanup(cancel)
	go s.runWebhookDispatch(loopCtx)
	// Let the loop seed its cursor to "no events yet" before we append one,
	// so this event is guaranteed to land after the cursor rather than racing it.
	time.Sleep(100 * time.Millisecond)

	if _, err := s.events.Append(ctx, event.Event{Type: "process.started", ProcessID: "p1"}); err != nil {
		t.Fatal(err)
	}
	select {
	case h := <-delivered:
		if h.ID != "h1" {
			t.Fatalf("delivered to %q, want h1", h.ID)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("event did not reach the registered hook within the deadline")
	}
}

func TestPidAliveNonPositive(t *testing.T) {
	t.Parallel()
	if pidAlive(0) {
		t.Fatal("pidAlive(0) = true, want false")
	}
	if pidAlive(-1) {
		t.Fatal("pidAlive(-1) = true, want false")
	}
}

func TestPidAliveLiveProcess(t *testing.T) {
	t.Parallel()
	if !pidAlive(os.Getpid()) {
		t.Fatal("pidAlive(self) = false, want true")
	}
}

func TestPidAliveDeadProcess(t *testing.T) {
	t.Parallel()
	// "true" is resolved via PATH: it lives at /bin/true on Linux but
	// /usr/bin/true on macOS, so a hardcoded path isn't portable.
	cmd := exec.Command("true")
	if err := cmd.Run(); err != nil {
		t.Fatalf("run true: %v", err)
	}
	pid := cmd.Process.Pid
	if pidAlive(pid) {
		t.Fatalf("pidAlive(%d) = true for an exited, reaped process, want false", pid)
	}
}
