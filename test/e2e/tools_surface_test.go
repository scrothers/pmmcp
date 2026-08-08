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

//go:build e2e

package e2e_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/scrothers/pmmcp/internal/api"
	"github.com/scrothers/pmmcp/internal/cli"
	"github.com/scrothers/pmmcp/internal/config"
	"github.com/scrothers/pmmcp/internal/daemon"
	"github.com/scrothers/pmmcp/internal/ipc"
)

// TestE2EAllCatalogToolsCallDaemon asserts every ToolMethod can be invoked without "unknown method".
func TestE2EAllCatalogToolsCallDaemon(t *testing.T) {
	dir := t.TempDir()
	sock := filepath.Join(dir, "pmmcpd.sock")
	cfg, err := config.Load(config.LoadOptions{GOOS: "linux", Home: dir, LookupEnv: func(string) (string, bool) { return "", false }})
	if err != nil {
		t.Fatal(err)
	}
	cfg.StateDir = filepath.Join(dir, "st")
	cfg.IPC.Endpoint = sock
	cfg.Sandbox.Default = "off"
	cfg.Relaunch.Enabled = false
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	srv, err := daemon.New(ctx, daemon.Options{Config: cfg, DBPath: filepath.Join(dir, "db")})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	go func() { _ = srv.ListenAndServe(ctx) }()
	var c *ipc.Client
	for i := 0; i < 80; i++ {
		c, err = ipc.Dial(ctx, sock)
		if err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if c == nil {
		t.Fatal(err)
	}
	defer c.Close()

	var start api.StartResult
	if err := c.Call(ctx, api.MethodStart, api.StartPayload{
		Name: "surf", Command: []string{"sleep", "30"}, Sandbox: "off", Ports: []string{"9"},
	}, &start); err != nil {
		t.Fatal(err)
	}

	// Ensure every catalog tool maps to a method that is not "unknown".
	for _, name := range cli.ToolNames() {
		method, ok := cli.ToolMethod[name]
		if !ok {
			t.Fatalf("tool %s missing method", name)
		}
		payload := map[string]any{"id": start.ID, "name": "surf", "pattern": "x", "url": "https://example.com/h", "path": dir, "yaml": "apiVersion: pmmcp.dev/v1alpha1\nkind: Project\nservices: {}\n", "command": []string{"true"}, "project_id": "p", "value": "v", "members": []map[string]any{{"name": "surf"}}}
		err := c.Call(ctx, method, payload, &map[string]any{})
		if err != nil && containsUnknown(err.Error()) {
			t.Errorf("%s -> %s: %v", name, method, err)
		}
	}
	_ = c.Call(ctx, api.MethodStop, map[string]any{"id": start.ID, "timeout_sec": 2}, &map[string]any{})
}

func containsUnknown(s string) bool {
	return len(s) > 0 && (contains(s, "unknown method") || contains(s, "unknown tool"))
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || (len(s) > 0 && indexOf(s, sub) >= 0))
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// silence unused import if build tags strip
var _ = os.Environ
var _ = exec.Command
