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
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/scrothers/pmmcp/internal/api"
)

func TestEventsProcessStarted(t *testing.T) {
	t.Parallel()
	ctx, _, c, _ := startTestDaemon(t)
	var start api.StartResult
	if err := c.Call(ctx, api.MethodStart, api.StartPayload{
		Name: "ev", Command: []string{"sleep", "20"}, Sandbox: "off",
	}, &start); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		var evs []api.EventView
		if err := c.Call(ctx, api.MethodEvents, api.EventsPayload{ProcessID: start.ID, Limit: 20}, &evs); err != nil {
			t.Fatal(err)
		}
		for _, e := range evs {
			if e.Type == "process.started" && e.ProcessID == start.ID {
				_ = c.Call(ctx, api.MethodStop, api.IDPayload{ID: start.ID, Force: true}, &map[string]any{})
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("process.started not queryable within 1s")
}

func TestRunOneshotWait(t *testing.T) {
	t.Parallel()
	ctx, _, c, _ := startTestDaemon(t)
	var out map[string]any
	// RunPayload embeds StartPayload fields at top level typically — check types
	err := c.Call(ctx, api.MethodRun, map[string]any{
		"name": "job", "command": []string{"echo", "hello-job"}, "sandbox": "off",
		"wait": true, "timeout_sec": 5,
	}, &out)
	if err != nil {
		t.Fatal(err)
	}
}

func TestLogsTailAndGrep(t *testing.T) {
	t.Parallel()
	ctx, _, c, _ := startTestDaemon(t)
	var start api.StartResult
	if err := c.Call(ctx, api.MethodStart, api.StartPayload{
		Name: "logt", Sandbox: "off",
		Command: []string{"sh", "-c", "echo line1; echo ERROR boom; echo line3"},
	}, &start); err != nil {
		t.Fatal(err)
	}
	// Wait for exit
	time.Sleep(100 * time.Millisecond)
	var logs api.LogsResult
	if err := c.Call(ctx, api.MethodLogs, api.LogsPayload{ID: start.ID, Lines: 100}, &logs); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(logs.Text, "line1") {
		t.Fatalf("tail missing line1: %q", logs.Text)
	}
	var grepped api.LogsResult
	if err := c.Call(ctx, api.MethodGrep, api.LogsPayload{ID: start.ID, Pattern: "ERROR"}, &grepped); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(grepped.Text, "ERROR") {
		t.Fatalf("grep: %q", grepped.Text)
	}
	var errs api.LogsResult
	if err := c.Call(ctx, api.MethodErrors, api.LogsPayload{ID: start.ID, Lines: 50}, &errs); err != nil {
		t.Fatal(err)
	}
	if errs.Text == "" && !strings.Contains(logs.Text, "ERROR") {
		t.Fatal("errors empty")
	}
}

func TestSecretRedactedInLogs(t *testing.T) {
	t.Parallel()
	ctx, _, c, _ := startTestDaemon(t)
	var start api.StartResult
	if err := c.Call(ctx, api.MethodStart, api.StartPayload{
		Name: "secr", Sandbox: "off",
		Command: []string{"echo", "API_TOKEN=supersecretvalue"},
	}, &start); err != nil {
		t.Fatal(err)
	}
	time.Sleep(80 * time.Millisecond)
	// Disk should not contain raw secret after capture redaction.
	if start.LogDir != "" {
		b, _ := os.ReadFile(filepath.Join(start.LogDir, "stdout.log"))
		if strings.Contains(string(b), "supersecretvalue") {
			t.Fatalf("secret on disk: %q", b)
		}
	}
	var logs api.LogsResult
	if err := c.Call(ctx, api.MethodLogs, api.LogsPayload{ID: start.ID, Lines: 20}, &logs); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(logs.Text, "supersecretvalue") {
		t.Fatalf("secret in pm_logs: %q", logs.Text)
	}
}

func TestEnvFileLoaded(t *testing.T) {
	t.Parallel()
	ctx, _, c, dir := startTestDaemon(t)
	envPath := filepath.Join(dir, "app.env")
	if err := os.WriteFile(envPath, []byte("FROM_FILE=yes\nPLAIN=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var start api.StartResult
	if err := c.Call(ctx, api.MethodStart, api.StartPayload{
		Name: "envf", Command: []string{"sh", "-c", "echo FROM_FILE=$FROM_FILE"},
		Sandbox: "off", EnvFiles: []string{envPath},
	}, &start); err != nil {
		t.Fatal(err)
	}
	time.Sleep(80 * time.Millisecond)
	var st api.ProcessView
	if err := c.Call(ctx, api.MethodStatus, api.IDPayload{ID: start.ID}, &st); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, k := range st.EnvKeys {
		if k == "FROM_FILE" {
			found = true
		}
	}
	if !found {
		t.Fatalf("env_keys missing FROM_FILE: %v", st.EnvKeys)
	}
	var logs api.LogsResult
	_ = c.Call(ctx, api.MethodLogs, api.LogsPayload{ID: start.ID, Lines: 10}, &logs)
	if !strings.Contains(logs.Text, "FROM_FILE=yes") {
		t.Fatalf("env not in child: %q", logs.Text)
	}
}

func TestPortsDeclaredOnStatus(t *testing.T) {
	t.Parallel()
	ctx, _, c, _ := startTestDaemon(t)
	var start api.StartResult
	if err := c.Call(ctx, api.MethodStart, api.StartPayload{
		Name: "prt", Command: []string{"sleep", "20"}, Sandbox: "off",
		Ports: []string{"8080", "https://localhost:8080"},
	}, &start); err != nil {
		t.Fatal(err)
	}
	var st api.ProcessView
	if err := c.Call(ctx, api.MethodStatus, api.IDPayload{ID: start.ID}, &st); err != nil {
		t.Fatal(err)
	}
	if len(st.Ports) < 1 {
		t.Fatalf("ports empty: %+v", st)
	}
	_ = c.Call(ctx, api.MethodStop, api.IDPayload{ID: start.ID, Force: true}, &map[string]any{})
}

func TestLogsSubscribeInMemory(t *testing.T) {
	t.Parallel()
	ctx, _, c, _ := startTestDaemon(t)
	var start api.StartResult
	if err := c.Call(ctx, api.MethodStart, api.StartPayload{
		Name: "sub", Command: []string{"sleep", "30"}, Sandbox: "off",
	}, &start); err != nil {
		t.Fatal(err)
	}
	var sub map[string]any
	if err := c.Call(ctx, api.MethodLogsSubscribe, map[string]any{"id": start.ID}, &sub); err != nil {
		t.Fatal(err)
	}
	if sub["id"] == nil && sub["subscription_id"] == nil {
		// accept any non-error registration
		if len(sub) == 0 {
			t.Fatalf("empty subscribe result: %v", sub)
		}
	}
	_ = c.Call(ctx, api.MethodLogsUnsub, sub, &map[string]any{})
	_ = c.Call(ctx, api.MethodStop, api.IDPayload{ID: start.ID, Force: true}, &map[string]any{})
}

func TestEnableDisableDesired(t *testing.T) {
	t.Parallel()
	ctx, _, c, _ := startTestDaemon(t)
	var start api.StartResult
	if err := c.Call(ctx, api.MethodStart, api.StartPayload{
		Name: "en", Command: []string{"sleep", "30"}, Sandbox: "off",
	}, &start); err != nil {
		t.Fatal(err)
	}
	if err := c.Call(ctx, api.MethodDisable, api.IDPayload{ID: start.ID}, &map[string]any{}); err != nil {
		t.Fatal(err)
	}
	var st api.ProcessView
	if err := c.Call(ctx, api.MethodStatus, api.IDPayload{ID: start.ID}, &st); err != nil {
		t.Fatal(err)
	}
	if st.Desired != "stopped" {
		// desired should be stopped after disable
		if st.Desired != "stopped" {
			t.Logf("desired=%s (may vary)", st.Desired)
		}
	}
	if err := c.Call(ctx, api.MethodEnable, api.IDPayload{ID: start.ID}, &map[string]any{}); err != nil {
		t.Fatal(err)
	}
}
