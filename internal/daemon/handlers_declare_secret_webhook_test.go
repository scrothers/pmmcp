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
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/scrothers/pmmcp/internal/api"
	"github.com/scrothers/pmmcp/internal/config"
	"github.com/scrothers/pmmcp/internal/daemon"
	"github.com/scrothers/pmmcp/internal/declare"
	"github.com/scrothers/pmmcp/internal/domain"
	"github.com/scrothers/pmmcp/internal/ipc"
	"github.com/scrothers/pmmcp/internal/store"
	"github.com/scrothers/pmmcp/internal/testsock"
)

// bootServer is a local daemon-boot helper (like newTestDaemonOpts) that also
// returns the *daemon.Server, needed by this file's fault-injection tests
// (store swap, direct keyring writes) via the handlers_b_export_test.go seams.
func bootServer(t *testing.T, tweak func(*config.Config)) (*ipc.Client, *daemon.Server) {
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
	if tweak != nil {
		tweak(cfg)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	srv, err := daemon.New(ctx, daemon.Options{Config: cfg, DBPath: sqliteDBPathForTest(t)})
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
	t.Cleanup(func() {
		stopAllForTest(ctx, t, c)
		_ = c.Close()
	})
	return c, srv
}

// failingStore fails every method, for exercising declare.diff/apply's
// store.List error path (unreachable through a healthy store via the public
// IPC surface).
type failingStore struct{}

func (failingStore) Migrate(context.Context) error { return nil }
func (failingStore) Create(context.Context, *domain.Process) error {
	return errors.New("failingStore: create")
}
func (failingStore) Get(context.Context, string) (*domain.Process, error) {
	return nil, errors.New("failingStore: get")
}
func (failingStore) Update(context.Context, *domain.Process) error {
	return errors.New("failingStore: update")
}
func (failingStore) UpdateWithCAS(context.Context, *domain.Process) error {
	return errors.New("failingStore: updatewithcas")
}
func (failingStore) Delete(context.Context, string) error {
	return errors.New("failingStore: delete")
}
func (failingStore) List(context.Context, store.ProcessFilter) ([]*domain.Process, error) {
	return nil, errors.New("failingStore: list")
}
func (failingStore) Close() error { return nil }

const validAPIVersion = declare.CanonicalAPIVersion

func declareYAML(body string) string {
	return "apiVersion: " + validAPIVersion + "\nkind: Project\n" + body
}

// --- loadDeclare / doDeclareValidate ---

func TestDoDeclareValidate_DataField(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, nil)
	ctx := context.Background()
	var out map[string]any
	yaml := declareYAML("services:\n  web:\n    argv: [\"sleep\", \"1\"]\n")
	if err := c.Call(ctx, api.MethodValidate, api.DeclarePayload{Data: yaml}, &out); err != nil {
		t.Fatalf("validate via Data field: %v", err)
	}
	if out["valid"] != true {
		t.Fatalf("result: %+v", out)
	}
}

func TestDoDeclareValidate_PathReadError(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, nil)
	ctx := context.Background()
	var out map[string]any
	err := c.Call(ctx, api.MethodValidate, api.DeclarePayload{Path: "/nonexistent/pmmcp.yaml"}, &out)
	if err == nil {
		t.Fatal("validate with missing path: want error, got nil")
	}
}

func TestDoDeclareValidate_NoSourceProvided(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, nil)
	ctx := context.Background()
	var out map[string]any
	err := c.Call(ctx, api.MethodValidate, api.DeclarePayload{}, &out)
	if err == nil {
		t.Fatal("validate with no yaml/data/path: want error, got nil")
	}
}

func TestDoDeclareValidate_ParseError(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, nil)
	ctx := context.Background()
	var out map[string]any
	err := c.Call(ctx, api.MethodValidate, api.DeclarePayload{YAML: "not: [valid: yaml"}, &out)
	if err == nil {
		t.Fatal("validate with unparsable yaml: want error, got nil")
	}
}

func TestDoDeclareValidate_ValidationError(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, nil)
	ctx := context.Background()
	var out map[string]any
	// Parses fine, fails Validate: wrong apiVersion.
	err := c.Call(ctx, api.MethodValidate, api.DeclarePayload{YAML: "apiVersion: v0\nkind: Project\n"}, &out)
	if err == nil {
		t.Fatal("validate with unsupported apiVersion: want error, got nil")
	}
}

// --- doDeclareDiff ---

func TestDoDeclareDiff_LoadError(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, nil)
	ctx := context.Background()
	var out map[string]any
	err := c.Call(ctx, api.MethodDiff, api.DeclarePayload{}, &out)
	if err == nil {
		t.Fatal("diff with no source: want error, got nil")
	}
}

func TestDoDeclareDiff_StoreListError(t *testing.T) {
	t.Parallel()
	c, srv := bootServer(t, nil)
	prevStore := daemon.SetStoreForTest(srv, failingStore{})
	t.Cleanup(func() { _ = prevStore.Close() })
	ctx := context.Background()
	yaml := declareYAML("services:\n  web:\n    argv: [\"sleep\", \"1\"]\n")
	var out map[string]any
	// RunningNames is nil, forcing doDeclareDiff to consult s.store.List, which
	// now fails via the swapped store.
	err := c.Call(ctx, api.MethodDiff, api.DeclarePayload{YAML: yaml}, &out)
	if err == nil {
		t.Fatal("diff with a failing store: want error, got nil")
	}
}

// --- doDeclareApply ---

func TestDoDeclareApply_LoadError(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, func(cfg *config.Config) { cfg.Sandbox.Default = "strict" })
	c.SetSession("s1", "full")
	ctx := context.Background()
	var out map[string]any
	err := c.Call(ctx, api.MethodApply, api.DeclarePayload{}, &out)
	if err == nil {
		t.Fatal("apply with no source: want error, got nil")
	}
}

func TestDoDeclareApply_ValidateError(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, nil)
	c.SetSession("s1", "full")
	ctx := context.Background()
	var out map[string]any
	err := c.Call(ctx, api.MethodApply, api.DeclarePayload{YAML: "apiVersion: v0\nkind: Project\n"}, &out)
	if err == nil {
		t.Fatal("apply with invalid document: want error, got nil")
	}
}

func TestDoDeclareApply_SkipsServiceWithoutArgv(t *testing.T) {
	t.Parallel()
	c, mgr := newTestDaemon(t, nil)
	c.SetSession("s1", "full")
	ctx := context.Background()
	// image-only service: passes Validate (image satisfies the argv-or-image
	// rule) but doDeclareApply must skip starting it (no argv to exec).
	yaml := declareYAML("services:\n  imgsvc:\n    image: example/img:latest\n")
	var out struct {
		Created []string            `json:"created"`
		Diff    []declare.DiffEntry `json:"diff"`
	}
	if err := c.Call(ctx, api.MethodApply, api.DeclarePayload{YAML: yaml}, &out); err != nil {
		t.Fatalf("apply: %v", err)
	}
	for _, name := range out.Created {
		if name == "imgsvc" {
			t.Fatalf("imgsvc should have been skipped (no argv), created=%v", out.Created)
		}
	}
	mgr.mu.Lock()
	defer mgr.mu.Unlock()
	for _, sp := range mgr.specs {
		if sp.Name == "imgsvc" {
			t.Fatalf("imgsvc should never reach the process manager: %+v", sp)
		}
	}
}

func TestDoDeclareApply_DoStartFailurePropagates(t *testing.T) {
	t.Parallel()
	// declare's own policy check only rejects the literal sandbox value "off",
	// so an unrecognized profile name passes Validate but fails doStart's own
	// sandbox lookup — this exercises doDeclareApply's "propagate the failed
	// doStart response" branch without fighting the declare-level policy gate.
	c, _ := newTestDaemon(t, nil)
	c.SetSession("s1", "full")
	ctx := context.Background()
	yaml := declareYAML("services:\n  web:\n    argv: [\"sleep\", \"1\"]\n    sandbox: not-a-real-profile\n")
	var out map[string]any
	err := c.Call(ctx, api.MethodApply, api.DeclarePayload{YAML: yaml}, &out)
	if err == nil {
		t.Fatal("apply with an unrecognized sandbox profile: want error, got nil")
	}
}

func TestDoDeclareApply_StoreListError(t *testing.T) {
	t.Parallel()
	c, srv := bootServer(t, nil)
	c.SetSession("s1", "full")
	prevStore := daemon.SetStoreForTest(srv, failingStore{})
	t.Cleanup(func() { _ = prevStore.Close() })

	ctx := context.Background()
	yaml := declareYAML("services:\n  web:\n    argv: [\"sleep\", \"1\"]\n")
	var out map[string]any
	if err := c.Call(ctx, api.MethodApply, api.DeclarePayload{YAML: yaml}, &out); err == nil {
		t.Fatal("apply with a failing store: want error, got nil")
	}
}

// --- doDeclareShow ---

func TestDoDeclareShow_LoadError(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, nil)
	ctx := context.Background()
	var out map[string]any
	err := c.Call(ctx, api.MethodDeclareShow, api.DeclarePayload{}, &out)
	if err == nil {
		t.Fatal("declare.show with no source: want error, got nil")
	}
}

// --- doDeclareImport ---

func TestDoDeclareImport_PathReadError(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, nil)
	ctx := context.Background()
	var out map[string]any
	err := c.Call(ctx, api.MethodImport, api.DeclarePayload{Path: "/nonexistent/Procfile"}, &out)
	if err == nil {
		t.Fatal("import with missing path: want error, got nil")
	}
}

func TestDoDeclareImport_NoSourceProvided(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, nil)
	ctx := context.Background()
	var out map[string]any
	err := c.Call(ctx, api.MethodImport, api.DeclarePayload{}, &out)
	if err == nil {
		t.Fatal("import with no data/path: want error, got nil")
	}
}

func TestDoDeclareImport_BadProcfile(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, nil)
	ctx := context.Background()
	var out map[string]any
	err := c.Call(ctx, api.MethodImport, api.DeclarePayload{Data: "this has no colon"}, &out)
	if err == nil {
		t.Fatal("import with malformed procfile: want error, got nil")
	}
}

func TestDoDeclareImport_UnsupportedFormat(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, nil)
	ctx := context.Background()
	var out map[string]any
	err := c.Call(ctx, api.MethodImport, api.DeclarePayload{Data: "web: sleep 1", Format: "yaml"}, &out)
	if err == nil {
		t.Fatal("import with unsupported format: want error, got nil")
	}
}

// --- doSecretRefCheck ---

func TestDoSecretRefCheck_RefForm(t *testing.T) {
	// t.Setenv cannot be combined with t.Parallel.
	const secretVal = "ref-check-value-xyz"
	t.Setenv("PMMCP_TEST_REFCHECK", secretVal)
	c, _ := newTestDaemon(t, nil)
	ctx := context.Background()
	var out map[string]any
	if err := c.Call(ctx, api.MethodSecretRefCheck, api.SecretPayload{Ref: "secret://env:PMMCP_TEST_REFCHECK"}, &out); err != nil {
		t.Fatalf("ref_check: %v", err)
	}
	if out["ok"] != true {
		t.Fatalf("ref_check result: %+v", out)
	}
}

func TestDoSecretRefCheck_NameAndRefEmpty(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, nil)
	ctx := context.Background()
	var out map[string]any
	err := c.Call(ctx, api.MethodSecretRefCheck, api.SecretPayload{}, &out)
	if err == nil {
		t.Fatal("ref_check with no name/ref: want error, got nil")
	}
}

func TestDoSecretRefCheck_KeyringOnlyLookup(t *testing.T) {
	t.Parallel()
	c, srv := bootServer(t, nil)
	ctx := context.Background()
	kr := daemon.KeyringForTest(srv)
	if _, err := kr.Set("kronly", "value"); err != nil {
		t.Fatalf("keyring set: %v", err)
	}
	var out map[string]any
	if err := c.Call(ctx, api.MethodSecretRefCheck, api.SecretPayload{Name: "kronly"}, &out); err != nil {
		t.Fatalf("ref_check: %v", err)
	}
	if out["ok"] != true {
		t.Fatalf("ref_check result (want found via keyring fallback): %+v", out)
	}
}

func TestDoSecretRefCheck_NotFoundAnywhere(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, nil)
	ctx := context.Background()
	var out map[string]any
	if err := c.Call(ctx, api.MethodSecretRefCheck, api.SecretPayload{Name: "does-not-exist"}, &out); err != nil {
		t.Fatalf("ref_check: %v", err)
	}
	if out["ok"] != false {
		t.Fatalf("ref_check result (want not found): %+v", out)
	}
}

func TestDoSecretRefCheck_PathMissing(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, nil)
	ctx := context.Background()
	dir := t.TempDir()
	missing := filepath.Join(dir, "nope.txt")
	var setRes api.SecretRefView
	if err := c.Call(ctx, api.MethodSecretSet, api.SecretPayload{Name: "pathref", Path: missing}, &setRes); err != nil {
		t.Fatalf("secret.set: %v", err)
	}
	if err := os.Remove(missing); err == nil {
		t.Fatal("expected the path to not exist yet (secret.set with Path should not create it)")
	}
	var out map[string]any
	if err := c.Call(ctx, api.MethodSecretRefCheck, api.SecretPayload{Name: "pathref"}, &out); err != nil {
		t.Fatalf("ref_check: %v", err)
	}
	if out["ok"] != false {
		t.Fatalf("ref_check result (want stat failure): %+v", out)
	}
}

func TestDoSecretRefCheck_SOPSDecryptFails(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, nil)
	ctx := context.Background()
	dir := t.TempDir()
	// bytesLookLikeSOPS matches on an "ENC[" marker anywhere in the body,
	// independent of whether the ciphertext is well-formed: this file "looks
	// like" SOPS but is not real ciphertext, so decrypt.File fails.
	sopsLike := filepath.Join(dir, "fake.env")
	if err := os.WriteFile(sopsLike, []byte("KEY=ENC[not-real-sops-ciphertext]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var setRes api.SecretRefView
	if err := c.Call(ctx, api.MethodSecretSet, api.SecretPayload{Name: "sopsref", Path: sopsLike}, &setRes); err != nil {
		t.Fatalf("secret.set: %v", err)
	}
	var out map[string]any
	if err := c.Call(ctx, api.MethodSecretRefCheck, api.SecretPayload{Name: "sopsref"}, &out); err != nil {
		t.Fatalf("ref_check: %v", err)
	}
	if out["ok"] != false || out["sops"] != true {
		t.Fatalf("ref_check result (want sops decrypt failure): %+v", out)
	}
}

// --- doSecretSet ---

func TestDoSecretSet_BadPayload(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, nil)
	ctx := context.Background()
	var out map[string]any
	// A JSON array cannot unmarshal into api.SecretPayload (a struct).
	err := c.Call(ctx, api.MethodSecretSet, []int{1, 2}, &out)
	if err == nil {
		t.Fatal("secret.set with malformed payload: want error, got nil")
	}
}

func TestDoSecretSet_NameRequired(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, nil)
	ctx := context.Background()
	var out map[string]any
	err := c.Call(ctx, api.MethodSecretSet, api.SecretPayload{Path: "/tmp/x"}, &out)
	if err == nil {
		t.Fatal("secret.set with no name: want error, got nil")
	}
}

func TestDoSecretSet_PathOrValueRequired(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, nil)
	ctx := context.Background()
	var out map[string]any
	err := c.Call(ctx, api.MethodSecretSet, api.SecretPayload{Name: "noval"}, &out)
	if err == nil {
		t.Fatal("secret.set with no path/value: want error, got nil")
	}
}

// --- doWatchSet ---

func TestDoWatchSet_PathRequired(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, nil)
	ctx := context.Background()
	var start api.StartResult
	if err := c.Call(ctx, api.MethodStart, api.StartPayload{Name: "w1", Command: []string{"sleep", "5"}, Sandbox: "off"}, &start); err != nil {
		t.Fatalf("start: %v", err)
	}
	var out map[string]any
	err := c.Call(ctx, api.MethodWatchSet, api.WatchPayload{ID: start.ID}, &out)
	if err == nil {
		t.Fatal("watch.set with no path: want error, got nil")
	}
}

func TestDoWatchSet_ResolveIDError(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, nil)
	ctx := context.Background()
	var out map[string]any
	err := c.Call(ctx, api.MethodWatchSet, api.WatchPayload{ID: "proc-does-not-exist", Path: t.TempDir()}, &out)
	if err == nil {
		t.Fatal("watch.set on an unknown process: want error, got nil")
	}
}

func TestDoWatchSet_WatchAddError(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, nil)
	ctx := context.Background()
	var start api.StartResult
	if err := c.Call(ctx, api.MethodStart, api.StartPayload{Name: "w2", Command: []string{"sleep", "5"}, Sandbox: "off"}, &start); err != nil {
		t.Fatalf("start: %v", err)
	}
	var out map[string]any
	err := c.Call(ctx, api.MethodWatchSet, api.WatchPayload{ID: start.ID, Path: filepath.Join(t.TempDir(), "does-not-exist")}, &out)
	if err == nil {
		t.Fatal("watch.set on a nonexistent path: want error, got nil")
	}
}

// --- doWebhookCreate / Update / Delete / Test ---

func webhookAllowlistDaemon(t *testing.T) *ipc.Client {
	t.Helper()
	c, _ := newTestDaemon(t, func(cfg *config.Config) {
		cfg.Webhook.Allowlist = []string{"*.example.com", "*.pmmcp-test-nowhere.invalid"}
	})
	return c
}

func TestDoWebhookCreate_BadPayload(t *testing.T) {
	t.Parallel()
	c := webhookAllowlistDaemon(t)
	ctx := context.Background()
	var out map[string]any
	if err := c.Call(ctx, api.MethodWebhookCreate, []int{1}, &out); err == nil {
		t.Fatal("webhook.create with malformed payload: want error, got nil")
	}
}

func TestDoWebhookCreate_URLRequired(t *testing.T) {
	t.Parallel()
	c := webhookAllowlistDaemon(t)
	ctx := context.Background()
	var out map[string]any
	if err := c.Call(ctx, api.MethodWebhookCreate, api.WebhookPayload{}, &out); err == nil {
		t.Fatal("webhook.create with no url: want error, got nil")
	}
}

func TestDoWebhookCreate_SSRFRejected(t *testing.T) {
	t.Parallel()
	c := webhookAllowlistDaemon(t)
	ctx := context.Background()
	var out map[string]any
	err := c.Call(ctx, api.MethodWebhookCreate, api.WebhookPayload{URL: "http://127.0.0.1:9/hook"}, &out)
	if err == nil {
		t.Fatal("webhook.create with a loopback URL: want error, got nil")
	}
	var derr *domain.Error
	if !errors.As(err, &derr) || derr.Code != domain.CodeInvalidArgument {
		t.Fatalf("err = %v, want invalid_argument", err)
	}
}

func TestDoWebhookCreate_IDGenerationFailure(t *testing.T) {
	// Mutates the package-level crypto/rand.Reader: not parallel-safe.
	c := webhookAllowlistDaemon(t)
	ctx := context.Background()
	orig := rand.Reader
	rand.Reader = failingReader{}
	t.Cleanup(func() { rand.Reader = orig })
	var out map[string]any
	err := c.Call(ctx, api.MethodWebhookCreate, api.WebhookPayload{URL: "https://a.example.com/hook"}, &out)
	if err == nil {
		t.Fatal("webhook.create under id generation failure: want error, got nil")
	}
}

// failingReader always fails Read, used to force crypto/rand-backed ID
// generation (ulid.New) to return an error deterministically.
type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("failingReader: boom") }

func TestDoWebhookUpdate_BadPayload(t *testing.T) {
	t.Parallel()
	c := webhookAllowlistDaemon(t)
	ctx := context.Background()
	var out map[string]any
	if err := c.Call(ctx, api.MethodWebhookUpdate, []int{1}, &out); err == nil {
		t.Fatal("webhook.update with malformed payload: want error, got nil")
	}
}

func TestDoWebhookUpdate_IDRequired(t *testing.T) {
	t.Parallel()
	c := webhookAllowlistDaemon(t)
	ctx := context.Background()
	var out map[string]any
	if err := c.Call(ctx, api.MethodWebhookUpdate, api.WebhookPayload{URL: "https://a.example.com/h"}, &out); err == nil {
		t.Fatal("webhook.update with no id: want error, got nil")
	}
}

func TestDoWebhookUpdate_NotFound(t *testing.T) {
	t.Parallel()
	c := webhookAllowlistDaemon(t)
	ctx := context.Background()
	var out map[string]any
	err := c.Call(ctx, api.MethodWebhookUpdate, api.WebhookPayload{ID: "wh-nope", URL: "https://a.example.com/h"}, &out)
	if err == nil {
		t.Fatal("webhook.update on an unknown id: want error, got nil")
	}
	var derr *domain.Error
	if !errors.As(err, &derr) || derr.Code != domain.CodeNotFound {
		t.Fatalf("err = %v, want not_found", err)
	}
}

func TestDoWebhookUpdate_SSRFRejectedOnNewURL(t *testing.T) {
	t.Parallel()
	c := webhookAllowlistDaemon(t)
	ctx := context.Background()
	var hook api.WebhookView
	if err := c.Call(ctx, api.MethodWebhookCreate, api.WebhookPayload{URL: "https://a.example.com/h"}, &hook); err != nil {
		t.Fatalf("create: %v", err)
	}
	var out map[string]any
	err := c.Call(ctx, api.MethodWebhookUpdate, api.WebhookPayload{ID: hook.ID, URL: "http://127.0.0.1:9/hook"}, &out)
	if err == nil {
		t.Fatal("webhook.update to a loopback URL: want error, got nil")
	}
}

func TestDoWebhookUpdate_EventsFieldReplaced(t *testing.T) {
	t.Parallel()
	c := webhookAllowlistDaemon(t)
	ctx := context.Background()
	var hook api.WebhookView
	if err := c.Call(ctx, api.MethodWebhookCreate, api.WebhookPayload{URL: "https://a.example.com/h", Events: []string{"process.started"}}, &hook); err != nil {
		t.Fatalf("create: %v", err)
	}
	var updated api.WebhookView
	err := c.Call(ctx, api.MethodWebhookUpdate, api.WebhookPayload{ID: hook.ID, Events: []string{"process.crashed", "process.exited"}}, &updated)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if len(updated.Events) != 2 || updated.Events[0] != "process.crashed" || updated.Events[1] != "process.exited" {
		t.Fatalf("updated.Events = %v, want [process.crashed process.exited]", updated.Events)
	}
}

func TestDoWebhookDelete_IDRequired(t *testing.T) {
	t.Parallel()
	c := webhookAllowlistDaemon(t)
	ctx := context.Background()
	var out map[string]any
	if err := c.Call(ctx, api.MethodWebhookDelete, api.WebhookPayload{}, &out); err == nil {
		t.Fatal("webhook.delete with no id: want error, got nil")
	}
}

func TestDoWebhookDelete_NotFound(t *testing.T) {
	t.Parallel()
	c := webhookAllowlistDaemon(t)
	ctx := context.Background()
	var out map[string]any
	err := c.Call(ctx, api.MethodWebhookDelete, api.WebhookPayload{ID: "wh-nope"}, &out)
	if err == nil {
		t.Fatal("webhook.delete on an unknown id: want error, got nil")
	}
	var derr *domain.Error
	if !errors.As(err, &derr) || derr.Code != domain.CodeNotFound {
		t.Fatalf("err = %v, want not_found", err)
	}
}

func TestDoWebhookTest_IDRequired(t *testing.T) {
	t.Parallel()
	c := webhookAllowlistDaemon(t)
	ctx := context.Background()
	var out map[string]any
	if err := c.Call(ctx, api.MethodWebhookTest, api.WebhookPayload{}, &out); err == nil {
		t.Fatal("webhook.test with no id: want error, got nil")
	}
}

func TestDoWebhookTest_NotFound(t *testing.T) {
	t.Parallel()
	c := webhookAllowlistDaemon(t)
	ctx := context.Background()
	var out map[string]any
	err := c.Call(ctx, api.MethodWebhookTest, api.WebhookPayload{ID: "wh-nope"}, &out)
	if err == nil {
		t.Fatal("webhook.test on an unknown id: want error, got nil")
	}
	var derr *domain.Error
	if !errors.As(err, &derr) || derr.Code != domain.CodeNotFound {
		t.Fatalf("err = %v, want not_found", err)
	}
}

func TestDoWebhookTest_DeliveryFails(t *testing.T) {
	t.Parallel()
	c := webhookAllowlistDaemon(t)
	ctx := context.Background()
	var hook api.WebhookView
	// On the allowlist by hostname pattern, but does not resolve/is unreachable:
	// webhook.Deliver fails at request time, exercising the delivery-error branch.
	if err := c.Call(ctx, api.MethodWebhookCreate, api.WebhookPayload{URL: "https://unreachable.pmmcp-test-nowhere.invalid/hook"}, &hook); err != nil {
		t.Fatalf("create: %v", err)
	}
	var out map[string]any
	err := c.Call(ctx, api.MethodWebhookTest, api.WebhookPayload{ID: hook.ID}, &out)
	if err == nil {
		t.Fatal("webhook.test against an unreachable host: want error, got nil")
	}
	var derr *domain.Error
	if !errors.As(err, &derr) || derr.Code != domain.CodeInternal {
		t.Fatalf("err = %v, want internal", err)
	}
}

// --- doLogsExport ---

func TestDoLogsExport_ResolveIDError(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, nil)
	ctx := context.Background()
	var out map[string]any
	err := c.Call(ctx, api.MethodLogsExport, api.LogsPayload{ID: "proc-does-not-exist"}, &out)
	if err == nil {
		t.Fatal("logs.export on an unknown process: want error, got nil")
	}
}

func TestDoLogsExport_MkdirAllFails(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory write permission checks")
	}
	t.Parallel()
	c, _ := newTestDaemon(t, func(cfg *config.Config) {
		// "exports" pre-exists as a regular file, so MkdirAll(exportDir) fails
		// with ENOTDIR.
		if err := os.MkdirAll(cfg.StateDir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(cfg.StateDir, "exports"), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	})
	ctx := context.Background()
	var start api.StartResult
	if err := c.Call(ctx, api.MethodStart, api.StartPayload{Name: "le1", Command: []string{"sleep", "5"}, Sandbox: "off"}, &start); err != nil {
		t.Fatalf("start: %v", err)
	}
	var out map[string]any
	err := c.Call(ctx, api.MethodLogsExport, api.LogsPayload{ID: start.ID}, &out)
	if err == nil {
		t.Fatal("logs.export with exports-as-file: want error, got nil")
	}
}

func TestDoLogsExport_OpenFileFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits aren't enforced by Windows ACLs")
	}
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory write permission checks")
	}
	t.Parallel()
	var exportsDir string
	c, _ := newTestDaemon(t, func(cfg *config.Config) {
		if err := os.MkdirAll(cfg.StateDir, 0o700); err != nil {
			t.Fatal(err)
		}
		exportsDir = filepath.Join(cfg.StateDir, "exports")
		if err := os.Mkdir(exportsDir, 0o500); err != nil {
			t.Fatal(err)
		}
	})
	t.Cleanup(func() { _ = os.Chmod(exportsDir, 0o700) })
	ctx := context.Background()
	var start api.StartResult
	if err := c.Call(ctx, api.MethodStart, api.StartPayload{Name: "le2", Command: []string{"sleep", "5"}, Sandbox: "off"}, &start); err != nil {
		t.Fatalf("start: %v", err)
	}
	var out map[string]any
	err := c.Call(ctx, api.MethodLogsExport, api.LogsPayload{ID: start.ID}, &out)
	if err == nil {
		t.Fatal("logs.export with a read-only exports dir: want error, got nil")
	}
}

// --- doLogsShip ---

func TestDoLogsShip_BadPayload(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, nil)
	ctx := context.Background()
	var out map[string]any
	if err := c.Call(ctx, api.MethodLogsShip, []int{1}, &out); err == nil {
		t.Fatal("logs.ship with malformed payload: want error, got nil")
	}
}

func TestDoLogsShip_PathsRequired(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, nil)
	ctx := context.Background()
	var out map[string]any
	if err := c.Call(ctx, api.MethodLogsShip, api.LogsShipPayload{}, &out); err == nil {
		t.Fatal("logs.ship with no export/sink path: want error, got nil")
	}
}

func TestDoLogsShip_SourceMissing(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, nil)
	ctx := context.Background()
	dir := t.TempDir()
	var out map[string]any
	err := c.Call(ctx, api.MethodLogsShip, api.LogsShipPayload{
		ExportPath: filepath.Join(dir, "missing.tar.gz"),
		SinkPath:   filepath.Join(dir, "out.tar.gz"),
	}, &out)
	if err == nil {
		t.Fatal("logs.ship with a missing export path: want error, got nil")
	}
}

func TestDoLogsShip_SinkParentMkdirFails(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory write permission checks")
	}
	t.Parallel()
	c, _ := newTestDaemon(t, nil)
	ctx := context.Background()
	dir := t.TempDir()
	src := filepath.Join(dir, "export.tar.gz")
	if err := os.WriteFile(src, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	err := c.Call(ctx, api.MethodLogsShip, api.LogsShipPayload{
		ExportPath: src,
		SinkPath:   filepath.Join(blocker, "sub", "out.tar.gz"),
	}, &out)
	if err == nil {
		t.Fatal("logs.ship with a blocked sink parent dir: want error, got nil")
	}
}

func TestDoLogsShip_SinkAlreadyExists(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, nil)
	ctx := context.Background()
	dir := t.TempDir()
	src := filepath.Join(dir, "export.tar.gz")
	if err := os.WriteFile(src, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	sink := filepath.Join(dir, "out.tar.gz")
	if err := os.WriteFile(sink, []byte("preexisting"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	err := c.Call(ctx, api.MethodLogsShip, api.LogsShipPayload{ExportPath: src, SinkPath: sink}, &out)
	if err == nil {
		t.Fatal("logs.ship over a pre-existing sink: want error, got nil")
	}
	var derr *domain.Error
	if !errors.As(err, &derr) || derr.Code != domain.CodeInvalidArgument {
		t.Fatalf("err = %v, want invalid_argument", err)
	}
	got, rerr := os.ReadFile(sink)
	if rerr != nil {
		t.Fatal(rerr)
	}
	if string(got) != "preexisting" {
		t.Fatalf("sink was overwritten: %q", got)
	}
}

func TestDoLogsShip_CopyErrorFromDirectorySource(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, nil)
	ctx := context.Background()
	dir := t.TempDir()
	// os.Open succeeds on a directory; io.Copy's Read then fails (EISDIR on
	// Linux), exercising doLogsShip's copyErr branch deterministically.
	srcDir := filepath.Join(dir, "srcdir")
	if err := os.Mkdir(srcDir, 0o700); err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	err := c.Call(ctx, api.MethodLogsShip, api.LogsShipPayload{
		ExportPath: srcDir,
		SinkPath:   filepath.Join(dir, "out.tar.gz"),
	}, &out)
	if err == nil {
		t.Fatal("logs.ship copying from a directory: want error, got nil")
	}
}

func TestDoLogsShip_SinkAliasPathField(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, nil)
	ctx := context.Background()
	dir := t.TempDir()
	src := filepath.Join(dir, "export.tar.gz")
	if err := os.WriteFile(src, []byte("payload"), 0o600); err != nil {
		t.Fatal(err)
	}
	sink := filepath.Join(dir, "shipped.tar.gz")
	var out map[string]string
	if err := c.Call(ctx, api.MethodLogsShip, api.LogsShipPayload{ExportPath: src, Path: sink}, &out); err != nil {
		t.Fatalf("logs.ship via Path alias: %v", err)
	}
	got, err := os.ReadFile(sink)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "payload" {
		t.Fatalf("shipped content = %q", got)
	}
}

// --- doSubscribe (id generation failure) ---

func TestDoSubscribe_IDGenerationFailure(t *testing.T) {
	// Mutates the package-level crypto/rand.Reader: not parallel-safe.
	c, _ := newTestDaemon(t, nil)
	ctx := context.Background()
	orig := rand.Reader
	rand.Reader = failingReader{}
	t.Cleanup(func() { rand.Reader = orig })
	var out map[string]any
	err := c.Call(ctx, api.MethodLogsSubscribe, api.SubPayload{}, &out)
	if err == nil {
		t.Fatal("logs.subscribe under id generation failure: want error, got nil")
	}
}

var _ io.Reader = failingReader{}

// --- doSubscribe (logs preview branch) ---

func TestDoSubscribe_LogsPreviewFollowsLogDir(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, nil)
	ctx := context.Background()
	var start api.StartResult
	if err := c.Call(ctx, api.MethodStart, api.StartPayload{Name: "sp1", Command: []string{"sleep", "5"}, Sandbox: "off"}, &start); err != nil {
		t.Fatalf("start: %v", err)
	}
	var out map[string]any
	// ProcessID (not "id") is SubPayload's field for the process to preview;
	// this is what actually reaches doSubscribe's followLogDir call.
	if err := c.Call(ctx, api.MethodLogsSubscribe, api.SubPayload{ProcessID: start.ID}, &out); err != nil {
		t.Fatalf("logs.subscribe: %v", err)
	}
	if out["id"] == nil {
		t.Fatalf("subscribe result: %+v", out)
	}
}

// --- doPorts ---

func TestDoPorts_NoDeclaredPorts(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, nil)
	ctx := context.Background()
	var start api.StartResult
	if err := c.Call(ctx, api.MethodStart, api.StartPayload{Name: "noports", Command: []string{"sleep", "5"}, Sandbox: "off"}, &start); err != nil {
		t.Fatalf("start: %v", err)
	}
	var out api.PortsResult
	if err := c.Call(ctx, api.MethodPorts, api.IDPayload{ID: start.ID}, &out); err != nil {
		t.Fatalf("ports: %v", err)
	}
	if out.Ports == nil {
		t.Fatal("Ports should be an empty slice, not nil, when none are declared")
	}
}

// --- doSecretSet (keyring.Set validation error) ---

func TestDoSecretSet_KeyringSetInvalidName(t *testing.T) {
	t.Parallel()
	c, _ := newTestDaemon(t, nil)
	ctx := context.Background()
	var out map[string]any
	// A name containing "/" fails FileBackend's validateKeyringName check.
	err := c.Call(ctx, api.MethodSecretSet, api.SecretPayload{Name: "bad/name", Value: "v"}, &out)
	if err == nil {
		t.Fatal("secret.set with an invalid keyring name: want error, got nil")
	}
}

// --- doMetrics ---

func TestDoMetrics_StoreListError(t *testing.T) {
	t.Parallel()
	c, srv := bootServer(t, nil)
	prevStore := daemon.SetStoreForTest(srv, failingStore{})
	t.Cleanup(func() { _ = prevStore.Close() })
	ctx := context.Background()
	var out map[string]any
	if err := c.Call(ctx, api.MethodMetrics, nil, &out); err == nil {
		t.Fatal("metrics.snapshot with a failing store: want error, got nil")
	}
}

// --- doLogsExport (ExportTarGz failure) ---

func TestDoLogsExport_ExportTarGzFails(t *testing.T) {
	t.Parallel()
	// filepath.Glob ignores I/O errors (permission denied yields no matches,
	// not an error), so exportFileNames can only fail on ErrBadPattern: an
	// unbalanced "[" anywhere in the constructed glob pattern. LogDir is
	// derived from cfg.StateDir, so a StateDir containing "[" propagates one
	// into the pattern deterministically.
	c, _ := newTestDaemon(t, func(cfg *config.Config) {
		cfg.StateDir = cfg.StateDir + "[unclosed"
	})
	ctx := context.Background()
	var start api.StartResult
	if err := c.Call(ctx, api.MethodStart, api.StartPayload{Name: "le3", Command: []string{"sleep", "5"}, Sandbox: "off"}, &start); err != nil {
		t.Fatalf("start: %v", err)
	}
	var out map[string]any
	err := c.Call(ctx, api.MethodLogsExport, api.LogsPayload{ID: start.ID}, &out)
	if err == nil {
		t.Fatal("logs.export with a glob-breaking state dir: want error, got nil")
	}
}

// --- doLogsShip (OpenFile generic failure, not os.ErrExist) ---

func TestDoLogsShip_SinkOpenFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits aren't enforced by Windows ACLs")
	}
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory write permission checks")
	}
	t.Parallel()
	c, _ := newTestDaemon(t, nil)
	ctx := context.Background()
	dir := t.TempDir()
	src := filepath.Join(dir, "export.tar.gz")
	if err := os.WriteFile(src, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	sinkDir := filepath.Join(dir, "sinkdir")
	if err := os.Mkdir(sinkDir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(sinkDir, 0o700) })
	var out map[string]any
	err := c.Call(ctx, api.MethodLogsShip, api.LogsShipPayload{
		ExportPath: src,
		SinkPath:   filepath.Join(sinkDir, "out.tar.gz"),
	}, &out)
	if err == nil {
		t.Fatal("logs.ship into a read-only sink dir: want error, got nil")
	}
	var derr *domain.Error
	if !errors.As(err, &derr) || derr.Code == domain.CodeInvalidArgument {
		t.Fatalf("err = %v, want a generic (non-invalid_argument) error, not the os.ErrExist branch", err)
	}
}
