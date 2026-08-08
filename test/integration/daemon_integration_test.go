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

//go:build integration

package integration_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/scrothers/pmmcp/internal/api"
	"github.com/scrothers/pmmcp/internal/config"
	"github.com/scrothers/pmmcp/internal/daemon"
	"github.com/scrothers/pmmcp/internal/ipc"
)

func TestIntegrationStartGroupHealth(t *testing.T) {
	dir := t.TempDir()
	sock := filepath.Join(dir, "s.sock")
	cfg, err := config.Load(config.LoadOptions{GOOS: "linux", Home: dir, LookupEnv: func(string) (string, bool) { return "", false }})
	if err != nil {
		t.Fatal(err)
	}
	cfg.StateDir = filepath.Join(dir, "state")
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
	for i := 0; i < 50; i++ {
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
	var st api.StartResult
	if err := c.Call(ctx, api.MethodStart, api.StartPayload{Name: "i", Command: []string{"sleep", "5"}, Sandbox: "off"}, &st); err != nil {
		t.Fatal(err)
	}
	if st.ID == "" || st.PID <= 0 {
		t.Fatalf("%+v", st)
	}
	var g map[string]any
	if err := c.Call(ctx, api.MethodGroupCreate, map[string]any{
		"name": "ig", "members": []map[string]any{{"name": "i"}},
	}, &g); err != nil {
		t.Fatal(err)
	}
	if err := c.Call(ctx, api.MethodHealthCheck, map[string]any{"id": st.ID}, &map[string]any{}); err != nil {
		t.Fatal(err)
	}
	_ = c.Call(ctx, api.MethodStop, map[string]any{"id": st.ID, "timeout_sec": 2}, &map[string]any{})
}
