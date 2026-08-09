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
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/scrothers/pmmcp/internal/api"
	pmmcpv1 "github.com/scrothers/pmmcp/internal/api/gen/pmmcp/v1"
	"github.com/scrothers/pmmcp/internal/config"
	"github.com/scrothers/pmmcp/internal/daemon"
	"github.com/scrothers/pmmcp/internal/domain"
	"github.com/scrothers/pmmcp/internal/ipc"
	"github.com/scrothers/pmmcp/internal/process"
	"github.com/scrothers/pmmcp/internal/testsock"
	"github.com/scrothers/pmmcp/internal/webhook"
)

// fakeMgr is a hermetic process.Manager: it records StartSpecs and reports
// processes running, so tests exercise daemon logic without spawning or bwrap.
type fakeMgr struct {
	mu      sync.Mutex
	specs   []process.StartSpec
	handles map[string]*process.Handle
}

func newFakeMgr() *fakeMgr {
	return &fakeMgr{handles: make(map[string]*process.Handle)}
}

func (m *fakeMgr) Start(_ context.Context, spec process.StartSpec) (*process.Handle, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.specs = append(m.specs, spec)
	h := &process.Handle{ID: spec.ID, PID: 4242, Status: domain.StatusRunning}
	m.handles[spec.ID] = h
	return &process.Handle{ID: spec.ID, PID: 4242, Status: domain.StatusRunning}, nil
}

func (m *fakeMgr) Stop(_ context.Context, id string, _ time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if h, ok := m.handles[id]; ok {
		code := 0
		h.Status = domain.StatusExited
		h.ExitCode = &code
	}
	return nil
}

func (m *fakeMgr) Wait(_ context.Context, id string) (*process.Handle, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	h, ok := m.handles[id]
	if !ok {
		return nil, process.ErrNotFound
	}
	cp := *h
	return &cp, nil
}

func (m *fakeMgr) Inspect(_ context.Context, id string) (*process.Handle, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	h, ok := m.handles[id]
	if !ok {
		return nil, process.ErrNotFound
	}
	cp := *h
	return &cp, nil
}

func (m *fakeMgr) Signal(_ context.Context, _ string, _ os.Signal) error { return nil }

func (m *fakeMgr) sandboxFor(name string) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, s := range m.specs {
		if s.Name == name {
			return s.Sandbox, true
		}
	}
	return "", false
}

// newTestDaemon starts a daemon with a fake manager and returns a connected
// client. tweak may adjust the config before New.
func newTestDaemon(t *testing.T, tweak func(*config.Config)) (*ipc.Client, *fakeMgr) {
	return newTestDaemonOpts(t, tweak, nil)
}

// newTestDaemonOpts is newTestDaemon plus an Options mutator for injecting test
// seams (e.g. WebhookDeliver, WebhookPoll).
func newTestDaemonOpts(t *testing.T, tweak func(*config.Config), optsTweak func(*daemon.Options)) (*ipc.Client, *fakeMgr) {
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
	cfg.Relaunch.Enabled = false
	if tweak != nil {
		tweak(cfg)
	}
	mgr := newFakeMgr()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	opts := daemon.Options{
		Config: cfg, DBPath: sqliteDBPathForTest(t), Manager: mgr,
		// Fast test clocks: same code paths as production, just quicker ticks
		// and follow windows so handler tests don't wait out second-scale
		// production intervals.
		AutoRestartTick:     25 * time.Millisecond,
		AutoRestartBackoff:  5 * time.Millisecond,
		WebhookRetryBackoff: 5 * time.Millisecond,
		LogsPreviewFollow:   100 * time.Millisecond,
	}
	if optsTweak != nil {
		optsTweak(&opts)
	}
	srv, err := daemon.New(ctx, opts)
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
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c, mgr
}

// TestDeclareApplyUsesConfigDefaultSandbox is the CRITICAL 2 regression: an
// applied service that omits sandbox must inherit cfg.Sandbox.Default (strict),
// never a silent "off".
func TestDeclareApplyUsesConfigDefaultSandbox(t *testing.T) {
	t.Parallel()
	const doc = `apiVersion: pmmcp.dev/v1alpha1
kind: Project
services:
  web:
    argv: ["sleep", "1"]
`
	t.Run("full role starts strict not off", func(t *testing.T) {
		c, mgr := newTestDaemon(t, func(cfg *config.Config) { cfg.Sandbox.Default = "strict" })
		c.SetSession("sess-full", "full")
		ctx := context.Background()
		var out map[string]any
		if err := c.Call(ctx, api.MethodApply, api.DeclarePayload{YAML: doc}, &out); err != nil {
			t.Fatalf("apply: %v", err)
		}
		sb, ok := mgr.sandboxFor("web")
		if !ok {
			t.Fatal("web service was never started")
		}
		if sb != "strict" {
			t.Fatalf("applied sandbox = %q, want strict (config default), never off", sb)
		}
	})

	t.Run("agent role succeeds not permission_denied", func(t *testing.T) {
		c, mgr := newTestDaemon(t, func(cfg *config.Config) { cfg.Sandbox.Default = "strict" })
		c.SetSession("sess-agent", "agent")
		ctx := context.Background()
		var out map[string]any
		if err := c.Call(ctx, api.MethodApply, api.DeclarePayload{YAML: doc}, &out); err != nil {
			t.Fatalf("agent apply must succeed (empty sandbox is not a relaxation), got: %v", err)
		}
		if sb, ok := mgr.sandboxFor("web"); !ok || sb != "strict" {
			t.Fatalf("agent applied sandbox = %q ok=%v, want strict", sb, ok)
		}
	})
}

// TestRestartWritesAuditRow is the HIGH regression: process.restart must audit.
func TestRestartWritesAuditRow(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, func(cfg *config.Config) { cfg.Sandbox.Default = "off" })
	c.SetSession("sess-full", "full")
	ctx := context.Background()
	var start api.StartResult
	if err := c.Call(ctx, api.MethodStart, api.StartPayload{
		Name: "svc", Command: []string{"sleep", "1"}, Sandbox: "off",
	}, &start); err != nil {
		t.Fatalf("start: %v", err)
	}
	var res api.StartResult
	if err := c.Call(ctx, api.MethodRestart, api.IDPayload{ID: start.ID}, &res); err != nil {
		t.Fatalf("restart: %v", err)
	}
	if !auditHas(t, c, func(r auditRow) bool { return r.Action == "process.restart" }) {
		t.Fatal("no process.restart audit row after restart")
	}
}

// TestExtendedMutationsDenyReadonly is the CRITICAL 1 regression: previously
// ungated mutating handlers must reject a readonly principal.
func TestExtendedMutationsDenyReadonly(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, func(cfg *config.Config) {
		cfg.Sandbox.Default = "off"
		cfg.Webhook.Allowlist = []string{"*.example.com"}
	})
	c.SetSession("sess-ro", "readonly")
	ctx := context.Background()
	cases := []struct {
		method  string
		payload any
	}{
		{api.MethodSecretSet, api.SecretPayload{Name: "k", Value: "v"}},
		{api.MethodWebhookCreate, api.WebhookPayload{URL: "https://hooks.example.com/x"}},
		{api.MethodGroupCreate, api.GroupPayload{Name: "g", Members: []api.GroupMemberPayload{{Name: "a"}}}},
		{api.MethodProfileCreate, api.ProfilePayload{Name: "p"}},
		{api.MethodWatchSet, api.WatchPayload{ID: "proc-x", Path: "/tmp/x"}},
		{api.MethodShare, api.SharePayload{Target: "proc-x", ToSession: "s2", Cap: "process:stop"}},
		{api.MethodApply, api.DeclarePayload{YAML: "apiVersion: pmmcp.dev/v1alpha1\nkind: Project\nservices: {}\n"}},
	}
	for _, tc := range cases {
		t.Run(tc.method, func(t *testing.T) {
			err := c.Call(ctx, tc.method, tc.payload, &map[string]any{})
			if err == nil {
				t.Fatalf("%s: readonly must be denied, got nil error", tc.method)
			}
			if !strings.Contains(err.Error(), "permission") {
				t.Fatalf("%s: want permission_denied, got %v", tc.method, err)
			}
		})
	}
}

// TestAuthzDenialsAreAudited is the Medium regression: every deny writes an
// audit row with the denied outcome.
func TestAuthzDenialsAreAudited(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, func(cfg *config.Config) { cfg.Sandbox.Default = "off" })
	ctx := context.Background()
	// readonly attempts a mutation → denied.
	c.SetSession("sess-ro", "readonly")
	_ = c.Call(ctx, api.MethodSecretSet, api.SecretPayload{Name: "k", Value: "v"}, &map[string]any{})
	// full reads the audit trail.
	c.SetSession("sess-full", "full")
	if !auditHas(t, c, func(r auditRow) bool {
		return r.Outcome == "denied" && r.Capability == "secrets:write"
	}) {
		t.Fatal("denied secret.set was not audited with outcome=denied capability=secrets:write")
	}
}

// TestVersionSkewFailsClosed is the HIGH regression: the daemon must reject a
// skewed or empty api_version with ipc_version_mismatch (numeric compare).
func TestVersionSkewFailsClosed(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, nil)
	ctx := context.Background()
	rpc := c.DaemonRPC()
	for _, ver := range []string{"2.0", "", "10.0"} {
		resp, err := rpc.Call(ctx, &pmmcpv1.CallRequest{ApiVersion: ver, Method: api.MethodList, Role: "full"})
		if err != nil {
			t.Fatalf("rpc(%q): transport error %v", ver, err)
		}
		if resp.GetOk() {
			t.Fatalf("api_version %q: want failure, got ok", ver)
		}
		if resp.GetErrorCode() != string(domain.CodeIPCVersionMismatch) {
			t.Fatalf("api_version %q: code = %q, want %s", ver, resp.GetErrorCode(), domain.CodeIPCVersionMismatch)
		}
	}
}

// TestSubscribeLogsRequiresLogsRead is the HIGH regression: streaming is gated
// on logs:read; a readonly principal (no logs:read) is rejected before streaming.
func TestSubscribeLogsRequiresLogsRead(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, nil)
	c.SetSession("sess-ro", "readonly")
	ctx := context.Background()
	stream, err := c.SubscribeLogs(ctx, "proc-anything", "both", 5)
	if err == nil {
		_, err = stream.Recv()
	}
	if err == nil {
		t.Fatal("readonly SubscribeLogs: want permission denied, got nil")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "permission") {
		t.Fatalf("want permission denied, got %v", err)
	}
}

// TestSessionEndCrossSessionRequiresProcessStop is the CRITICAL 1 regression:
// session.end that would stop a session's stop-on-disconnect processes is a
// de-facto process:stop. Because the session id is client-asserted, a readonly
// principal can assert a victim's session id — but it lacks process:stop, so the
// teardown must be denied rather than silently killing the victim's processes.
func TestSessionEndCrossSessionRequiresProcessStop(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, func(cfg *config.Config) { cfg.Sandbox.Default = "off" })
	ctx := context.Background()
	// A full-role owner starts a stop-on-disconnect process under session sess-A.
	c.SetSession("sess-A", "full")
	if err := c.Call(ctx, api.MethodStart, api.StartPayload{
		Name: "victim", Command: []string{"sleep", "1"}, Sandbox: "off", StopOnDisconnect: true,
	}, &api.StartResult{}); err != nil {
		t.Fatalf("start: %v", err)
	}
	// A readonly principal asserts the same session id and tries to end it,
	// which would stop the SOD process — must be denied (lacks process:stop).
	c.SetSession("sess-A", "readonly")
	err := c.Call(ctx, api.MethodSessionEnd, map[string]string{"id": "sess-A"}, &map[string]any{})
	if err == nil {
		t.Fatal("readonly session.end that stops SOD processes must be denied")
	}
	if !strings.Contains(err.Error(), "permission") {
		t.Fatalf("want permission_denied, got %v", err)
	}
}

// TestEventTriggersWebhookDelivery is the telemetry High regression: a real
// domain event must fan out to a matching registered webhook (previously
// webhooks fired only from the webhook test). Uses an injected delivery recorder
// so the assertion is hermetic (webhook.Deliver blocks loopback by SSRF policy).
func TestEventTriggersWebhookDelivery(t *testing.T) {
	t.Parallel()
	delivered := make(chan webhook.Hook, 4)
	c, _ := newTestDaemonOpts(t,
		func(cfg *config.Config) {
			cfg.Sandbox.Default = "off"
			cfg.Webhook.Allowlist = []string{"*.example.com"}
		},
		func(o *daemon.Options) {
			o.WebhookPoll = 25 * time.Millisecond
			o.WebhookDeliver = func(_ context.Context, h webhook.Hook, _ webhook.Event) error {
				delivered <- h
				return nil
			}
		})
	c.SetSession("sess-full", "full")
	ctx := context.Background()
	if err := c.Call(ctx, api.MethodWebhookCreate, api.WebhookPayload{URL: "https://hooks.example.com/x"}, &api.WebhookView{}); err != nil {
		t.Fatalf("webhook create: %v", err)
	}
	// Starting a process emits a process.started event the dispatcher should fan out.
	if err := c.Call(ctx, api.MethodStart, api.StartPayload{
		Name: "svc", Command: []string{"sleep", "1"}, Sandbox: "off",
	}, &api.StartResult{}); err != nil {
		t.Fatalf("start: %v", err)
	}
	select {
	case h := <-delivered:
		if h.URL != "https://hooks.example.com/x" {
			t.Fatalf("delivered to unexpected hook %q", h.URL)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("event did not trigger a webhook delivery")
	}
}

// TestResolvedSecretValueRedactedInLogs is the telemetry item #4 regression: a
// value resolved from a secret:// ref must be registered so it is scrubbed from
// the process's captured logs (custom tokens that global-pattern redaction can't
// know). Writes the resolved value into the log dir and asserts logs.tail hides it.
func TestResolvedSecretValueRedactedInLogs(t *testing.T) {
	const secretVal = "supersecret-token-abc123xyz"
	t.Setenv("PMMCP_TEST_SECRET", secretVal)
	c, _ := newTestDaemon(t, func(cfg *config.Config) { cfg.Sandbox.Default = "off" })
	c.SetSession("sess-full", "full")
	ctx := context.Background()
	var start api.StartResult
	if err := c.Call(ctx, api.MethodStart, api.StartPayload{
		Name:    "svc",
		Command: []string{"sleep", "1"},
		Sandbox: "off",
		Env:     map[string]string{"TOK": "secret://env/PMMCP_TEST_SECRET"},
	}, &start); err != nil {
		t.Fatalf("start: %v", err)
	}
	// The fake manager writes no logs; simulate the process leaking its secret.
	if err := os.WriteFile(filepath.Join(start.LogDir, "stdout.log"),
		[]byte("connecting with token "+secretVal+"\n"), 0o600); err != nil {
		t.Fatalf("write log: %v", err)
	}
	var out api.LogsResult
	if err := c.Call(ctx, api.MethodLogs, api.LogsPayload{ID: start.ID}, &out); err != nil {
		t.Fatalf("logs: %v", err)
	}
	if strings.Contains(out.Text, secretVal) {
		t.Fatalf("resolved secret value leaked into logs: %q", out.Text)
	}
}

// auditRow mirrors the exported fields of audit.Record used in assertions.
type auditRow struct {
	Action     string
	Outcome    string
	Capability string
	Target     string
}

func auditHas(t *testing.T, c *ipc.Client, pred func(auditRow) bool) bool {
	t.Helper()
	var rows []auditRow
	if err := c.Call(context.Background(), api.MethodAudit, map[string]any{"limit": 1000}, &rows); err != nil {
		t.Fatalf("audit query: %v", err)
	}
	for _, r := range rows {
		if pred(r) {
			return true
		}
	}
	return false
}
