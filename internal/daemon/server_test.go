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
	"github.com/scrothers/pmmcp/internal/ipc"
)

func TestDaemonStartStopLogs(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	sock := filepath.Join(dir, "pmmcpd.sock")
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
	cfg.Sandbox.Default = "off" // avoid project-root strict requirement for sleep

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	srv, err := daemon.New(ctx, daemon.Options{Config: cfg, DBPath: filepath.Join(dir, "db.sqlite")})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe(ctx) }()

	// wait for socket
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
	var start api.StartResult
	if err := client.Call(ctx, api.MethodStart, api.StartPayload{
		Name: "sleeper", Command: []string{"sleep", "30"}, Sandbox: "off",
	}, &start); err != nil {
		t.Fatalf("start: %v", err)
	}
	if start.PID <= 0 || start.ID == "" {
		t.Fatalf("%+v", start)
	}
	var list []api.ProcessView
	if err := client.Call(ctx, api.MethodList, api.ListPayload{}, &list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("list %d", len(list))
	}
	if err := client.Call(ctx, api.MethodStop, api.IDPayload{ID: start.ID, TimeoutSec: 2}, &map[string]any{}); err != nil {
		t.Fatalf("stop: %v", err)
	}
	cancel()
	select {
	case <-errCh:
	case <-time.After(2 * time.Second):
	}
}
