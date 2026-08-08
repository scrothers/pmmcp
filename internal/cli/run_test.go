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

package cli_test

import (
	"context"
	"encoding/json"
	"net"
	"path/filepath"
	"testing"

	"github.com/scrothers/pmmcp/internal/api"
	pmmcpv1 "github.com/scrothers/pmmcp/internal/api/gen/pmmcp/v1"
	"github.com/scrothers/pmmcp/internal/cli"
	"google.golang.org/grpc"
)

// fakeDaemon answers every IPC method with a shape-appropriate canned payload.
type fakeDaemon struct {
	pmmcpv1.UnimplementedDaemonServer
}

func (fakeDaemon) Call(_ context.Context, req *pmmcpv1.CallRequest) (*pmmcpv1.CallResponse, error) {
	raw := func(s string) (*pmmcpv1.CallResponse, error) {
		return &pmmcpv1.CallResponse{Ok: true, Payload: []byte(s)}, nil
	}
	switch req.GetMethod() {
	case api.MethodHello:
		b, _ := json.Marshal(api.HelloResult{APIVersion: api.APIVersion, DaemonVersion: "9.9.9"})
		return &pmmcpv1.CallResponse{Ok: true, Payload: b}, nil
	case api.MethodList, api.MethodEvents:
		return raw(`[]`)
	case api.MethodLogs, api.MethodGrep, api.MethodErrors:
		return raw(`{"text":"log output\n"}`)
	case api.MethodStart:
		b, _ := json.Marshal(api.StartResult{ID: "proc-1", Name: "api", PID: 42, Status: "running", LogDir: "/logs"})
		return &pmmcpv1.CallResponse{Ok: true, Payload: b}, nil
	default:
		return raw(`{}`)
	}
}

func startDaemon(t *testing.T) string {
	t.Helper()
	sock := filepath.Join(t.TempDir(), "d.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	s := grpc.NewServer()
	pmmcpv1.RegisterDaemonServer(s, fakeDaemon{})
	go func() { _ = s.Serve(ln) }()
	t.Cleanup(s.Stop)
	return sock
}

func TestRunDispatch(t *testing.T) {
	sock := startDaemon(t)
	t.Setenv("PMMCP_IPC_ENDPOINT", sock)
	ctx := context.Background()

	t.Setenv("HOME", t.TempDir())
	binDir := t.TempDir()
	binPath := filepath.Join(binDir, "pmmcpd")

	ok := [][]string{
		{"version"},
		{"doctor"},
		{"install-service", "--bin", binPath},
		{"uninstall-service"},
		{"--json", "list"},
		{"list", "--all"},
		{"status", "web"},
		{"status", "proc-1"},
		{"stop", "web"},
		{"restart", "proc-1"},
		{"remove", "web"},
		{"update", "--name", "web", "--restart", "true"},
		{"logs", "web"},
		{"grep", "web", "pattern"},
		{"errors", "web"},
		{"logs", "export", "--name", "web"},
		{"logs", "ship", "export_path=/x", "sink_path=/y"},
		{"events", "proc-1"},
		{"run", "--name", "job", "--", "echo", "hi"},
		{"wait", "proc-1"},
		{"enable", "web"},
		{"disable", "web"},
		{"health", "web"},
		{"start", "--name", "api", "--", "./bin/api"},
		{"group", "create", "--name", "stack"},
		{"group", "list"},
		{"group", "status", "stack"},
		{"group", "start", "stack"},
		{"group", "stop", "stack"},
		{"group", "restart", "stack"},
		{"group", "remove", "stack"},
		{"profile", "list"},
		{"profile", "get", "dev"},
		{"profile", "create", "--name", "dev"},
		{"profile", "update", "--name", "dev"},
		{"profile", "delete", "dev"},
		{"profile", "use", "dev"},
		{"session", "info"},
		{"session", "end"},
		{"share", "--target", "proc-1"},
		{"unshare", "--target", "proc-1"},
		{"validate", "path=/x"},
		{"apply", "path=/x"},
		{"diff", "path=/x"},
		{"declare", "show"},
		{"audit", "types=declare.apply"},
		{"runtime"},
		{"webhook", "create", "url=https://example.com"},
		{"webhook", "list"},
		{"webhook", "update", "wh-1"},
		{"webhook", "delete", "wh-1"},
		{"webhook", "test", "wh-1"},
		{"metrics"},
		{"sandbox-profiles"},
		{"ports", "web"},
		{"whoami"},
		{"reload"},
		{"daemon-info"},
		{"watch", "set", "--name", "web", "path=/x"},
		{"watch", "status"},
		{"project", "current"},
		{"project", "list"},
		{"import", "data=web: ./a"},
		{"secret", "list"},
		{"secret", "check", "name=db"},
		{"list", "--include-exited", "--project", "p", "--json"},
		{"logs", "proc-1"},
		{"events"},
		{"run", "--cwd", "/x", "--name", "j", "echo", "hi"},
		{"ports"},
	}
	for _, args := range ok {
		if err := cli.Run(ctx, args); err != nil {
			t.Errorf("Run(%v) = %v, want nil", args, err)
		}
	}
}

func TestRunErrors(t *testing.T) {
	sock := startDaemon(t)
	t.Setenv("PMMCP_IPC_ENDPOINT", sock)
	ctx := context.Background()
	bad := [][]string{
		{},
		{"nonsense-command"},
		{"start"},                // missing --name
		{"start", "--name", "x"}, // missing command
		{"start", "--bogus"},     // unknown flag
		{"group"},                // missing subcommand
		{"group", "frobnicate"},  // unknown subcommand
		{"declare", "bogus"},     // unknown declare subcommand
		{"session", "bogus"},     // unknown session subcommand
		{"secret", "bogus"},      // unknown secret subcommand
		{"watch", "bogus"},       // unknown watch subcommand
		{"project", "bogus"},     // unknown project subcommand
		{"profile", "bogus"},     // unknown profile subcommand
		{"webhook", "bogus"},     // unknown webhook subcommand
	}
	for _, args := range bad {
		if err := cli.Run(ctx, args); err == nil {
			t.Errorf("Run(%v) = nil, want error", args)
		}
	}
}

func TestRunDaemonDown(t *testing.T) {
	t.Setenv("PMMCP_IPC_ENDPOINT", filepath.Join(t.TempDir(), "absent.sock"))
	err := cli.Run(context.Background(), []string{"list"})
	if err == nil {
		t.Fatal("want daemon_unavailable error, got nil")
	}
}
