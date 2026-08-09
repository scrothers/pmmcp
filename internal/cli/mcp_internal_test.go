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

package cli

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scrothers/pmmcp/internal/api"
	pmmcpv1 "github.com/scrothers/pmmcp/internal/api/gen/pmmcp/v1"
	"github.com/scrothers/pmmcp/internal/ipc"
	"github.com/scrothers/pmmcp/internal/testsock"
	"google.golang.org/grpc"
)

type mcpFakeDaemon struct {
	pmmcpv1.UnimplementedDaemonServer
}

func (mcpFakeDaemon) Call(_ context.Context, req *pmmcpv1.CallRequest) (*pmmcpv1.CallResponse, error) {
	raw := func(s string) (*pmmcpv1.CallResponse, error) {
		return &pmmcpv1.CallResponse{Ok: true, Payload: []byte(s)}, nil
	}
	switch req.GetMethod() {
	case api.MethodHello:
		b, _ := json.Marshal(api.HelloResult{APIVersion: api.APIVersion, DaemonVersion: "9.9.9"})
		return &pmmcpv1.CallResponse{Ok: true, Payload: b}, nil
	case api.MethodList:
		return raw(`[]`)
	case api.MethodEvents:
		return raw(`[]`)
	case api.MethodLogs, api.MethodGrep, api.MethodErrors:
		return raw(`{"text":"logs\n"}`)
	default:
		return raw(`{}`)
	}
}

func startMCPDaemon(t *testing.T) string {
	t.Helper()
	sock := testsock.Path(t)
	ln, err := ipc.Listen(sock)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	s := grpc.NewServer()
	pmmcpv1.RegisterDaemonServer(s, mcpFakeDaemon{})
	go func() { _ = s.Serve(ln) }()
	t.Cleanup(s.Stop)
	return sock
}

func TestMCPCallToolPaths(t *testing.T) {
	endpoint := startMCPDaemon(t)
	ctx := context.Background()
	cases := []struct {
		name string
		args map[string]any
	}{
		{"pm_start", map[string]any{"name": "api", "command": []any{"./bin/api"}, "sandbox": "strict"}},
		{"pm_run", map[string]any{"command": []any{"./bin/job"}, "cwd": "/tmp", "wait": true, "timeout_sec": float64(5)}},
		{"pm_stop", map[string]any{"name": "api"}},
		{"pm_status", map[string]any{"id": "proc-1"}},
		{"pm_wait", map[string]any{"id": "proc-1", "timeout_sec": float64(5)}},
		{"pm_list", map[string]any{"project": "p"}},
		{"pm_logs", map[string]any{"name": "api", "lines": float64(10)}},
		{"pm_grep", map[string]any{"name": "api", "pattern": "x"}},
		{"pm_daemon_info", map[string]any{}},
		{"pm_whoami", map[string]any{}},
		{"pm_events", map[string]any{"limit": float64(5)}},
		{"pm_metrics_snapshot", map[string]any{}},
	}
	for _, tc := range cases {
		out, err := mcpCall(ctx, endpoint, tc.name, tc.args)
		if err != nil {
			t.Errorf("mcpCall(%s) = %v", tc.name, err)
		}
		if strings.TrimSpace(out) == "" {
			t.Errorf("mcpCall(%s) empty output", tc.name)
		}
	}
}

func TestMCPCallUnknownTool(t *testing.T) {
	endpoint := startMCPDaemon(t)
	if _, err := mcpCall(context.Background(), endpoint, "pm_not_a_tool", nil); err == nil {
		t.Fatal("want error for unknown tool")
	}
}

func TestMCPCallDaemonDown(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "absent.sock")
	if _, err := mcpCall(context.Background(), sock, "pm_list", nil); err == nil {
		t.Fatal("want daemon-down error")
	}
}
