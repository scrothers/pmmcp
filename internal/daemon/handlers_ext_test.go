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
	"testing"
	"time"

	"github.com/scrothers/pmmcp/internal/api"
	"github.com/scrothers/pmmcp/internal/config"
	"github.com/scrothers/pmmcp/internal/daemon"
	"github.com/scrothers/pmmcp/internal/declare"
	"github.com/scrothers/pmmcp/internal/ipc"
	"github.com/scrothers/pmmcp/internal/testsock"
)

func TestHandlersExt_GroupHealthValidateWebhookProfile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	sock := testsock.Path(t)
	cfg, err := config.Load(config.LoadOptions{
		GOOS:      "linux",
		Home:      dir,
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
	defer cancel()
	srv, err := daemon.New(ctx, daemon.Options{Config: cfg, DBPath: filepath.Join(dir, "db.sqlite")})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe(ctx) }()

	var client *ipc.Client
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		client, err = ipc.Dial(ctx, sock)
		if err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if client == nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()

	if _, err := client.Hello(ctx); err != nil {
		t.Fatal(err)
	}

	// Start a process used as a group member.
	var start api.StartResult
	if err := client.Call(ctx, api.MethodStart, api.StartPayload{
		Name: "api", Command: []string{"sleep", "60"}, Sandbox: "off",
		HealthURL: "", // health check falls back to Inspect
	}, &start); err != nil {
		t.Fatalf("start: %v", err)
	}
	if start.ID == "" {
		t.Fatal("empty start id")
	}

	// Group create + status.
	var gview api.GroupView
	if err := client.Call(ctx, api.MethodGroupCreate, api.GroupPayload{
		Name: "web",
		Members: []api.GroupMemberPayload{
			{Name: "api"},
		},
	}, &gview); err != nil {
		t.Fatalf("group create: %v", err)
	}
	if gview.ID == "" || gview.Name != "web" {
		t.Fatalf("group view: %+v", gview)
	}

	var gstatus api.GroupView
	if err := client.Call(ctx, api.MethodGroupStatus, api.GroupPayload{ID: gview.ID}, &gstatus); err != nil {
		t.Fatalf("group status: %v", err)
	}
	if gstatus.Desired != 1 {
		t.Fatalf("desired=%d want 1", gstatus.Desired)
	}
	if len(gstatus.Members) != 1 || gstatus.Members[0].Name != "api" {
		t.Fatalf("members: %+v", gstatus.Members)
	}
	if !gstatus.Members[0].Ready {
		t.Fatalf("member not ready: %+v", gstatus.Members[0])
	}

	// Health check via process still running.
	var health api.HealthCheckResult
	if err := client.Call(ctx, api.MethodHealthCheck, api.IDPayload{ID: start.ID}, &health); err != nil {
		t.Fatalf("health: %v", err)
	}
	if !health.OK {
		t.Fatalf("health not ok: %+v", health)
	}

	// Validate declare YAML.
	yaml := "apiVersion: " + declare.CanonicalAPIVersion + "\n" +
		"kind: Project\n" +
		"services:\n" +
		"  web:\n" +
		"    argv: [\"sleep\", \"1\"]\n"
	var val map[string]any
	if err := client.Call(ctx, api.MethodValidate, api.DeclarePayload{YAML: yaml}, &val); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if val["valid"] != true {
		t.Fatalf("validate result: %+v", val)
	}

	// Webhook create + list (public host to pass SSRF policy).
	var hook api.WebhookView
	if err := client.Call(ctx, api.MethodWebhookCreate, api.WebhookPayload{
		URL:    "https://example.com/hooks/pmmcp",
		Events: []string{"process.crashed"},
	}, &hook); err != nil {
		t.Fatalf("webhook create: %v", err)
	}
	if hook.ID == "" || hook.URL == "" {
		t.Fatalf("hook: %+v", hook)
	}
	var hooks []api.WebhookView
	if err := client.Call(ctx, api.MethodWebhookList, nil, &hooks); err != nil {
		t.Fatalf("webhook list: %v", err)
	}
	if len(hooks) != 1 {
		t.Fatalf("hooks len=%d", len(hooks))
	}

	// Profile create + list.
	var prof api.ProfileView
	if err := client.Call(ctx, api.MethodProfileCreate, api.ProfilePayload{
		Name: "dev", ProjectID: "/tmp/proj", Env: map[string]string{"FOO": "bar"},
	}, &prof); err != nil {
		t.Fatalf("profile create: %v", err)
	}
	if prof.ID == "" || prof.Name != "dev" {
		t.Fatalf("profile: %+v", prof)
	}
	var profiles []api.ProfileView
	if err := client.Call(ctx, api.MethodProfileList, api.ProfilePayload{ProjectID: "/tmp/proj"}, &profiles); err != nil {
		t.Fatalf("profile list: %v", err)
	}
	if len(profiles) != 1 {
		t.Fatalf("profiles len=%d", len(profiles))
	}

	// Cleanup process.
	_ = client.Call(ctx, api.MethodStop, api.IDPayload{ID: start.ID, TimeoutSec: 2}, &map[string]any{})
	cancel()
	select {
	case <-errCh:
	case <-time.After(2 * time.Second):
	}
}
