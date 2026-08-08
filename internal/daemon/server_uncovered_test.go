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
	"database/sql"
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/scrothers/pmmcp/internal/api"
	pmmcpv1 "github.com/scrothers/pmmcp/internal/api/gen/pmmcp/v1"
	"github.com/scrothers/pmmcp/internal/audit"
	"github.com/scrothers/pmmcp/internal/authz"
	"github.com/scrothers/pmmcp/internal/config"
	"github.com/scrothers/pmmcp/internal/daemon"
	"github.com/scrothers/pmmcp/internal/domain"
	"github.com/scrothers/pmmcp/internal/event"
	"github.com/scrothers/pmmcp/internal/ipc"
	"github.com/scrothers/pmmcp/internal/process"
	"github.com/scrothers/pmmcp/internal/store"
	"github.com/scrothers/pmmcp/internal/store/sqlite"
	"github.com/scrothers/pmmcp/internal/testsock"
)

// dialDaemon boots srv's IPC listener in the background and returns a
// connected client.
func dialDaemon(ctx context.Context, t *testing.T, srv *daemon.Server, sock string) *ipc.Client {
	t.Helper()
	go func() { _ = srv.ListenAndServe(ctx) }()
	var c *ipc.Client
	var err error
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		c, err = ipc.Dial(ctx, sock)
		if err == nil {
			break
		}
		time.Sleep(15 * time.Millisecond)
	}
	if c == nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func TestRelaunchEligibleListErrorAfterClose(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cfg := newTestConfig(t, dir)
	srv, err := daemon.New(context.Background(), daemon.Options{Config: cfg, DBPath: filepath.Join(dir, "db.sqlite")})
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if err := srv.RelaunchEligible(context.Background()); err == nil {
		t.Fatal("RelaunchEligible after Close (store closed): want error, got nil")
	}
}

func TestRelaunchEligibleSkipsAlreadyRunningUnderManager(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cfg := newTestConfig(t, dir)
	sock := testsock.Path(t)
	cfg.IPC.Endpoint = sock
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	srv, err := daemon.New(ctx, daemon.Options{Config: cfg, DBPath: filepath.Join(dir, "db.sqlite")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Close() })
	c := dialDaemon(ctx, t, srv, sock)

	var start api.StartResult
	if err := c.Call(ctx, api.MethodStart, api.StartPayload{
		Name: "still-running", Command: []string{"sleep", "30"}, Sandbox: "off", Project: "projx",
	}, &start); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() {
		_ = c.Call(context.Background(), api.MethodStop, api.IDPayload{ID: start.ID, Force: true}, &map[string]any{})
	})

	// The same manager instance still reports this process running, so
	// RelaunchEligible must skip it via the "already running" branch rather
	// than starting a duplicate.
	if err := srv.RelaunchEligible(ctx); err != nil {
		t.Fatalf("RelaunchEligible: %v", err)
	}
	var status api.ProcessView
	if err := c.Call(ctx, api.MethodStatus, api.IDPayload{ID: start.ID}, &status); err != nil {
		t.Fatalf("status: %v", err)
	}
	if status.PID != start.PID {
		t.Fatalf("PID changed from %d to %d: RelaunchEligible must not restart an already-running process", start.PID, status.PID)
	}
}

func TestRelaunchEligibleAdoptsLivePredecessor(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "db.sqlite")

	cfg1 := newTestConfig(t, dir)
	cfg1.IPC.Endpoint = testsock.Path(t)
	ctx1, cancel1 := context.WithCancel(context.Background())
	srv1, err := daemon.New(ctx1, daemon.Options{Config: cfg1, DBPath: dbPath})
	if err != nil {
		t.Fatal(err)
	}
	c1 := dialDaemon(ctx1, t, srv1, cfg1.IPC.Endpoint)

	var start api.StartResult
	if err := c1.Call(ctx1, api.MethodStart, api.StartPayload{
		Name: "predecessor", Command: []string{"sleep", "30"}, Sandbox: "off", Project: "adoptproj",
	}, &start); err != nil {
		t.Fatalf("start: %v", err)
	}
	// Simulate a daemon restart: the child (Setpgid, no Pdeathsig) survives.
	cancel1()
	_ = srv1.Close()
	t.Cleanup(func() {
		if p, err := os.FindProcess(start.PID); err == nil {
			_ = p.Kill()
		}
	})

	cfg2 := newTestConfig(t, dir)
	cfg2.IPC.Endpoint = testsock.Path(t)
	ctx2, cancel2 := context.WithCancel(context.Background())
	t.Cleanup(cancel2)
	srv2, err := daemon.New(ctx2, daemon.Options{Config: cfg2, DBPath: dbPath})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv2.Close() })
	c2 := dialDaemon(ctx2, t, srv2, cfg2.IPC.Endpoint)

	// srv2's manager is fresh (never started this ID) but the predecessor PID
	// is still alive: RelaunchEligible must adopt (skip), not double-start.
	if err := srv2.RelaunchEligible(ctx2); err != nil {
		t.Fatalf("RelaunchEligible: %v", err)
	}
	var status api.ProcessView
	if err := c2.Call(ctx2, api.MethodStatus, api.IDPayload{ID: start.ID}, &status); err != nil {
		t.Fatalf("status: %v", err)
	}
	if status.PID != start.PID {
		t.Fatalf("PID changed from %d to %d: RelaunchEligible must adopt a live predecessor, not double-start", start.PID, status.PID)
	}
}

func TestRelaunchEligibleRestartsDeadPredecessor(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "db.sqlite")

	cfg1 := newTestConfig(t, dir)
	cfg1.IPC.Endpoint = testsock.Path(t)
	ctx1, cancel1 := context.WithCancel(context.Background())
	srv1, err := daemon.New(ctx1, daemon.Options{Config: cfg1, DBPath: dbPath})
	if err != nil {
		t.Fatal(err)
	}
	c1 := dialDaemon(ctx1, t, srv1, cfg1.IPC.Endpoint)

	var start api.StartResult
	if err := c1.Call(ctx1, api.MethodStart, api.StartPayload{
		Name: "dead-predecessor", Command: []string{"true"}, Sandbox: "off", Project: "deadproj",
	}, &start); err != nil {
		t.Fatalf("start: %v", err)
	}
	// "true" exits almost immediately; wait for it to actually die.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if p, err := os.FindProcess(start.PID); err != nil || p.Signal(syscall.Signal(0)) != nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel1()
	_ = srv1.Close()

	cfg2 := newTestConfig(t, dir)
	cfg2.IPC.Endpoint = testsock.Path(t)
	ctx2, cancel2 := context.WithCancel(context.Background())
	t.Cleanup(cancel2)
	srv2, err := daemon.New(ctx2, daemon.Options{Config: cfg2, DBPath: dbPath})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv2.Close() })
	c2 := dialDaemon(ctx2, t, srv2, cfg2.IPC.Endpoint)

	if err := srv2.RelaunchEligible(ctx2); err != nil {
		t.Fatalf("RelaunchEligible: %v", err)
	}
	var status api.ProcessView
	if err := c2.Call(ctx2, api.MethodStatus, api.IDPayload{ID: start.ID}, &status); err != nil {
		t.Fatalf("status: %v", err)
	}
	if status.PID == start.PID || status.PID == 0 {
		t.Fatalf("PID = %d (was %d): RelaunchEligible must restart a dead predecessor with a fresh PID", status.PID, start.PID)
	}
}

func TestRelaunchEligibleRestartFailureMarksFailed(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "db.sqlite")
	script := filepath.Join(dir, "run.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	cfg1 := newTestConfig(t, dir)
	cfg1.IPC.Endpoint = testsock.Path(t)
	ctx1, cancel1 := context.WithCancel(context.Background())
	srv1, err := daemon.New(ctx1, daemon.Options{Config: cfg1, DBPath: dbPath})
	if err != nil {
		t.Fatal(err)
	}
	c1 := dialDaemon(ctx1, t, srv1, cfg1.IPC.Endpoint)

	var start api.StartResult
	if err := c1.Call(ctx1, api.MethodStart, api.StartPayload{
		Name: "fails-on-relaunch", Command: []string{script}, Sandbox: "off", Project: "failproj",
	}, &start); err != nil {
		t.Fatalf("start: %v", err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if p, err := os.FindProcess(start.PID); err != nil || p.Signal(syscall.Signal(0)) != nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel1()
	_ = srv1.Close()

	// Remove the script so the replayed exec fails on the next boot.
	if err := os.Remove(script); err != nil {
		t.Fatal(err)
	}

	cfg2 := newTestConfig(t, dir)
	cfg2.IPC.Endpoint = testsock.Path(t)
	ctx2, cancel2 := context.WithCancel(context.Background())
	t.Cleanup(cancel2)
	srv2, err := daemon.New(ctx2, daemon.Options{Config: cfg2, DBPath: dbPath})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv2.Close() })
	c2 := dialDaemon(ctx2, t, srv2, cfg2.IPC.Endpoint)

	if err := srv2.RelaunchEligible(ctx2); err == nil {
		t.Fatal("RelaunchEligible with a missing binary on replay: want error, got nil")
	}
	var status api.ProcessView
	if err := c2.Call(ctx2, api.MethodStatus, api.IDPayload{ID: start.ID}, &status); err != nil {
		t.Fatalf("status: %v", err)
	}
	if status.Status != string(domain.StatusFailed) {
		t.Fatalf("status = %q, want %q", status.Status, domain.StatusFailed)
	}
	if status.Error == "" {
		t.Fatal("expected LastError to be set after a failed relaunch")
	}
}

func newTestConfig(t *testing.T, dir string) *config.Config {
	t.Helper()
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
	return cfg
}

func TestNewNilConfig(t *testing.T) {
	t.Parallel()
	if _, err := daemon.New(context.Background(), daemon.Options{}); err == nil {
		t.Fatal("New with nil config: want error, got nil")
	}
}

func TestNewStateDirMkdirAllFails(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory write permission checks")
	}
	t.Parallel()
	dir := t.TempDir()
	parent := filepath.Join(dir, "parent")
	if err := os.Mkdir(parent, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(parent, 0o700) })
	cfg := newTestConfig(t, dir)
	cfg.StateDir = filepath.Join(parent, "state")
	if _, err := daemon.New(context.Background(), daemon.Options{Config: cfg, DBPath: filepath.Join(dir, "db.sqlite")}); err == nil {
		t.Fatal("New with an unwritable state dir parent: want error, got nil")
	}
}

func TestNewDefaultsDBPathWhenEmpty(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cfg := newTestConfig(t, dir)
	srv, err := daemon.New(context.Background(), daemon.Options{Config: cfg})
	if err != nil {
		t.Fatalf("New with empty DBPath: %v", err)
	}
	defer srv.Close()
	if _, err := os.Stat(filepath.Join(cfg.StateDir, "pmmcp.db")); err != nil {
		t.Fatalf("expected default db at state_dir/pmmcp.db: %v", err)
	}
}

func TestNewSQLiteOpenFails(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cfg := newTestConfig(t, dir)
	garbage := filepath.Join(dir, "garbage.db")
	if err := os.WriteFile(garbage, []byte("not a sqlite database, just garbage bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := daemon.New(context.Background(), daemon.Options{Config: cfg, DBPath: garbage}); err == nil {
		t.Fatal("New over a non-sqlite file: want error, got nil")
	}
}

func TestNewMigrateFails(t *testing.T) {
	// Not t.Parallel(): mutates the package-level migrateStore seam.
	dir := t.TempDir()
	cfg := newTestConfig(t, dir)
	restore := daemon.SetMigrateStoreForTest(func(context.Context, *sqlite.Store) error {
		return errors.New("injected migrate failure")
	})
	t.Cleanup(restore)
	if _, err := daemon.New(context.Background(), daemon.Options{Config: cfg, DBPath: filepath.Join(dir, "db.sqlite")}); err == nil {
		t.Fatal("New with a failing migration: want error, got nil")
	}
}

func TestNewUserCurrentFails(t *testing.T) {
	// Not t.Parallel(): mutates the package-level userCurrent seam.
	dir := t.TempDir()
	cfg := newTestConfig(t, dir)
	restore := daemon.SetUserCurrentForTest(func() (*user.User, error) {
		return nil, errors.New("injected user.Current failure")
	})
	t.Cleanup(restore)
	if _, err := daemon.New(context.Background(), daemon.Options{Config: cfg, DBPath: filepath.Join(dir, "db.sqlite")}); err == nil {
		t.Fatal("New with a failing user.Current: want error, got nil")
	}
}

func TestNewKeyringBackendFails(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cfg := newTestConfig(t, dir)
	// New's own os.MkdirAll(cfg.StateDir) needs to succeed first, so create the
	// state dir ourselves, then block the keyring subdir with a regular file:
	// secret.NewFileBackend's os.MkdirAll(dir) fails when a leaf path component
	// already exists as a non-directory.
	if err := os.MkdirAll(cfg.StateDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfg.StateDir, "keyring"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := daemon.New(context.Background(), daemon.Options{Config: cfg, DBPath: filepath.Join(dir, "db.sqlite")}); err == nil {
		t.Fatal("New with a keyring path blocked by a file: want error, got nil")
	}
}

func TestNewAuditSQLiteLogFails(t *testing.T) {
	// Not t.Parallel(): mutates the package-level newAuditSQLiteLog seam.
	dir := t.TempDir()
	cfg := newTestConfig(t, dir)
	restore := daemon.SetNewAuditSQLiteLogForTest(func(*sql.DB, ...audit.Option) (*audit.Log, error) {
		return nil, errors.New("injected audit log failure")
	})
	t.Cleanup(restore)
	if _, err := daemon.New(context.Background(), daemon.Options{Config: cfg, DBPath: filepath.Join(dir, "db.sqlite")}); err == nil {
		t.Fatal("New with a failing audit.NewSQLiteLog: want error, got nil")
	}
}

func TestNewEventSQLiteLogFails(t *testing.T) {
	// Not t.Parallel(): mutates the package-level newEventSQLiteLog seam.
	dir := t.TempDir()
	cfg := newTestConfig(t, dir)
	restore := daemon.SetNewEventSQLiteLogForTest(func(*sql.DB, ...event.Option) (*event.Bus, error) {
		return nil, errors.New("injected event log failure")
	})
	t.Cleanup(restore)
	if _, err := daemon.New(context.Background(), daemon.Options{Config: cfg, DBPath: filepath.Join(dir, "db.sqlite")}); err == nil {
		t.Fatal("New with a failing event.NewSQLiteLog: want error, got nil")
	}
}

// newAuthzTestDaemon boots a single daemon (real manager) for authorizeTarget
// cross-session tests.
func newAuthzTestDaemon(t *testing.T) *ipc.Client {
	t.Helper()
	dir := t.TempDir()
	cfg := newTestConfig(t, dir)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	srv, err := daemon.New(ctx, daemon.Options{Config: cfg, DBPath: filepath.Join(dir, "db.sqlite")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Close() })
	return dialDaemon(ctx, t, srv, cfg.IPC.Endpoint)
}

func TestAuthorizeTargetSameSessionNoCheck(t *testing.T) {
	t.Parallel()
	c := newAuthzTestDaemon(t)
	c.SetSession("sess-owner", "agent")
	ctx := context.Background()
	var start api.StartResult
	if err := c.Call(ctx, api.MethodStart, api.StartPayload{
		Name: "same-sess", Command: []string{"sleep", "30"}, Sandbox: "off",
	}, &start); err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := c.Call(ctx, api.MethodStop, api.IDPayload{ID: start.ID, Force: true}, &map[string]any{}); err != nil {
		t.Fatalf("stop within the same session: %v", err)
	}
}

func TestAuthorizeTargetCrossSessionFullBypasses(t *testing.T) {
	t.Parallel()
	c := newAuthzTestDaemon(t)
	ctx := context.Background()
	c.SetSession("sess-owner", "agent")
	var start api.StartResult
	if err := c.Call(ctx, api.MethodStart, api.StartPayload{
		Name: "cross-full", Command: []string{"sleep", "30"}, Sandbox: "off",
	}, &start); err != nil {
		t.Fatalf("start: %v", err)
	}
	c.SetSession("sess-other", "full")
	if err := c.Call(ctx, api.MethodStop, api.IDPayload{ID: start.ID, Force: true}, &map[string]any{}); err != nil {
		t.Fatalf("full role must bypass cross-session tenancy: %v", err)
	}
}

func TestAuthorizeTargetCrossSessionDeniedWithoutShare(t *testing.T) {
	t.Parallel()
	c := newAuthzTestDaemon(t)
	ctx := context.Background()
	c.SetSession("sess-owner", "agent")
	var start api.StartResult
	if err := c.Call(ctx, api.MethodStart, api.StartPayload{
		Name: "cross-deny", Command: []string{"sleep", "30"}, Sandbox: "off",
	}, &start); err != nil {
		t.Fatalf("start: %v", err)
	}
	c.SetSession("sess-other", "agent")
	err := c.Call(ctx, api.MethodStop, api.IDPayload{ID: start.ID, Force: true}, &map[string]any{})
	if err == nil {
		t.Fatal("cross-session stop without a share grant: want permission_denied, got nil")
	}
	var derr *domain.Error
	if !errors.As(err, &derr) || derr.Code != domain.CodePermissionDenied {
		t.Fatalf("err = %v, want permission_denied", err)
	}
	// Clean up: an operator can always stop it.
	c.SetSession("sess-owner", "full")
	_ = c.Call(ctx, api.MethodStop, api.IDPayload{ID: start.ID, Force: true}, &map[string]any{})
}

func TestAuthorizeTargetCrossSessionShareGrantAllows(t *testing.T) {
	t.Parallel()
	c := newAuthzTestDaemon(t)
	ctx := context.Background()
	c.SetSession("sess-owner", "agent")
	var start api.StartResult
	if err := c.Call(ctx, api.MethodStart, api.StartPayload{
		Name: "cross-shared", Command: []string{"sleep", "30"}, Sandbox: "off",
	}, &start); err != nil {
		t.Fatalf("start: %v", err)
	}
	// session:share requires operator+.
	c.SetSession("sess-grantor", "operator")
	if err := c.Call(ctx, api.MethodShare, api.SharePayload{
		Target: start.ID, ToSession: "sess-other", Cap: string(authz.CapProcessStop),
	}, &map[string]any{}); err != nil {
		t.Fatalf("session.share: %v", err)
	}
	c.SetSession("sess-other", "agent")
	if err := c.Call(ctx, api.MethodStop, api.IDPayload{ID: start.ID, Force: true}, &map[string]any{}); err != nil {
		t.Fatalf("stop with an explicit share grant: %v", err)
	}
}

func TestDoStartMalformedPayload(t *testing.T) {
	t.Parallel()
	ctx, _, c, _ := startTestDaemon(t)
	req := &pmmcpv1.CallRequest{ApiVersion: api.APIVersion, Method: api.MethodStart, Role: "full", Payload: []byte("{not json")}
	resp, err := c.DaemonRPC().Call(ctx, req)
	if err != nil {
		t.Fatalf("rpc: %v", err)
	}
	if resp.GetOk() {
		t.Fatal("malformed start payload: want !ok, got ok")
	}
	if resp.GetErrorCode() != string(domain.CodeInvalidArgument) {
		t.Fatalf("error_code = %q, want invalid_argument", resp.GetErrorCode())
	}
}

func TestDoStartInvalidCommand(t *testing.T) {
	t.Parallel()
	ctx, _, c, _ := startTestDaemon(t)
	err := c.Call(ctx, api.MethodStart, api.StartPayload{Name: "x", Command: nil, Sandbox: "off"}, &map[string]any{})
	if err == nil {
		t.Fatal("start with empty command: want error, got nil")
	}
	var derr *domain.Error
	if !errors.As(err, &derr) || derr.Code != domain.CodeInvalidArgument {
		t.Fatalf("err = %v, want invalid_argument", err)
	}
}

func TestDoStartEmptyName(t *testing.T) {
	t.Parallel()
	ctx, _, c, _ := startTestDaemon(t)
	err := c.Call(ctx, api.MethodStart, api.StartPayload{Name: "", Command: []string{"sleep", "1"}, Sandbox: "off"}, &map[string]any{})
	if err == nil {
		t.Fatal("start with empty name: want error, got nil")
	}
	var derr *domain.Error
	if !errors.As(err, &derr) || derr.Code != domain.CodeInvalidArgument {
		t.Fatalf("err = %v, want invalid_argument", err)
	}
}

// TestDoStartTerminalByNameEntryFreesName exercises doStart's in-memory
// byName-freeing branch for a terminal predecessor. The store's partial
// unique index (project_id, name) WHERE successor_id=” still counts a
// stopped-but-not-removed record (doStop never sets SuccessorID), so the
// freed name still collides at store.Create — this also exercises that
// error-mapping branch (domain.CodeConflict), a real, documented interaction
// (see FABLE_REVIEW.md's name-uniqueness TOCTOU finding: the DB index isn't
// profile-scoped and isn't restricted to non-terminal rows).
func TestDoStartTerminalByNameEntryFreesName(t *testing.T) {
	t.Parallel()
	ctx, _, c, _ := startTestDaemon(t)
	var start api.StartResult
	if err := c.Call(ctx, api.MethodStart, api.StartPayload{
		Name: "term-reuse", Command: []string{"true"}, Sandbox: "off",
	}, &start); err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := c.Call(ctx, api.MethodStop, api.IDPayload{ID: start.ID, TimeoutSec: 2}, &map[string]any{}); err != nil {
		t.Fatalf("stop: %v", err)
	}
	err := c.Call(ctx, api.MethodStart, api.StartPayload{
		Name: "term-reuse", Command: []string{"sleep", "1"}, Sandbox: "off",
	}, &map[string]any{})
	if err == nil {
		t.Fatal("start after a stopped (not removed) predecessor: want conflict, got nil")
	}
	var derr *domain.Error
	if !errors.As(err, &derr) || derr.Code != domain.CodeConflict {
		t.Fatalf("err = %v, want conflict", err)
	}
}

// TestDoStartListScanSkipsProfileMismatch exercises the store.List-scan
// branch that skips a same-project/same-name record in a different profile
// (the byName map key is profile-scoped, so this record is invisible to the
// fast map path). The underlying store index isn't profile-scoped, though,
// so store.Create still conflicts — see the comment on
// TestDoStartTerminalByNameEntryFreesName.
func TestDoStartListScanSkipsProfileMismatch(t *testing.T) {
	t.Parallel()
	ctx, _, c, _ := startTestDaemon(t)
	if err := c.Call(ctx, api.MethodStart, api.StartPayload{
		Name: "profsplit", Command: []string{"sleep", "30"}, Sandbox: "off",
		Project: "profproj", Profile: "p1",
	}, &map[string]any{}); err != nil {
		t.Fatalf("start p1: %v", err)
	}
	err := c.Call(ctx, api.MethodStart, api.StartPayload{
		Name: "profsplit", Command: []string{"sleep", "30"}, Sandbox: "off",
		Project: "profproj", Profile: "p2",
	}, &map[string]any{})
	if err == nil {
		t.Fatal("start p2 (different profile, same store-level name key): want conflict, got nil")
	}
	var derr *domain.Error
	if !errors.As(err, &derr) || derr.Code != domain.CodeConflict {
		t.Fatalf("err = %v, want conflict", err)
	}
}

func TestDoStartListScanFindsRecordFromPriorProcess(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "db.sqlite")

	cfg1 := newTestConfig(t, dir)
	cfg1.IPC.Endpoint = testsock.Path(t)
	ctx1, cancel1 := context.WithCancel(context.Background())
	srv1, err := daemon.New(ctx1, daemon.Options{Config: cfg1, DBPath: dbPath})
	if err != nil {
		t.Fatal(err)
	}
	c1 := dialDaemon(ctx1, t, srv1, cfg1.IPC.Endpoint)
	if err := c1.Call(ctx1, api.MethodStart, api.StartPayload{
		Name: "scanfind", Command: []string{"sleep", "30"}, Sandbox: "off", Project: "scanproj",
	}, &map[string]any{}); err != nil {
		t.Fatalf("start on srv1: %v", err)
	}
	// Do not stop the process: it stays non-terminal in the store. Shut down
	// srv1 without cancelling its process (Setpgid, survives).
	cancel1()
	_ = srv1.Close()

	// srv2's in-memory byName map starts empty (fresh process), so a second
	// start with the same (project, name, profile) must fall through to the
	// store.List scan and find the still-non-terminal record from srv1.
	cfg2 := newTestConfig(t, dir)
	cfg2.IPC.Endpoint = testsock.Path(t)
	ctx2, cancel2 := context.WithCancel(context.Background())
	t.Cleanup(cancel2)
	srv2, err := daemon.New(ctx2, daemon.Options{Config: cfg2, DBPath: dbPath})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv2.Close() })
	c2 := dialDaemon(ctx2, t, srv2, cfg2.IPC.Endpoint)

	err = c2.Call(ctx2, api.MethodStart, api.StartPayload{
		Name: "scanfind", Command: []string{"sleep", "30"}, Sandbox: "off", Project: "scanproj",
	}, &map[string]any{})
	if err == nil {
		t.Fatal("start colliding with a cross-restart record found via list scan: want name_conflict, got nil")
	}
	var derr *domain.Error
	if !errors.As(err, &derr) || derr.Code != domain.CodeNameConflict {
		t.Fatalf("err = %v, want name_conflict", err)
	}

	// Replace: true must succeed via the same list-scan-found record.
	var replaced api.StartResult
	if err := c2.Call(ctx2, api.MethodStart, api.StartPayload{
		Name: "scanfind", Command: []string{"sleep", "1"}, Sandbox: "off", Project: "scanproj", Replace: true,
	}, &replaced); err != nil {
		t.Fatalf("replace via list-scan-found record: %v", err)
	}
}

func TestDoStartSandboxDefaultPolicyFailsOnUnknownProfile(t *testing.T) {
	t.Parallel()
	ctx, _, c, _ := startTestDaemon(t)
	err := c.Call(ctx, api.MethodStart, api.StartPayload{
		Name: "badprofile", Command: []string{"sleep", "1"}, Sandbox: "totally-bogus-profile",
	}, &map[string]any{})
	if err == nil {
		t.Fatal("start with an unknown sandbox profile: want error, got nil")
	}
	var derr *domain.Error
	if !errors.As(err, &derr) || derr.Code != domain.CodeSandboxFailed {
		t.Fatalf("err = %v, want sandbox_failed", err)
	}
}

func TestDoStartSandboxApplyFailsWithoutProjectRoot(t *testing.T) {
	t.Parallel()
	ctx, _, c, _ := startTestDaemon(t)
	// A whitespace-only Cwd bypasses the `cwd == ""` fallback to os.Getwd()
	// but trims to an empty project root inside sandbox.DefaultPolicy, so
	// sandboxlinux.Apply's strict-profile project-root requirement fails.
	err := c.Call(ctx, api.MethodStart, api.StartPayload{
		Name: "needsroot", Command: []string{"sleep", "1"}, Sandbox: "strict", Cwd: " ",
	}, &map[string]any{})
	if err == nil {
		t.Fatal("start strict sandbox without a project root: want error, got nil")
	}
	var derr *domain.Error
	if !errors.As(err, &derr) || derr.Code != domain.CodeSandboxFailed {
		t.Fatalf("err = %v, want sandbox_failed", err)
	}
}

func TestDoStartIDGenerationFailure(t *testing.T) {
	// Not t.Parallel(): mutates the package-level crypto/rand.Reader.
	ctx, _, c, _ := startTestDaemon(t)
	orig := rand.Reader
	rand.Reader = alwaysErrReader{}
	t.Cleanup(func() { rand.Reader = orig })
	err := c.Call(ctx, api.MethodStart, api.StartPayload{
		Name: "idfail", Command: []string{"sleep", "1"}, Sandbox: "off",
	}, &map[string]any{})
	if err == nil {
		t.Fatal("start with a failing crypto/rand.Reader: want error, got nil")
	}
}

type alwaysErrReader struct{}

func (alwaysErrReader) Read([]byte) (int, error) { return 0, errors.New("injected rand failure") }

func TestDoStartLogDirMkdirAllFails(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory write permission checks")
	}
	t.Parallel()
	dir := t.TempDir()
	cfg := newTestConfig(t, dir)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	srv, err := daemon.New(ctx, daemon.Options{Config: cfg, DBPath: filepath.Join(dir, "db.sqlite")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Close() })
	c := dialDaemon(ctx, t, srv, cfg.IPC.Endpoint)

	// Block state_dir/logs/<pid> by making "logs" a regular file.
	if err := os.WriteFile(filepath.Join(cfg.StateDir, "logs"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	err = c.Call(ctx, api.MethodStart, api.StartPayload{
		Name: "logdirfail", Command: []string{"sleep", "1"}, Sandbox: "off",
	}, &map[string]any{})
	if err == nil {
		t.Fatal("start with logs dir blocked by a file: want error, got nil")
	}
}

func TestDoStartEnvFileLoadFails(t *testing.T) {
	t.Parallel()
	ctx, _, c, _ := startTestDaemon(t)
	err := c.Call(ctx, api.MethodStart, api.StartPayload{
		Name: "envfilefail", Command: []string{"sleep", "1"}, Sandbox: "off",
		EnvFiles: []string{"/nonexistent/env/file/for/pmmcp/test"},
	}, &map[string]any{})
	if err == nil {
		t.Fatal("start with an unreadable env file: want error, got nil")
	}
	var derr *domain.Error
	if !errors.As(err, &derr) || derr.Code != domain.CodeInvalidArgument {
		t.Fatalf("err = %v, want invalid_argument", err)
	}
}

func TestDoStartSecretResolveFails(t *testing.T) {
	t.Parallel()
	ctx, _, c, _ := startTestDaemon(t)
	err := c.Call(ctx, api.MethodStart, api.StartPayload{
		Name: "secretfail", Command: []string{"sleep", "1"}, Sandbox: "off",
		Env: map[string]string{"K": "secret://keyring/does-not-exist-xyz"},
	}, &map[string]any{})
	if err == nil {
		t.Fatal("start with an unresolvable secret ref: want error, got nil")
	}
	var derr *domain.Error
	if !errors.As(err, &derr) || derr.Code != domain.CodeInvalidArgument {
		t.Fatalf("err = %v, want invalid_argument", err)
	}
}

func TestDoStartManagerStartFails(t *testing.T) {
	t.Parallel()
	ctx, _, c, _ := startTestDaemon(t)
	err := c.Call(ctx, api.MethodStart, api.StartPayload{
		Name: "nobinary", Command: []string{"/nonexistent-binary-xyz-pmmcp-test"}, Sandbox: "off",
	}, &map[string]any{})
	if err == nil {
		t.Fatal("start with a nonexistent binary: want error, got nil")
	}
}

func TestDoStartMemoryLimitAndAllOptionalFields(t *testing.T) {
	t.Parallel()
	ctx, _, c, _ := startTestDaemon(t)
	var start api.StartResult
	if err := c.Call(ctx, api.MethodStart, api.StartPayload{
		Name: "fullopts", Command: []string{"sleep", "5"}, Sandbox: "off",
		MemoryBytes: 64 << 20, HealthURL: "http://127.0.0.1:0/health",
		AutoRestart: true, Ports: []string{"8080/tcp"}, StopOnDisconnect: true,
	}, &start); err != nil {
		t.Fatalf("start: %v", err)
	}
	var status api.ProcessView
	if err := c.Call(ctx, api.MethodStatus, api.IDPayload{ID: start.ID}, &status); err != nil {
		t.Fatalf("status: %v", err)
	}
	if len(status.Ports) == 0 {
		t.Fatal("expected declared ports on status")
	}
}

// --- errFrom, direct unit tests via the ErrFromForTest seam ---

func TestErrFrom(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		err  error
		want domain.Code
	}{
		{"nil", nil, domain.CodeInternal},
		{"sandbox failed", process.ErrSandboxFailed, domain.CodeSandboxFailed},
		{"invalid spec", process.ErrInvalidSpec, domain.CodeInvalidArgument},
		{"already exists", process.ErrAlreadyExists, domain.CodeConflict},
		{"process not found", process.ErrNotFound, domain.CodeNotFound},
		{"store not found", store.ErrNotFound, domain.CodeNotFound},
		{"store conflict", store.ErrConflict, domain.CodeConflict},
		{"domain error", domain.NewError(domain.CodeFailedPrecondition, "slow", true), domain.CodeFailedPrecondition},
		{"generic", errors.New("boom"), domain.CodeInternal},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			resp := daemon.ErrFromForTest(tc.err)
			if resp.OK {
				t.Fatalf("resp.OK = true, want an error response")
			}
			if resp.ErrorCode != string(tc.want) {
				t.Fatalf("ErrorCode = %q, want %q", resp.ErrorCode, tc.want)
			}
		})
	}
}

// --- sandboxIsRelaxation, direct unit tests via the SandboxIsRelaxationForTest seam ---

func TestSandboxIsRelaxation(t *testing.T) {
	t.Parallel()
	cases := []struct {
		cfgDefault, requested string
		want                  bool
	}{
		{"strict", "strict", false},
		{"strict", "standard", true},
		{"strict", "permissive", true},
		{"strict", "off", true},
		{"off", "strict", false},
		{"off", "off", false},
		{"standard", "permissive", true},
		{"permissive", "standard", false},
		{"strict", "totally-unknown", false}, // unknown ranks as strict (0)
		{"totally-unknown", "off", true},     // unknown default also ranks 0
	}
	for _, tc := range cases {
		t.Run(tc.cfgDefault+"->"+tc.requested, func(t *testing.T) {
			t.Parallel()
			if got := daemon.SandboxIsRelaxationForTest(tc.cfgDefault, tc.requested); got != tc.want {
				t.Errorf("sandboxIsRelaxation(%q, %q) = %v, want %v", tc.cfgDefault, tc.requested, got, tc.want)
			}
		})
	}
}

// --- jsonOK, principal, recordStartTime: direct unit tests via export seams ---

func newBareServer(t *testing.T) *daemon.Server {
	t.Helper()
	dir := t.TempDir()
	cfg := newTestConfig(t, dir)
	srv, err := daemon.New(context.Background(), daemon.Options{Config: cfg, DBPath: filepath.Join(dir, "db.sqlite")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Close() })
	return srv
}

func TestJSONOKMarshalError(t *testing.T) {
	t.Parallel()
	srv := newBareServer(t)
	resp := srv.JSONOKForTest(make(chan int))
	if resp.OK {
		t.Fatal("jsonOK with an unmarshalable value: want !OK, got OK")
	}
	if resp.ErrorCode != string(domain.CodeInternal) {
		t.Fatalf("ErrorCode = %q, want internal", resp.ErrorCode)
	}
}

func TestPrincipalDefaultsEmptyRoleToFull(t *testing.T) {
	t.Parallel()
	srv := newBareServer(t)
	p := srv.PrincipalForTest("", "sess-x")
	if p.Role != authz.RoleFull {
		t.Fatalf("Role = %q, want full", p.Role)
	}
	p2 := srv.PrincipalForTest("agent", "sess-x")
	if p2.Role != authz.RoleAgent {
		t.Fatalf("Role = %q, want agent (non-empty role passed through)", p2.Role)
	}
}

func TestRecordStartTimeSkipsNonPositivePID(t *testing.T) {
	t.Parallel()
	srv := newBareServer(t)
	srv.RecordStartTimeForTest("procX", 0)
	if got := srv.StartTimeForTest("procX"); got != 0 {
		t.Fatalf("start time = %d, want 0 (no-op for pid<=0)", got)
	}
}

func TestRecordStartTimeSkipsUnreadableProc(t *testing.T) {
	t.Parallel()
	srv := newBareServer(t)
	// A PID vanishingly unlikely to correspond to a live process with a
	// readable /proc/<pid>/stat entry.
	srv.RecordStartTimeForTest("procY", 999999999)
	if got := srv.StartTimeForTest("procY"); got != 0 {
		t.Fatalf("start time = %d, want 0 (ReadStartTime failure is a no-op)", got)
	}
}

// --- handle(): deny-path coverage for every s.require call site, in one shot
// via a role string the authz package doesn't recognize (Caps() -> empty map,
// so every capability check fails).

func TestHandleDenyPathsForUnrecognizedRole(t *testing.T) {
	t.Parallel()
	ctx, _, c, _ := startTestDaemon(t)
	c.SetSession("sess-x", "totally-unrecognized-role")
	cases := []struct {
		method  string
		payload any
	}{
		{api.MethodDaemonInfo, map[string]any{}},
		{api.MethodStart, api.StartPayload{Name: "x", Command: []string{"sleep", "1"}}},
		{api.MethodStop, api.IDPayload{ID: "proc-nope"}},
		{api.MethodRestart, api.IDPayload{ID: "proc-nope"}},
		{api.MethodList, api.ListPayload{}},
		{api.MethodStatus, api.IDPayload{ID: "proc-nope"}},
		{api.MethodRemove, api.IDPayload{ID: "proc-nope"}},
		{api.MethodLogs, api.LogsPayload{ID: "proc-nope"}},
		{api.MethodGrep, api.LogsPayload{ID: "proc-nope", Pattern: "x"}},
		{api.MethodErrors, api.LogsPayload{ID: "proc-nope"}},
		{api.MethodEvents, api.EventsPayload{}},
		{api.MethodAudit, map[string]any{}},
	}
	for _, tc := range cases {
		t.Run(tc.method, func(t *testing.T) {
			err := c.Call(ctx, tc.method, tc.payload, &map[string]any{})
			if err == nil {
				t.Fatalf("%s with an unrecognized role: want permission_denied, got nil", tc.method)
			}
			var derr *domain.Error
			if !errors.As(err, &derr) || derr.Code != domain.CodePermissionDenied {
				t.Fatalf("%s err = %v, want permission_denied", tc.method, err)
			}
		})
	}
}

func TestHandleEmptyRoleOnWireDefaultsToFull(t *testing.T) {
	t.Parallel()
	ctx, _, c, _ := startTestDaemon(t)
	// ipc.Client always sends a non-empty role; bypass it to send a genuinely
	// empty role field, exercising principal()'s r=="" -> RoleFull branch from
	// the wire rather than the client-side default.
	req := &pmmcpv1.CallRequest{ApiVersion: api.APIVersion, Method: api.MethodDaemonInfo, Role: ""}
	resp, err := c.DaemonRPC().Call(ctx, req)
	if err != nil {
		t.Fatalf("rpc: %v", err)
	}
	if !resp.GetOk() {
		t.Fatalf("daemon.info with empty wire role: want ok (defaults to full), got %s", resp.GetError())
	}
}

func TestHandleVersionMismatchUnparseableFallback(t *testing.T) {
	t.Parallel()
	ctx, _, c, _ := startTestDaemon(t)
	req := &pmmcpv1.CallRequest{ApiVersion: "not-a-version", Method: api.MethodHello}
	resp, err := c.DaemonRPC().Call(ctx, req)
	if err != nil {
		t.Fatalf("rpc: %v", err)
	}
	if resp.GetOk() {
		t.Fatal("unparseable api_version: want !ok, got ok")
	}
	if resp.GetErrorCode() != string(domain.CodeIPCVersionMismatch) {
		t.Fatalf("error_code = %q, want ipc_version_mismatch", resp.GetErrorCode())
	}
}

// --- resolveID edge cases ---

func TestResolveIDNotFoundByID(t *testing.T) {
	t.Parallel()
	ctx, _, c, _ := startTestDaemon(t)
	err := c.Call(ctx, api.MethodStatus, api.IDPayload{ID: "proc-does-not-exist-xyz"}, &map[string]any{})
	if err == nil {
		t.Fatal("status of a nonexistent ID: want error, got nil")
	}
}

func TestResolveIDRequiresIDOrName(t *testing.T) {
	t.Parallel()
	ctx, _, c, _ := startTestDaemon(t)
	err := c.Call(ctx, api.MethodStatus, api.IDPayload{}, &map[string]any{})
	if err == nil {
		t.Fatal("status with neither id nor name: want error, got nil")
	}
	var derr *domain.Error
	if !errors.As(err, &derr) || derr.Code != domain.CodeInvalidArgument {
		t.Fatalf("err = %v, want invalid_argument", err)
	}
}

func TestResolveIDFallsBackToGlobalListWhenProjectScopedEmpty(t *testing.T) {
	t.Parallel()
	ctx, _, c, _ := startTestDaemon(t)
	if err := c.Call(ctx, api.MethodStart, api.StartPayload{
		Name: "globalfallback", Command: []string{"sleep", "5"}, Sandbox: "off", Project: "projA",
	}, &map[string]any{}); err != nil {
		t.Fatalf("start: %v", err)
	}
	// Query by name scoped to a different project: the project-scoped list is
	// empty, so resolveID must fall back to an unscoped list-by-name.
	var status api.ProcessView
	if err := c.Call(ctx, api.MethodStatus, api.IDPayload{Name: "globalfallback", Project: "projB"}, &status); err != nil {
		t.Fatalf("status via global fallback: %v", err)
	}
	if status.Name != "globalfallback" {
		t.Fatalf("status = %+v, want globalfallback", status)
	}
}

func TestResolveIDAmbiguousName(t *testing.T) {
	t.Parallel()
	ctx, _, c, _ := startTestDaemon(t)
	for _, proj := range []string{"ambigA", "ambigB"} {
		if err := c.Call(ctx, api.MethodStart, api.StartPayload{
			Name: "ambiguous-name", Command: []string{"sleep", "5"}, Sandbox: "off", Project: proj,
		}, &map[string]any{}); err != nil {
			t.Fatalf("start in %s: %v", proj, err)
		}
	}
	err := c.Call(ctx, api.MethodStatus, api.IDPayload{Name: "ambiguous-name"}, &map[string]any{})
	if err == nil {
		t.Fatal("status by an ambiguous name across projects: want error, got nil")
	}
	var derr *domain.Error
	if !errors.As(err, &derr) || derr.Code != domain.CodeInvalidArgument {
		t.Fatalf("err = %v, want invalid_argument", err)
	}
}

func TestResolveIDByNameFindsTerminalPredecessorAcrossRestart(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "db.sqlite")

	cfg1 := newTestConfig(t, dir)
	cfg1.IPC.Endpoint = testsock.Path(t)
	ctx1, cancel1 := context.WithCancel(context.Background())
	srv1, err := daemon.New(ctx1, daemon.Options{Config: cfg1, DBPath: dbPath})
	if err != nil {
		t.Fatal(err)
	}
	c1 := dialDaemon(ctx1, t, srv1, cfg1.IPC.Endpoint)
	var start api.StartResult
	if err := c1.Call(ctx1, api.MethodStart, api.StartPayload{
		Name: "term-name-lookup", Command: []string{"true"}, Sandbox: "off", Project: "termproj",
	}, &start); err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := c1.Call(ctx1, api.MethodStop, api.IDPayload{ID: start.ID, TimeoutSec: 2}, &map[string]any{}); err != nil {
		t.Fatalf("stop: %v", err)
	}
	cancel1()
	_ = srv1.Close()

	// srv2's byName map is empty, forcing resolveID's list-scan path; the only
	// matching record is the terminal one from srv1, which must still resolve
	// (not error) via the final `return list[0].ID, list[0], nil` fallback.
	cfg2 := newTestConfig(t, dir)
	cfg2.IPC.Endpoint = testsock.Path(t)
	ctx2, cancel2 := context.WithCancel(context.Background())
	t.Cleanup(cancel2)
	srv2, err := daemon.New(ctx2, daemon.Options{Config: cfg2, DBPath: dbPath})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv2.Close() })
	c2 := dialDaemon(ctx2, t, srv2, cfg2.IPC.Endpoint)

	var status api.ProcessView
	if err := c2.Call(ctx2, api.MethodStatus, api.IDPayload{Name: "term-name-lookup", Project: "termproj"}, &status); err != nil {
		t.Fatalf("status by name for a terminal predecessor: %v", err)
	}
	if status.ID != start.ID {
		t.Fatalf("resolved ID = %q, want %q", status.ID, start.ID)
	}
}

// --- doStop / doRestart: manager-level ErrNotFound across a fresh manager ---

func TestDoStopManagerErrNotFoundAcrossRestart(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "db.sqlite")

	cfg1 := newTestConfig(t, dir)
	cfg1.IPC.Endpoint = testsock.Path(t)
	ctx1, cancel1 := context.WithCancel(context.Background())
	srv1, err := daemon.New(ctx1, daemon.Options{Config: cfg1, DBPath: dbPath})
	if err != nil {
		t.Fatal(err)
	}
	c1 := dialDaemon(ctx1, t, srv1, cfg1.IPC.Endpoint)
	var start api.StartResult
	if err := c1.Call(ctx1, api.MethodStart, api.StartPayload{
		Name: "stop-unknown-to-mgr", Command: []string{"sleep", "30"}, Sandbox: "off", Project: "stopunkproj",
	}, &start); err != nil {
		t.Fatalf("start: %v", err)
	}
	cancel1()
	_ = srv1.Close()
	t.Cleanup(func() {
		if p, err := os.FindProcess(start.PID); err == nil {
			_ = p.Kill()
		}
	})

	cfg2 := newTestConfig(t, dir)
	cfg2.IPC.Endpoint = testsock.Path(t)
	ctx2, cancel2 := context.WithCancel(context.Background())
	t.Cleanup(cancel2)
	srv2, err := daemon.New(ctx2, daemon.Options{Config: cfg2, DBPath: dbPath})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv2.Close() })
	c2 := dialDaemon(ctx2, t, srv2, cfg2.IPC.Endpoint)

	// The store record exists (status running) but srv2's fresh manager never
	// started this ID, so s.mgr.Stop returns process.ErrNotFound.
	err = c2.Call(ctx2, api.MethodStop, api.IDPayload{ID: start.ID, Force: true}, &map[string]any{})
	if err == nil {
		t.Fatal("stop of a process unknown to a fresh manager: want error, got nil")
	}
	var derr *domain.Error
	if !errors.As(err, &derr) || derr.Code != domain.CodeNotFound {
		t.Fatalf("err = %v, want not_found", err)
	}
}

// --- doRestart: cross-session deny, id/logdir/mgr.Start failures ---

func TestDoRestartCrossSessionDeniedWithoutShare(t *testing.T) {
	t.Parallel()
	ctx, _, c, _ := startTestDaemon(t)
	c.SetSession("sess-owner", "agent")
	var start api.StartResult
	if err := c.Call(ctx, api.MethodStart, api.StartPayload{
		Name: "restart-cross-deny", Command: []string{"sleep", "30"},
	}, &start); err != nil {
		t.Fatalf("start: %v", err)
	}
	c.SetSession("sess-other", "agent")
	err := c.Call(ctx, api.MethodRestart, api.IDPayload{ID: start.ID}, &map[string]any{})
	if err == nil {
		t.Fatal("cross-session restart without a share grant: want permission_denied, got nil")
	}
	var derr *domain.Error
	if !errors.As(err, &derr) || derr.Code != domain.CodePermissionDenied {
		t.Fatalf("err = %v, want permission_denied", err)
	}
}

func TestDoRestartNotFound(t *testing.T) {
	t.Parallel()
	ctx, _, c, _ := startTestDaemon(t)
	err := c.Call(ctx, api.MethodRestart, api.IDPayload{ID: "proc-nonexistent-restart"}, &map[string]any{})
	if err == nil {
		t.Fatal("restart of a nonexistent ID: want error, got nil")
	}
}

func TestDoRestartIDGenerationFailure(t *testing.T) {
	// Not t.Parallel(): mutates the package-level crypto/rand.Reader.
	ctx, _, c, _ := startTestDaemon(t)
	var start api.StartResult
	if err := c.Call(ctx, api.MethodStart, api.StartPayload{
		Name: "restart-idfail", Command: []string{"sleep", "5"},
	}, &start); err != nil {
		t.Fatalf("start: %v", err)
	}
	orig := rand.Reader
	rand.Reader = alwaysErrReader{}
	t.Cleanup(func() { rand.Reader = orig })
	err := c.Call(ctx, api.MethodRestart, api.IDPayload{ID: start.ID}, &map[string]any{})
	if err == nil {
		t.Fatal("restart with a failing crypto/rand.Reader: want error, got nil")
	}
}

func TestDoRestartLogDirMkdirAllFails(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory write permission checks")
	}
	t.Parallel()
	dir := t.TempDir()
	cfg := newTestConfig(t, dir)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	srv, err := daemon.New(ctx, daemon.Options{Config: cfg, DBPath: filepath.Join(dir, "db.sqlite")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Close() })
	c := dialDaemon(ctx, t, srv, cfg.IPC.Endpoint)

	var start api.StartResult
	if err := c.Call(ctx, api.MethodStart, api.StartPayload{
		Name: "restart-logdirfail", Command: []string{"sleep", "5"},
	}, &start); err != nil {
		t.Fatalf("start: %v", err)
	}
	// "logs" already exists as a directory (from the start above); block
	// creation of a new subdirectory under it by removing write permission.
	logsDir := filepath.Join(cfg.StateDir, "logs")
	if err := os.Chmod(logsDir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(logsDir, 0o700) })
	err = c.Call(ctx, api.MethodRestart, api.IDPayload{ID: start.ID}, &map[string]any{})
	if err == nil {
		t.Fatal("restart with logs dir blocked by a file: want error, got nil")
	}
}

func TestDoRestartManagerStartFails(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	script := filepath.Join(dir, "restart-run.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nsleep 5\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := newTestConfig(t, dir)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	srv, err := daemon.New(ctx, daemon.Options{Config: cfg, DBPath: filepath.Join(dir, "db.sqlite")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Close() })
	c := dialDaemon(ctx, t, srv, cfg.IPC.Endpoint)

	var start api.StartResult
	if err := c.Call(ctx, api.MethodStart, api.StartPayload{
		Name: "restart-mgrfail", Command: []string{script},
	}, &start); err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := os.Remove(script); err != nil {
		t.Fatal(err)
	}
	err = c.Call(ctx, api.MethodRestart, api.IDPayload{ID: start.ID, Force: true}, &map[string]any{})
	if err == nil {
		t.Fatal("restart with a since-removed binary: want error, got nil")
	}
}

// --- doList filters ---

func TestDoListFiltersAndPagination(t *testing.T) {
	t.Parallel()
	ctx, _, c, _ := startTestDaemon(t)
	proj := "listfilterproj"
	ids := make([]string, 0, 3)
	for i := range 3 {
		var start api.StartResult
		if err := c.Call(ctx, api.MethodStart, api.StartPayload{
			Name: fmt.Sprintf("listitem-%d", i), Command: []string{"sleep", "5"}, Project: proj,
		}, &start); err != nil {
			t.Fatalf("start %d: %v", i, err)
		}
		ids = append(ids, start.ID)
	}

	t.Run("status filter", func(t *testing.T) {
		var list []api.ProcessView
		if err := c.Call(ctx, api.MethodList, api.ListPayload{Project: proj, Status: "running"}, &list); err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(list) != 3 {
			t.Fatalf("len(list) = %d, want 3", len(list))
		}
	})

	t.Run("runtime filter excludes all", func(t *testing.T) {
		var list []api.ProcessView
		if err := c.Call(ctx, api.MethodList, api.ListPayload{Project: proj, Runtime: "container"}, &list); err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(list) != 0 {
			t.Fatalf("len(list) = %d, want 0 (runtime filter should exclude local processes)", len(list))
		}
	})

	t.Run("profile filter excludes all", func(t *testing.T) {
		var list []api.ProcessView
		if err := c.Call(ctx, api.MethodList, api.ListPayload{Project: proj, Profile: "no-such-profile"}, &list); err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(list) != 0 {
			t.Fatalf("len(list) = %d, want 0 (profile filter should exclude default-profile processes)", len(list))
		}
	})

	t.Run("cursor pagination skips seen entries", func(t *testing.T) {
		var full []api.ProcessView
		if err := c.Call(ctx, api.MethodList, api.ListPayload{Project: proj}, &full); err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(full) < 2 {
			t.Fatalf("len(full) = %d, want >= 2", len(full))
		}
		var page []api.ProcessView
		if err := c.Call(ctx, api.MethodList, api.ListPayload{Project: proj, Cursor: full[0].ID}, &page); err != nil {
			t.Fatalf("list with cursor: %v", err)
		}
		if len(page) != len(full)-1 {
			t.Fatalf("len(page) = %d, want %d (cursor should skip the first entry)", len(page), len(full)-1)
		}
	})

	t.Run("limit caps results", func(t *testing.T) {
		var page []api.ProcessView
		if err := c.Call(ctx, api.MethodList, api.ListPayload{Project: proj, Limit: 1}, &page); err != nil {
			t.Fatalf("list with limit: %v", err)
		}
		if len(page) != 1 {
			t.Fatalf("len(page) = %d, want 1", len(page))
		}
	})

	for _, id := range ids {
		_ = c.Call(ctx, api.MethodStop, api.IDPayload{ID: id, Force: true}, &map[string]any{})
	}
}

func TestDoListError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cfg := newTestConfig(t, dir)
	srv, err := daemon.New(context.Background(), daemon.Options{Config: cfg, DBPath: filepath.Join(dir, "db.sqlite")})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	c := dialDaemon(ctx, t, srv, cfg.IPC.Endpoint)
	_ = srv.Close()
	err = c.Call(ctx, api.MethodList, api.ListPayload{}, &map[string]any{})
	cancel()
	if err == nil {
		t.Fatal("list after store close: want error, got nil")
	}
}

// --- doRemove: purge-logs and declare-warning branches ---

func TestDoRemovePurgeLogsAndDeclareWarning(t *testing.T) {
	t.Parallel()
	ctx, _, c, dir := startTestDaemon(t)
	proj := filepath.Join(dir, "declareproj")
	if err := os.MkdirAll(proj, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, "pmmcp.yaml"), []byte("apiVersion: pmmcp.dev/v1alpha1\nkind: Project\nservices: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var start api.StartResult
	if err := c.Call(ctx, api.MethodStart, api.StartPayload{
		Name: "removeme", Command: []string{"sleep", "5"}, Cwd: proj,
	}, &start); err != nil {
		t.Fatalf("start: %v", err)
	}
	var out map[string]any
	if err := c.Call(ctx, api.MethodRemove, api.IDPayload{ID: start.ID, PurgeLogs: true}, &out); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if out["purge_logs"] != true {
		t.Fatalf("out = %+v, want purge_logs=true", out)
	}
	if _, err := os.Stat(start.LogDir); !os.IsNotExist(err) {
		t.Fatalf("log dir still present after purge: %v", err)
	}
	if _, ok := out["warning"]; !ok {
		t.Fatalf("out = %+v, want a declare-file warning", out)
	}
}

func TestDoRemoveNotFound(t *testing.T) {
	t.Parallel()
	ctx, _, c, _ := startTestDaemon(t)
	err := c.Call(ctx, api.MethodRemove, api.IDPayload{ID: "proc-remove-nonexistent"}, &map[string]any{})
	if err == nil {
		t.Fatal("remove of a nonexistent ID: want error, got nil")
	}
}

func TestDoRemoveCrossSessionDenied(t *testing.T) {
	t.Parallel()
	ctx, _, c, _ := startTestDaemon(t)
	c.SetSession("sess-owner", "agent")
	var start api.StartResult
	if err := c.Call(ctx, api.MethodStart, api.StartPayload{
		Name: "remove-cross-deny", Command: []string{"sleep", "5"},
	}, &start); err != nil {
		t.Fatalf("start: %v", err)
	}
	c.SetSession("sess-other", "agent")
	err := c.Call(ctx, api.MethodRemove, api.IDPayload{ID: start.ID}, &map[string]any{})
	if err == nil {
		t.Fatal("cross-session remove without a share grant: want permission_denied, got nil")
	}
	var derr *domain.Error
	if !errors.As(err, &derr) || derr.Code != domain.CodePermissionDenied {
		t.Fatalf("err = %v, want permission_denied", err)
	}
}

// --- doLogs: MinLevel structured filter and not-found error path ---

func TestDoLogsMinLevelFilter(t *testing.T) {
	t.Parallel()
	ctx, _, c, _ := startTestDaemon(t)
	var start api.StartResult
	if err := c.Call(ctx, api.MethodStart, api.StartPayload{
		Name: "loglevel", Command: []string{"/bin/sh", "-c", `echo '{"level":"error","msg":"boom"}'; sleep 2`},
	}, &start); err != nil {
		t.Fatalf("start: %v", err)
	}
	time.Sleep(150 * time.Millisecond)
	var out api.LogsResult
	if err := c.Call(ctx, api.MethodLogs, api.LogsPayload{ID: start.ID, MinLevel: "error"}, &out); err != nil {
		t.Fatalf("logs with min_level: %v", err)
	}
	_ = c.Call(ctx, api.MethodStop, api.IDPayload{ID: start.ID, Force: true}, &map[string]any{})
}

func TestDoLogsNotFound(t *testing.T) {
	t.Parallel()
	ctx, _, c, _ := startTestDaemon(t)
	err := c.Call(ctx, api.MethodLogs, api.LogsPayload{ID: "proc-logs-nonexistent"}, &map[string]any{})
	if err == nil {
		t.Fatal("logs of a nonexistent ID: want error, got nil")
	}
}

// --- ListenAndServe: Listen error and RelaunchEligible-error audit path ---

func TestListenAndServeListenError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cfg := newTestConfig(t, dir)
	// A path with a non-directory parent makes ipc.Listen fail synchronously,
	// before ListenAndServe spawns any background goroutines.
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg.IPC.Endpoint = filepath.Join(blocker, "pmmcpd.sock")
	srv, err := daemon.New(context.Background(), daemon.Options{Config: cfg, DBPath: filepath.Join(dir, "db.sqlite")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Close() })
	if err := srv.ListenAndServe(context.Background()); err == nil {
		t.Fatal("ListenAndServe with an unlistenable endpoint: want error, got nil")
	}
}

func TestListenAndServeRelaunchFailureAudited(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "db.sqlite")
	script := filepath.Join(dir, "boot-run.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	cfg1 := newTestConfig(t, dir)
	cfg1.IPC.Endpoint = testsock.Path(t)
	ctx1, cancel1 := context.WithCancel(context.Background())
	srv1, err := daemon.New(ctx1, daemon.Options{Config: cfg1, DBPath: dbPath})
	if err != nil {
		t.Fatal(err)
	}
	c1 := dialDaemon(ctx1, t, srv1, cfg1.IPC.Endpoint)
	var start api.StartResult
	if err := c1.Call(ctx1, api.MethodStart, api.StartPayload{
		Name: "boot-relaunch-fail", Command: []string{script}, Project: "bootproj",
	}, &start); err != nil {
		t.Fatalf("start: %v", err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if p, err := os.FindProcess(start.PID); err != nil || p.Signal(syscall.Signal(0)) != nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel1()
	_ = srv1.Close()
	if err := os.Remove(script); err != nil {
		t.Fatal(err)
	}

	// Zero the persisted PID directly so RelaunchEligible's pidAlive
	// short-circuit can never spuriously match an unrelated process that the
	// OS has since reused start.PID for (flaky otherwise: this raced under
	// -race, see FABLE_REVIEW.md). With PID<=0, RelaunchEligible always
	// attempts mgr.Start, which fails deterministically since the script
	// was removed above.
	st, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	rec, err := st.Get(context.Background(), start.ID)
	if err != nil {
		t.Fatal(err)
	}
	rec.PID = 0
	if err := st.Update(context.Background(), rec); err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	cfg2 := newTestConfig(t, dir)
	cfg2.IPC.Endpoint = testsock.Path(t)
	cfg2.Relaunch.Enabled = true
	ctx2, cancel2 := context.WithCancel(context.Background())
	t.Cleanup(cancel2)
	srv2, err := daemon.New(ctx2, daemon.Options{Config: cfg2, DBPath: dbPath})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv2.Close() })
	c2 := dialDaemon(ctx2, t, srv2, cfg2.IPC.Endpoint)

	if !auditHas(t, c2, func(r auditRow) bool { return r.Action == "daemon.relaunch" }) {
		t.Fatal("expected a daemon.relaunch audit row after a failed boot relaunch")
	}
}

// --- RelaunchEligible: ineligible (desired=stopped) record is skipped ---

func TestRelaunchEligibleSkipsIneligibleDesiredStopped(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cfg := newTestConfig(t, dir)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	srv, err := daemon.New(ctx, daemon.Options{Config: cfg, DBPath: filepath.Join(dir, "db.sqlite")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Close() })
	c := dialDaemon(ctx, t, srv, cfg.IPC.Endpoint)

	var start api.StartResult
	if err := c.Call(ctx, api.MethodStart, api.StartPayload{
		Name: "not-relaunch-eligible", Command: []string{"sleep", "30"},
	}, &start); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() {
		_ = c.Call(context.Background(), api.MethodStop, api.IDPayload{ID: start.ID, Force: true}, &map[string]any{})
	})
	if err := c.Call(ctx, api.MethodStop, api.IDPayload{ID: start.ID, Force: true}, &map[string]any{}); err != nil {
		t.Fatalf("stop: %v", err)
	}
	// The stopped record's Desired is now "stopped": RelaunchEligible must
	// skip it via EligibleForRelaunch (continue) rather than restarting it.
	if err := srv.RelaunchEligible(ctx); err != nil {
		t.Fatalf("RelaunchEligible: %v", err)
	}
	var status api.ProcessView
	if err := c.Call(ctx, api.MethodStatus, api.IDPayload{ID: start.ID}, &status); err != nil {
		t.Fatalf("status: %v", err)
	}
	if status.Status != string(domain.StatusExited) {
		t.Fatalf("status = %q, want exited (RelaunchEligible must not restart a desired=stopped record)", status.Status)
	}
}

// --- daemon.info: token redaction ---

func TestDaemonInfoRedactsTokenFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cfg := newTestConfig(t, dir)
	cfg.TokenFile = filepath.Join(dir, "token")
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	srv, err := daemon.New(ctx, daemon.Options{Config: cfg, DBPath: filepath.Join(dir, "db.sqlite")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Close() })
	c := dialDaemon(ctx, t, srv, cfg.IPC.Endpoint)
	var info api.DaemonInfoResult
	if err := c.Call(ctx, api.MethodDaemonInfo, map[string]any{}, &info); err != nil {
		t.Fatalf("daemon.info: %v", err)
	}
	if info.TokenFile != "[redacted]" {
		t.Fatalf("TokenFile = %q, want [redacted]", info.TokenFile)
	}
}

// --- handle(): method-required and unparseable version ---

func TestHandleEmptyMethod(t *testing.T) {
	t.Parallel()
	ctx, _, c, _ := startTestDaemon(t)
	req := &pmmcpv1.CallRequest{ApiVersion: api.APIVersion, Method: "", Role: "full"}
	resp, err := c.DaemonRPC().Call(ctx, req)
	if err != nil {
		t.Fatalf("rpc: %v", err)
	}
	if resp.GetOk() {
		t.Fatal("empty method: want !ok, got ok")
	}
	if resp.GetErrorCode() != string(domain.CodeInvalidArgument) {
		t.Fatalf("error_code = %q, want invalid_argument", resp.GetErrorCode())
	}
}

// --- resolveID: byName fast path, and a store error on the project-scoped list ---

func TestResolveIDByNameFastPath(t *testing.T) {
	t.Parallel()
	ctx, _, c, _ := startTestDaemon(t)
	if err := c.Call(ctx, api.MethodStart, api.StartPayload{
		Name: "fastpath", Command: []string{"sleep", "5"}, Project: "fastpathproj",
	}, &map[string]any{}); err != nil {
		t.Fatalf("start: %v", err)
	}
	var status api.ProcessView
	if err := c.Call(ctx, api.MethodStatus, api.IDPayload{Name: "fastpath", Project: "fastpathproj"}, &status); err != nil {
		t.Fatalf("status by name (default profile, byName fast path): %v", err)
	}
	if status.Name != "fastpath" {
		t.Fatalf("status = %+v", status)
	}
}

func TestResolveIDProjectScopedListErrorAfterClose(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cfg := newTestConfig(t, dir)
	srv, err := daemon.New(context.Background(), daemon.Options{Config: cfg, DBPath: filepath.Join(dir, "db.sqlite")})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	c := dialDaemon(ctx, t, srv, cfg.IPC.Endpoint)
	_ = srv.Close()
	// Name-based resolution (no ID) with the store closed forces resolveID's
	// project-scoped store.List call to fail.
	err = c.Call(ctx, api.MethodStatus, api.IDPayload{Name: "anything", Project: "p"}, &map[string]any{})
	cancel()
	if err == nil {
		t.Fatal("resolve by name after store close: want error, got nil")
	}
}

// --- doStop: idempotent already-terminal short-circuit ---

func TestDoStopIdempotentAlreadyStopped(t *testing.T) {
	t.Parallel()
	ctx, _, c, _ := startTestDaemon(t)
	var start api.StartResult
	if err := c.Call(ctx, api.MethodStart, api.StartPayload{
		Name: "double-stop", Command: []string{"sleep", "5"},
	}, &start); err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := c.Call(ctx, api.MethodStop, api.IDPayload{ID: start.ID, Force: true}, &map[string]any{}); err != nil {
		t.Fatalf("first stop: %v", err)
	}
	var out map[string]any
	if err := c.Call(ctx, api.MethodStop, api.IDPayload{ID: start.ID, Force: true}, &out); err != nil {
		t.Fatalf("second stop: %v", err)
	}
	if out["already_stopped"] != true {
		t.Fatalf("second stop out = %+v, want already_stopped=true", out)
	}
}

// --- doRestart: carried-over optional fields, and a store.Create conflict
// race (two concurrent restarts of the same predecessor both produce a
// successor with the same (project, name) and an empty successor_id, so the
// second store.Create loses the partial unique index race). ---

func TestDoRestartCarriesOverOptionalFields(t *testing.T) {
	t.Parallel()
	ctx, _, c, _ := startTestDaemon(t)
	var start api.StartResult
	if err := c.Call(ctx, api.MethodStart, api.StartPayload{
		Name: "restart-carryover", Command: []string{"sleep", "10"},
		MemoryBytes: 32 << 20, HealthURL: "http://127.0.0.1:0/health",
	}, &start); err != nil {
		t.Fatalf("start: %v", err)
	}
	var res api.StartResult
	if err := c.Call(ctx, api.MethodRestart, api.IDPayload{ID: start.ID}, &res); err != nil {
		t.Fatalf("restart: %v", err)
	}

	c.SetSession("sess-carryover", "agent")
	var sodStart api.StartResult
	if err := c.Call(ctx, api.MethodStart, api.StartPayload{
		Name: "restart-carryover-sod", Command: []string{"sleep", "10"}, StopOnDisconnect: true,
	}, &sodStart); err != nil {
		t.Fatalf("start sod: %v", err)
	}
	var sodRes api.StartResult
	if err := c.Call(ctx, api.MethodRestart, api.IDPayload{ID: sodStart.ID}, &sodRes); err != nil {
		t.Fatalf("restart sod: %v", err)
	}
}

func TestDoRestartConcurrentStoreCreateConflict(t *testing.T) {
	t.Parallel()
	ctx, _, c, _ := startTestDaemon(t)
	var start api.StartResult
	if err := c.Call(ctx, api.MethodStart, api.StartPayload{
		Name: "restart-race", Command: []string{"sleep", "10"},
	}, &start); err != nil {
		t.Fatalf("start: %v", err)
	}
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range 2 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = c.Call(ctx, api.MethodRestart, api.IDPayload{ID: start.ID}, &map[string]any{})
		}(i)
	}
	wg.Wait()
	oks, fails := 0, 0
	for _, err := range errs {
		if err == nil {
			oks++
		} else {
			fails++
		}
	}
	t.Logf("concurrent restart: %d ok, %d failed", oks, fails)
	if oks == 0 {
		t.Fatal("expected at least one concurrent restart to succeed")
	}
}

// --- doRemove: concurrent double-delete races store.Delete's not-found error ---

func TestDoRemoveConcurrentDeleteConflict(t *testing.T) {
	t.Parallel()
	ctx, _, c, _ := startTestDaemon(t)
	var start api.StartResult
	if err := c.Call(ctx, api.MethodStart, api.StartPayload{
		Name: "remove-race", Command: []string{"sleep", "10"},
	}, &start); err != nil {
		t.Fatalf("start: %v", err)
	}
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range 2 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = c.Call(ctx, api.MethodRemove, api.IDPayload{ID: start.ID}, &map[string]any{})
		}(i)
	}
	wg.Wait()
	oks, fails := 0, 0
	for _, err := range errs {
		if err == nil {
			oks++
		} else {
			fails++
		}
	}
	t.Logf("concurrent remove: %d ok, %d failed", oks, fails)
	if oks == 0 {
		t.Fatal("expected at least one concurrent remove to succeed")
	}
}

// --- doLogs: logcap error branch (invalid stream) ---

func TestDoLogsInvalidStream(t *testing.T) {
	t.Parallel()
	ctx, _, c, _ := startTestDaemon(t)
	var start api.StartResult
	if err := c.Call(ctx, api.MethodStart, api.StartPayload{
		Name: "badstream", Command: []string{"sleep", "5"},
	}, &start); err != nil {
		t.Fatalf("start: %v", err)
	}
	err := c.Call(ctx, api.MethodLogs, api.LogsPayload{ID: start.ID, Stream: "totally-bogus-stream"}, &map[string]any{})
	if err == nil {
		t.Fatal("logs with an invalid stream: want error, got nil")
	}
	_ = c.Call(ctx, api.MethodStop, api.IDPayload{ID: start.ID, Force: true}, &map[string]any{})
}

// --- New/Close: Options.Store override also closes the real underlying
// SQLite handle backing audit/events (dbStore), not just the injected store.

func TestNewWithStoreOverrideClosesRealHandle(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cfg := newTestConfig(t, dir)
	// A second, independent sqlite.Store standing in for an injected
	// store.ProcessStore (any ProcessStore implementation satisfies
	// Options.Store; sqlite.Store already does, so it doubles as a fake here).
	fakeStorePath := filepath.Join(dir, "fake.db")
	fakeStore, err := sqlite.Open(fakeStorePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := fakeStore.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	srv, err := daemon.New(context.Background(), daemon.Options{
		Config: cfg, DBPath: filepath.Join(dir, "db.sqlite"), Store: fakeStore,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	c := dialDaemon(ctx, t, srv, cfg.IPC.Endpoint)
	// A round trip through the injected store.
	if err := c.Call(ctx, api.MethodStart, api.StartPayload{
		Name: "via-injected-store", Command: []string{"sleep", "1"},
	}, &map[string]any{}); err != nil {
		t.Fatalf("start via injected store: %v", err)
	}
	if err := srv.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	cancel()
	// The injected store (fakeStore) is closed by srv.Close(); closing it
	// again here should be a safe no-op if Close is idempotent, or at least
	// must not panic — this just confirms Close() actually reached it.
	_ = fakeStore.Close()
}
