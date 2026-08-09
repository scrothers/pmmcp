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
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/scrothers/pmmcp/internal/api"
	pmmcpv1 "github.com/scrothers/pmmcp/internal/api/gen/pmmcp/v1"
	"github.com/scrothers/pmmcp/internal/config"
	"github.com/scrothers/pmmcp/internal/domain"
	"github.com/scrothers/pmmcp/internal/event"
	"google.golang.org/grpc/metadata"
)

// seedEvent builds a minimal event.Event for tests exercising SubscribeEvents.
func seedEvent(processID string) event.Event {
	return event.Event{Type: "process.started", ProcessID: processID, Message: "started"}
}

// newGRPCInternalTestServer builds a minimal *Server directly (white-box, package
// daemon) so grpcAdapter's stream handlers can be exercised without a real
// gRPC transport, and so error/edge branches (Send failures, ctx-cancel vs.
// run-context shutdown) can be triggered deterministically.
func newGRPCInternalTestServer(t *testing.T) *Server {
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
	srv, err := New(context.Background(), Options{Config: cfg, DBPath: sqliteDBPathForTest(t)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Close() })
	return srv
}

// newLogRecord inserts a process record backed by real log files on disk so
// logcap.Tail/readFollow have something to read, returning the process ID.
func newLogRecord(t *testing.T, srv *Server, stdout, stderr string) string {
	t.Helper()
	logDir := t.TempDir()
	if stdout != "" {
		if err := os.WriteFile(filepath.Join(logDir, "stdout.log"), []byte(stdout), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if stderr != "" {
		if err := os.WriteFile(filepath.Join(logDir, "stderr.log"), []byte(stderr), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	id := "proc-" + t.Name()
	rec := &domain.Process{
		ID: id, Name: t.Name(), Command: []string{"true"},
		Status: domain.StatusRunning, LogDir: logDir,
	}
	if err := srv.store.Create(context.Background(), rec); err != nil {
		t.Fatal(err)
	}
	return id
}

// fakeLogStream is a hand-written pmmcpv1.Daemon_SubscribeLogsServer (a
// grpc.ServerStreamingServer[LogChunk]) that lets tests control the stream
// context and inject Send failures deterministically.
type fakeLogStream struct {
	ctxFn    func() context.Context
	sendErrs []error // sendErrs[i] applies to the i'th Send call; nil/short means no error
	sends    []*pmmcpv1.LogChunk
}

func (f *fakeLogStream) Send(c *pmmcpv1.LogChunk) error {
	i := len(f.sends)
	f.sends = append(f.sends, c)
	if i < len(f.sendErrs) && f.sendErrs[i] != nil {
		return f.sendErrs[i]
	}
	return nil
}
func (f *fakeLogStream) Context() context.Context {
	if f.ctxFn != nil {
		return f.ctxFn()
	}
	return context.Background()
}
func (f *fakeLogStream) SetHeader(metadata.MD) error  { return nil }
func (f *fakeLogStream) SendHeader(metadata.MD) error { return nil }
func (f *fakeLogStream) SetTrailer(metadata.MD)       {}
func (f *fakeLogStream) SendMsg(any) error            { return nil }
func (f *fakeLogStream) RecvMsg(any) error            { return nil }

// fakeEventStream is the SubscribeEvents analogue of fakeLogStream.
type fakeEventStream struct {
	ctxFn    func() context.Context
	sendErrs []error
	sends    []*pmmcpv1.EventChunk
}

func (f *fakeEventStream) Send(c *pmmcpv1.EventChunk) error {
	i := len(f.sends)
	f.sends = append(f.sends, c)
	if i < len(f.sendErrs) && f.sendErrs[i] != nil {
		return f.sendErrs[i]
	}
	return nil
}
func (f *fakeEventStream) Context() context.Context {
	if f.ctxFn != nil {
		return f.ctxFn()
	}
	return context.Background()
}
func (f *fakeEventStream) SetHeader(metadata.MD) error  { return nil }
func (f *fakeEventStream) SendHeader(metadata.MD) error { return nil }
func (f *fakeEventStream) SetTrailer(metadata.MD)       {}
func (f *fakeEventStream) SendMsg(any) error            { return nil }
func (f *fakeEventStream) RecvMsg(any) error            { return nil }

func TestGRPCAdapterCallNilRequest(t *testing.T) {
	t.Parallel()
	g := &grpcAdapter{s: &Server{}}
	if _, err := g.Call(context.Background(), nil); err == nil {
		t.Fatal("Call(nil): want error, got nil")
	}
}

func TestGRPCSubscribeLogsVersionMismatch(t *testing.T) {
	t.Parallel()
	srv := newGRPCInternalTestServer(t)
	g := &grpcAdapter{s: srv}
	stream := &fakeLogStream{ctxFn: func() context.Context { return context.Background() }}
	err := g.SubscribeLogs(&pmmcpv1.SubscribeLogsRequest{ApiVersion: "99.0", Role: "full"}, stream)
	if err == nil {
		t.Fatal("SubscribeLogs with mismatched api_version: want error, got nil")
	}
}

func TestGRPCSubscribeLogsAuthzDenied(t *testing.T) {
	t.Parallel()
	srv := newGRPCInternalTestServer(t)
	g := &grpcAdapter{s: srv}
	stream := &fakeLogStream{ctxFn: func() context.Context { return context.Background() }}
	// "bogus" is not a recognized role, so authz.Caps returns the empty set —
	// deny-all, including logs:read (every *named* role in the matrix has it).
	err := g.SubscribeLogs(&pmmcpv1.SubscribeLogsRequest{ApiVersion: api.APIVersion, Role: "bogus"}, stream)
	if err == nil {
		t.Fatal("SubscribeLogs with an unrecognized role: want permission denied, got nil")
	}
}

func TestGRPCSubscribeLogsEmptyProcessID(t *testing.T) {
	t.Parallel()
	srv := newGRPCInternalTestServer(t)
	g := &grpcAdapter{s: srv}
	stream := &fakeLogStream{ctxFn: func() context.Context { return context.Background() }}
	err := g.SubscribeLogs(&pmmcpv1.SubscribeLogsRequest{ApiVersion: api.APIVersion, Role: "full"}, stream)
	if err == nil {
		t.Fatal("SubscribeLogs with empty process_id: want error, got nil")
	}
}

func TestGRPCSubscribeLogsUnknownProcessID(t *testing.T) {
	t.Parallel()
	srv := newGRPCInternalTestServer(t)
	g := &grpcAdapter{s: srv}
	stream := &fakeLogStream{ctxFn: func() context.Context { return context.Background() }}
	err := g.SubscribeLogs(&pmmcpv1.SubscribeLogsRequest{ApiVersion: api.APIVersion, Role: "full", ProcessId: "does-not-exist"}, stream)
	if err == nil {
		t.Fatal("SubscribeLogs on an unknown process: want error, got nil")
	}
}

func TestGRPCSubscribeLogsInitialDumpSendError(t *testing.T) {
	t.Parallel()
	srv := newGRPCInternalTestServer(t)
	pid := newLogRecord(t, srv, "hello from stdout\n", "")
	g := &grpcAdapter{s: srv}
	stream := &fakeLogStream{ctxFn: func() context.Context { return context.Background() }, sendErrs: []error{context.Canceled}}
	err := g.SubscribeLogs(&pmmcpv1.SubscribeLogsRequest{ApiVersion: api.APIVersion, Role: "full", ProcessId: pid, MaxDurationSec: 30}, stream)
	if err == nil {
		t.Fatal("SubscribeLogs with a failing initial-dump Send: want error, got nil")
	}
	if len(stream.sends) != 1 {
		t.Fatalf("sends = %d, want exactly 1 (the failed initial dump, no further attempts)", len(stream.sends))
	}
}

func TestGRPCSubscribeLogsStreamNameDefaultsAndDurationClamped(t *testing.T) {
	t.Parallel()
	srv := newGRPCInternalTestServer(t)
	// No initial content: skips the initial-dump Send entirely, isolating the
	// max<=0 default and the empty-Stream default-to-"both" assignment.
	pid := newLogRecord(t, srv, "", "")
	g := &grpcAdapter{s: srv}
	ctx, cancel := context.WithCancel(context.Background())
	stream := &fakeLogStream{ctxFn: func() context.Context { return ctx }}
	done := make(chan error, 1)
	go func() {
		// MaxDurationSec omitted (0) exercises the "max <= 0 -> 30s default" branch.
		done <- g.SubscribeLogs(&pmmcpv1.SubscribeLogsRequest{ApiVersion: api.APIVersion, Role: "full", ProcessId: pid}, stream)
	}()
	time.Sleep(150 * time.Millisecond) // let store.Get/setup finish before cancelling
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("SubscribeLogs with cancelled ctx: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("SubscribeLogs did not return after ctx cancel")
	}
	if len(stream.sends) != 1 || !stream.sends[0].GetEof() {
		t.Fatalf("sends = %+v, want exactly one EOF chunk", stream.sends)
	}
}

func TestGRPCSubscribeLogsMaxDurationCappedAtFiveMinutes(t *testing.T) {
	t.Parallel()
	srv := newGRPCInternalTestServer(t)
	pid := newLogRecord(t, srv, "", "")
	g := &grpcAdapter{s: srv}
	ctx, cancel := context.WithCancel(context.Background())
	stream := &fakeLogStream{ctxFn: func() context.Context { return ctx }}
	done := make(chan error, 1)
	go func() {
		// A duration far beyond 5 minutes exercises the clamp branch.
		done <- g.SubscribeLogs(&pmmcpv1.SubscribeLogsRequest{ApiVersion: api.APIVersion, Role: "full", ProcessId: pid, MaxDurationSec: 36000}, stream)
	}()
	time.Sleep(150 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("SubscribeLogs: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("SubscribeLogs did not return after ctx cancel")
	}
}

func TestGRPCSubscribeLogsCtxDoneBranch(t *testing.T) {
	t.Parallel()
	srv := newGRPCInternalTestServer(t)
	pid := newLogRecord(t, srv, "", "")
	g := &grpcAdapter{s: srv}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	stream := &fakeLogStream{ctxFn: func() context.Context { return ctx }}
	go func() {
		done <- g.SubscribeLogs(&pmmcpv1.SubscribeLogsRequest{ApiVersion: api.APIVersion, Role: "full", ProcessId: pid, MaxDurationSec: 30}, stream)
	}()
	time.Sleep(150 * time.Millisecond) // let it enter the select loop
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("SubscribeLogs after client ctx cancel: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("SubscribeLogs did not return after ctx cancel")
	}
}

func TestGRPCSubscribeLogsRunDoneBranch(t *testing.T) {
	t.Parallel()
	srv := newGRPCInternalTestServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	srv.runCancel = cancel
	srv.runDoneCh = ctx.Done()
	pid := newLogRecord(t, srv, "", "")
	g := &grpcAdapter{s: srv}
	// The stream's own context stays open; only the server's run context is
	// cancelled, isolating the runDone() branch from the ctx.Done() branch.
	stream := &fakeLogStream{ctxFn: func() context.Context { return context.Background() }}
	done := make(chan error, 1)
	go func() {
		done <- g.SubscribeLogs(&pmmcpv1.SubscribeLogsRequest{ApiVersion: api.APIVersion, Role: "full", ProcessId: pid, MaxDurationSec: 30}, stream)
	}()
	time.Sleep(150 * time.Millisecond)
	srv.runCancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("SubscribeLogs after run-context cancel: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("SubscribeLogs did not return after run-context cancel")
	}
	if len(stream.sends) != 1 || !stream.sends[0].GetEof() {
		t.Fatalf("sends = %+v, want exactly one EOF chunk", stream.sends)
	}
}

func TestGRPCSubscribeLogsFollowUpSendError(t *testing.T) {
	t.Parallel()
	srv := newGRPCInternalTestServer(t)
	pid := newLogRecord(t, srv, "", "") // no initial content -> skips initial-dump Send
	g := &grpcAdapter{s: srv}
	stream := &fakeLogStream{ctxFn: func() context.Context { return context.Background() }, sendErrs: []error{context.Canceled}}
	// Write new content shortly after subscribing so the 200ms ticker sees a
	// non-empty follow-up chunk and calls Send, which is rigged to fail.
	go func() {
		time.Sleep(250 * time.Millisecond)
		rec, err := srv.store.Get(context.Background(), pid)
		if err != nil {
			return
		}
		_ = os.WriteFile(filepath.Join(rec.LogDir, "stdout.log"), []byte("new data\n"), 0o600)
	}()
	err := g.SubscribeLogs(&pmmcpv1.SubscribeLogsRequest{ApiVersion: api.APIVersion, Role: "full", ProcessId: pid, MaxDurationSec: 5}, stream)
	if err == nil {
		t.Fatal("SubscribeLogs with a failing follow-up Send: want error, got nil")
	}
}

func TestReadFollowRotation(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "stdout.log")
	if err := os.WriteFile(path, []byte("0123456789"), 0o600); err != nil {
		t.Fatal(err)
	}
	text, off := readFollow(path, 0)
	if text != "0123456789" || off != 10 {
		t.Fatalf("initial readFollow = %q, off=%d", text, off)
	}
	// Simulate log rotation: the file is replaced with something shorter than
	// the current offset, so readFollow must reset to 0 and re-read from start.
	if err := os.WriteFile(path, []byte("ab"), 0o600); err != nil {
		t.Fatal(err)
	}
	text, off = readFollow(path, off)
	if text != "ab" || off != 2 {
		t.Fatalf("post-rotation readFollow = %q, off=%d, want %q, 2", text, off, "ab")
	}
}

func TestFollowLogDirCtxDone(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already done: the select's ctx.Done() case fires on the first loop iteration
	got := followLogDir(ctx, dir, 5*time.Second)
	if got != "" {
		t.Fatalf("followLogDir with no log files and a done ctx = %q, want empty", got)
	}
}

func TestFollowLogDirDetectsNewData(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "stdout.log"), []byte("initial"), 0o600); err != nil {
		t.Fatal(err)
	}
	go func() {
		time.Sleep(80 * time.Millisecond)
		_ = os.WriteFile(filepath.Join(dir, "stdout.log"), []byte("initialMORE"), 0o600)
	}()
	start := time.Now()
	got := followLogDir(context.Background(), dir, 5*time.Second)
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("followLogDir took %v, want an early return once new data appeared", elapsed)
	}
	if got != "MORE" {
		t.Fatalf("followLogDir = %q, want %q", got, "MORE")
	}
}

func TestFollowLogDirNaturalTimeout(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "stdout.log"), []byte("steady"), 0o600); err != nil {
		t.Fatal(err)
	}
	got := followLogDir(context.Background(), dir, 120*time.Millisecond)
	if got != "" {
		t.Fatalf("followLogDir with no new data = %q, want empty after natural timeout", got)
	}
}

func TestGRPCSubscribeEventsMaxDurationCappedAtFiveMinutes(t *testing.T) {
	t.Parallel()
	srv := newGRPCInternalTestServer(t)
	g := &grpcAdapter{s: srv}
	ctx, cancel := context.WithCancel(context.Background())
	stream := &fakeEventStream{ctxFn: func() context.Context { return ctx }}
	done := make(chan error, 1)
	go func() {
		done <- g.SubscribeEvents(&pmmcpv1.SubscribeEventsRequest{ApiVersion: api.APIVersion, Role: "full", MaxDurationSec: 36000}, stream)
	}()
	time.Sleep(150 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("SubscribeEvents: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("SubscribeEvents did not return after ctx cancel")
	}
}

func TestGRPCSubscribeEventsVersionMismatch(t *testing.T) {
	t.Parallel()
	srv := newGRPCInternalTestServer(t)
	g := &grpcAdapter{s: srv}
	stream := &fakeEventStream{ctxFn: func() context.Context { return context.Background() }}
	err := g.SubscribeEvents(&pmmcpv1.SubscribeEventsRequest{ApiVersion: "99.0", Role: "full"}, stream)
	if err == nil {
		t.Fatal("SubscribeEvents with mismatched api_version: want error, got nil")
	}
}

func TestGRPCSubscribeEventsAuthzDenied(t *testing.T) {
	t.Parallel()
	srv := newGRPCInternalTestServer(t)
	g := &grpcAdapter{s: srv}
	stream := &fakeEventStream{ctxFn: func() context.Context { return context.Background() }}
	err := g.SubscribeEvents(&pmmcpv1.SubscribeEventsRequest{ApiVersion: api.APIVersion, Role: "bogus"}, stream)
	if err == nil {
		t.Fatal("SubscribeEvents with an unrecognized role: want permission denied, got nil")
	}
}

func TestGRPCSubscribeEventsSeedSendError(t *testing.T) {
	t.Parallel()
	srv := newGRPCInternalTestServer(t)
	ctx := context.Background()
	if _, err := srv.events.Append(ctx, seedEvent("proc-1")); err != nil {
		t.Fatal(err)
	}
	g := &grpcAdapter{s: srv}
	stream := &fakeEventStream{ctxFn: func() context.Context { return ctx }, sendErrs: []error{context.Canceled}}
	err := g.SubscribeEvents(&pmmcpv1.SubscribeEventsRequest{ApiVersion: api.APIVersion, Role: "full"}, stream)
	if err == nil {
		t.Fatal("SubscribeEvents with a failing seed Send: want error, got nil")
	}
}

func TestGRPCSubscribeEventsCtxDoneBranch(t *testing.T) {
	t.Parallel()
	srv := newGRPCInternalTestServer(t)
	g := &grpcAdapter{s: srv}
	ctx, cancel := context.WithCancel(context.Background())
	stream := &fakeEventStream{ctxFn: func() context.Context { return ctx }}
	done := make(chan error, 1)
	go func() {
		// MaxDurationSec=0 exercises the max<=0 default branch too.
		done <- g.SubscribeEvents(&pmmcpv1.SubscribeEventsRequest{ApiVersion: api.APIVersion, Role: "full"}, stream)
	}()
	time.Sleep(150 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("SubscribeEvents after ctx cancel: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("SubscribeEvents did not return after ctx cancel")
	}
	if len(stream.sends) != 1 || !stream.sends[0].GetEof() {
		t.Fatalf("sends = %+v, want exactly one EOF chunk", stream.sends)
	}
}

func TestGRPCSubscribeEventsRunDoneBranch(t *testing.T) {
	t.Parallel()
	srv := newGRPCInternalTestServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	srv.runCancel = cancel
	srv.runDoneCh = ctx.Done()
	g := &grpcAdapter{s: srv}
	stream := &fakeEventStream{ctxFn: func() context.Context { return context.Background() }}
	done := make(chan error, 1)
	go func() {
		done <- g.SubscribeEvents(&pmmcpv1.SubscribeEventsRequest{ApiVersion: api.APIVersion, Role: "full", MaxDurationSec: 30}, stream)
	}()
	time.Sleep(150 * time.Millisecond)
	srv.runCancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("SubscribeEvents after run-context cancel: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("SubscribeEvents did not return after run-context cancel")
	}
}

func TestGRPCSubscribeEventsTickerNewEventAndSendError(t *testing.T) {
	t.Parallel()
	srv := newGRPCInternalTestServer(t)
	g := &grpcAdapter{s: srv}
	stream := &fakeEventStream{ctxFn: func() context.Context { return context.Background() }, sendErrs: []error{context.Canceled}}
	go func() {
		time.Sleep(300 * time.Millisecond)
		_, _ = srv.events.Append(context.Background(), seedEvent("proc-2"))
	}()
	err := g.SubscribeEvents(&pmmcpv1.SubscribeEventsRequest{ApiVersion: api.APIVersion, Role: "full", MaxDurationSec: 5}, stream)
	if err == nil {
		t.Fatal("SubscribeEvents with a failing follow-up Send: want error, got nil")
	}
}

func TestGRPCSubscribeEventsNaturalDeadline(t *testing.T) {
	t.Parallel()
	srv := newGRPCInternalTestServer(t)
	if _, err := srv.events.Append(context.Background(), seedEvent("proc-3")); err != nil {
		t.Fatal(err)
	}
	g := &grpcAdapter{s: srv}
	stream := &fakeEventStream{ctxFn: func() context.Context { return context.Background() }}
	// Cap far below 5 minutes (MaxDurationSec omitted is fine — max<=0
	// defaults to 30s, too long for a unit test) so use a tiny explicit
	// duration and let the loop's deadline elapse naturally.
	err := g.SubscribeEvents(&pmmcpv1.SubscribeEventsRequest{ApiVersion: api.APIVersion, Role: "full", MaxDurationSec: 1}, stream)
	if err != nil {
		t.Fatalf("SubscribeEvents: %v", err)
	}
	if len(stream.sends) < 2 {
		t.Fatalf("sends = %+v, want the seeded event plus a final EOF", stream.sends)
	}
	last := stream.sends[len(stream.sends)-1]
	if !last.GetEof() {
		t.Fatalf("last send = %+v, want Eof", last)
	}
}
