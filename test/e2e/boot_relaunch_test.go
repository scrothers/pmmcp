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
	"path/filepath"
	"testing"
	"time"

	"github.com/scrothers/pmmcp/internal/api"
	"github.com/scrothers/pmmcp/internal/config"
	"github.com/scrothers/pmmcp/internal/daemon"
	"github.com/scrothers/pmmcp/internal/ipc"
)

// TestBootRelaunchAfterDaemonRestart proves desired=running units come back
// after a daemon process restart (supervision-boot-relaunch).
func TestBootRelaunchAfterDaemonRestart(t *testing.T) {
	dir := t.TempDir()
	sock := filepath.Join(dir, "pmmcpd.sock")
	db := filepath.Join(dir, "db.sqlite")
	state := filepath.Join(dir, "state")

	startDaemon := func() (context.CancelFunc, *ipc.Client) {
		cfg, err := config.Load(config.LoadOptions{
			GOOS: "linux", Home: dir,
			LookupEnv: func(string) (string, bool) { return "", false },
		})
		if err != nil {
			t.Fatal(err)
		}
		cfg.StateDir = state
		cfg.IPC.Endpoint = sock
		cfg.Sandbox.Default = "off"
		cfg.Relaunch.Enabled = true
		ctx, cancel := context.WithCancel(context.Background())
		srv, err := daemon.New(ctx, daemon.Options{Config: cfg, DBPath: db})
		if err != nil {
			cancel()
			t.Fatal(err)
		}
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
			cancel()
			_ = srv.Close()
			t.Fatal(err)
		}
		t.Cleanup(func() {
			_ = c.Close()
			cancel()
			_ = srv.Close()
		})
		return cancel, c
	}

	cancel1, c1 := startDaemon()
	var start api.StartResult
	if err := c1.Call(context.Background(), api.MethodStart, api.StartPayload{
		Name: "persist", Command: []string{"sleep", "120"}, Sandbox: "off",
	}, &start); err != nil {
		t.Fatal(err)
	}
	// Mark durable desired=running.
	if err := c1.Call(context.Background(), api.MethodEnable, api.IDPayload{ID: start.ID}, &map[string]any{}); err != nil {
		t.Fatal(err)
	}
	// Kill first daemon (simulate reboot of pmmcpd only).
	cancel1()
	_ = c1.Close()
	time.Sleep(100 * time.Millisecond)

	// Second daemon with same state/db should relaunch.
	_, c2 := startDaemon()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var st api.ProcessView
		if err := c2.Call(context.Background(), api.MethodStatus, api.IDPayload{ID: start.ID}, &st); err == nil {
			if st.Status == "running" || st.Status == "starting" {
				_ = c2.Call(context.Background(), api.MethodStop, api.IDPayload{ID: start.ID, Force: true}, &map[string]any{})
				return
			}
		}
		// also try by name after relaunch
		if err := c2.Call(context.Background(), api.MethodStatus, api.IDPayload{Name: "persist"}, &st); err == nil {
			if st.Status == "running" || st.Status == "starting" {
				_ = c2.Call(context.Background(), api.MethodStop, api.IDPayload{ID: st.ID, Force: true}, &map[string]any{})
				return
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("expected relaunch of enabled process after daemon restart")
}
