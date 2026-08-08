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

package mcp_test

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scrothers/pmmcp/internal/api"
	pmmcpv1 "github.com/scrothers/pmmcp/internal/api/gen/pmmcp/v1"
	"github.com/scrothers/pmmcp/internal/domain"
	"github.com/scrothers/pmmcp/internal/mcp"
	"google.golang.org/grpc"
)

// fakeDaemon answers a subset of IPC methods with canned JSON payloads.
type fakeDaemon struct {
	pmmcpv1.UnimplementedDaemonServer
}

func (fakeDaemon) Call(_ context.Context, req *pmmcpv1.CallRequest) (*pmmcpv1.CallResponse, error) {
	reply := func(v any) (*pmmcpv1.CallResponse, error) {
		b, _ := json.Marshal(v)
		return &pmmcpv1.CallResponse{Ok: true, Payload: b}, nil
	}
	switch req.GetMethod() {
	case api.MethodHello:
		return reply(api.HelloResult{APIVersion: api.APIVersion, DaemonVersion: "9.9.9"})
	case api.MethodList:
		return reply([]api.ProcessView{
			{ID: "proc-1", Name: "api", Status: "running", Ports: []string{"8080"}},
			{ID: "proc-2", Name: "web", Status: "running"},
		})
	case api.MethodDaemonInfo:
		return reply(api.DaemonInfoResult{Version: "9.9.9", APIVersion: api.APIVersion})
	case api.MethodProjectCurrent:
		return reply(api.ProjectResult{Root: "/proj", Key: "proj-abc"})
	case api.MethodProjectList:
		return reply(api.ProjectListResult{Projects: []api.ProjectEntry{{Key: "proj-abc", Root: "/proj"}}})
	case api.MethodEvents:
		return reply([]api.EventView{{ID: "evt-1", Type: "process.started"}})
	case api.MethodGroupStatus:
		return reply(api.GroupView{ID: "grp-1", Name: "stack", Phase: "running"})
	case api.MethodGroupList:
		return reply([]api.GroupView{{ID: "grp-1", Name: "stack"}})
	case api.MethodStatus:
		return reply(api.ProcessView{ID: "proc-1", Name: "api", Status: "running"})
	case api.MethodLogs:
		return reply(api.LogsResult{Text: "hello logs\n"})
	default:
		return &pmmcpv1.CallResponse{Ok: false, ErrorCode: "unimplemented", Error: req.GetMethod()}, nil
	}
}

func startFakeDaemon(t *testing.T) string {
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

// flakyDaemon answers like fakeDaemon except methods listed in fail return an
// IPC-level error, letting tests exercise the error branch of a specific
// daemon call without tearing down the whole connection (Hello still
// succeeds unless api.MethodHello itself is in fail).
type flakyDaemon struct {
	pmmcpv1.UnimplementedDaemonServer
	fail map[string]bool
}

func (d flakyDaemon) Call(ctx context.Context, req *pmmcpv1.CallRequest) (*pmmcpv1.CallResponse, error) {
	if d.fail[req.GetMethod()] {
		return &pmmcpv1.CallResponse{Ok: false, ErrorCode: "internal", Error: "forced failure: " + req.GetMethod()}, nil
	}
	return fakeDaemon{}.Call(ctx, req)
}

func startFlakyDaemon(t *testing.T, fail map[string]bool) string {
	t.Helper()
	sock := filepath.Join(t.TempDir(), "flaky.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	s := grpc.NewServer()
	pmmcpv1.RegisterDaemonServer(s, flakyDaemon{fail: fail})
	go func() { _ = s.Serve(ln) }()
	t.Cleanup(s.Stop)
	return sock
}

func TestReadResourceDaemonURIs(t *testing.T) {
	t.Parallel()
	endpoint := startFakeDaemon(t)
	ctx := context.Background()
	cases := map[string]string{
		"pmmcp://processes":          `"proc-1"`,
		"pmmcp://daemon":             `"9.9.9"`,
		"pmmcp://project/current":    `"proj-abc"`,
		"pmmcp://events/recent":      `"evt-1"`,
		"pmmcp://ports":              `"8080"`,
		"pmmcp://group/stack":        `"running"`,
		"pmmcp://project/proj-abc":   `"proj-abc"`,
		"pmmcp://process/proc-1":     `"api"`,
		"pmmcp://process/proc-1/log": "hello logs",
	}
	for uri, want := range cases {
		t.Run(uri, func(t *testing.T) {
			t.Parallel()
			got, err := mcp.ReadResource(ctx, endpoint, uri)
			if err != nil {
				t.Fatalf("ReadResource(%q): %v", uri, err)
			}
			if !strings.Contains(got, want) {
				t.Fatalf("ReadResource(%q) = %q, want substring %q", uri, got, want)
			}
		})
	}
}

func TestReadResourceUnknownURI(t *testing.T) {
	t.Parallel()
	endpoint := startFakeDaemon(t)
	_, err := mcp.ReadResource(context.Background(), endpoint, "pmmcp://bogus")
	var de *domain.Error
	if !errors.As(err, &de) || de.Code != domain.CodeInvalidArgument {
		t.Fatalf("want invalid_argument domain error, got %v", err)
	}
}

func TestReadResourceProjectNotFound(t *testing.T) {
	t.Parallel()
	endpoint := startFakeDaemon(t)
	_, err := mcp.ReadResource(context.Background(), endpoint, "pmmcp://project/nope")
	var de *domain.Error
	if !errors.As(err, &de) || de.Code != domain.CodeNotFound {
		t.Fatalf("want not_found, got %v", err)
	}
}

func TestReadResourceDeclare(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "pmmcp.yaml"), []byte("apiVersion: pmmcp.dev/v1alpha1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	got, err := mcp.ReadResource(context.Background(), "unused", "pmmcp://declare")
	if err != nil {
		t.Fatalf("declare: %v", err)
	}
	if !strings.Contains(got, "pmmcp.dev/v1alpha1") {
		t.Fatalf("declare content = %q", got)
	}
}

func TestReadResourceDeclareMissing(t *testing.T) {
	t.Chdir(t.TempDir())
	_, err := mcp.ReadResource(context.Background(), "unused", "pmmcp://declare")
	var de *domain.Error
	if !errors.As(err, &de) || de.Code != domain.CodeNotFound {
		t.Fatalf("want not_found for missing declare, got %v", err)
	}
}

func TestReadResourceDocs(t *testing.T) {
	t.Parallel()
	for _, uri := range []string{"pmmcp://docs/error-codes", "pmmcp://docs/tool-index"} {
		got, err := mcp.ReadResource(context.Background(), "unused", uri)
		if err != nil {
			t.Fatalf("%s: %v", uri, err)
		}
		if strings.TrimSpace(got) == "" {
			t.Fatalf("%s returned empty", uri)
		}
	}
}

func TestListResourcesDaemonUp(t *testing.T) {
	t.Parallel()
	endpoint := startFakeDaemon(t)
	res, err := mcp.ListResources(context.Background(), endpoint)
	if err != nil {
		t.Fatalf("ListResources: %v", err)
	}
	var hasProc, hasGroup bool
	for _, r := range res {
		if r.URI == "pmmcp://process/proc-1" {
			hasProc = true
		}
		if r.URI == "pmmcp://group/stack" {
			hasGroup = true
		}
	}
	if !hasProc || !hasGroup {
		t.Fatalf("dynamic resources missing: proc=%v group=%v", hasProc, hasGroup)
	}
}

func TestListResourcesDaemonDown(t *testing.T) {
	t.Parallel()
	sock := filepath.Join(t.TempDir(), "absent.sock")
	res, err := mcp.ListResources(context.Background(), sock)
	if err == nil {
		t.Fatal("want daemon-down error, got nil")
	}
	if len(res) == 0 {
		t.Fatal("want static resources even when daemon down")
	}
}

func TestListResourcesListCallError(t *testing.T) {
	t.Parallel()
	endpoint := startFlakyDaemon(t, map[string]bool{api.MethodList: true})
	res, err := mcp.ListResources(context.Background(), endpoint)
	if err == nil {
		t.Fatal("want error when the list RPC fails, got nil")
	}
	if len(res) != len(mcp.StaticResources()) {
		t.Fatalf("want only static resources back on list error, got %d", len(res))
	}
}

func TestResourceTemplates(t *testing.T) {
	t.Parallel()
	templates := mcp.ResourceTemplates()
	want := map[string]bool{
		"pmmcp://project/{id}":             true,
		"pmmcp://process/{name_or_id}":     true,
		"pmmcp://process/{name_or_id}/log": true,
		"pmmcp://group/{name}":             true,
	}
	if len(templates) != len(want) {
		t.Fatalf("template count = %d, want %d", len(templates), len(want))
	}
	for _, tpl := range templates {
		if !want[tpl.URITemplate] {
			t.Errorf("unexpected template %q", tpl.URITemplate)
		}
		if tpl.Name == "" {
			t.Errorf("template %q missing name", tpl.URITemplate)
		}
	}
}

func TestReadResourceDaemonDialError(t *testing.T) {
	t.Parallel()
	sock := filepath.Join(t.TempDir(), "absent.sock")
	_, err := mcp.ReadResource(context.Background(), sock, "pmmcp://processes")
	if err == nil {
		t.Fatal("want dial error, got nil")
	}
}

func TestReadResourceProcessLogCallError(t *testing.T) {
	t.Parallel()
	endpoint := startFlakyDaemon(t, map[string]bool{api.MethodLogs: true})
	_, err := mcp.ReadResource(context.Background(), endpoint, "pmmcp://process/proc-1/log")
	if err == nil {
		t.Fatal("want error when the logs RPC fails, got nil")
	}
}

func TestReadResourceCallJSONError(t *testing.T) {
	t.Parallel()
	endpoint := startFlakyDaemon(t, map[string]bool{api.MethodDaemonInfo: true})
	_, err := mcp.ReadResource(context.Background(), endpoint, "pmmcp://daemon")
	if err == nil {
		t.Fatal("want error when the daemon-info RPC fails, got nil")
	}
}

func TestReadResourceProjectByIDCallError(t *testing.T) {
	t.Parallel()
	endpoint := startFlakyDaemon(t, map[string]bool{api.MethodProjectList: true})
	_, err := mcp.ReadResource(context.Background(), endpoint, "pmmcp://project/proj-abc")
	if err == nil {
		t.Fatal("want error when the project-list RPC fails, got nil")
	}
}

func TestReadResourcePortsCallError(t *testing.T) {
	t.Parallel()
	endpoint := startFlakyDaemon(t, map[string]bool{api.MethodList: true})
	_, err := mcp.ReadResource(context.Background(), endpoint, "pmmcp://ports")
	if err == nil {
		t.Fatal("want error when the list RPC fails, got nil")
	}
}

func TestReadResourceProcessByName(t *testing.T) {
	t.Parallel()
	endpoint := startFakeDaemon(t)
	got, err := mcp.ReadResource(context.Background(), endpoint, "pmmcp://process/api")
	if err != nil {
		t.Fatalf("ReadResource by name: %v", err)
	}
	if !strings.Contains(got, `"api"`) {
		t.Fatalf("ReadResource by name = %q, want substring %q", got, `"api"`)
	}
}

func TestReadResourceProcessLogByName(t *testing.T) {
	t.Parallel()
	endpoint := startFakeDaemon(t)
	got, err := mcp.ReadResource(context.Background(), endpoint, "pmmcp://process/api/log")
	if err != nil {
		t.Fatalf("ReadResource log by name: %v", err)
	}
	if !strings.Contains(got, "hello logs") {
		t.Fatalf("ReadResource log by name = %q, want substring %q", got, "hello logs")
	}
}

// TestReadDeclareGetwdError removes the process's current directory out from
// under it so os.Getwd fails, exercising readDeclare's getwd error branch.
// Mutates process-global working directory state, so it cannot run in
// parallel with other tests that depend on cwd.
func TestReadDeclareGetwdError(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.RemoveAll(dir); err != nil {
		t.Fatalf("remove cwd: %v", err)
	}
	_, err := mcp.ReadResource(context.Background(), "unused", "pmmcp://declare")
	var de *domain.Error
	if !errors.As(err, &de) || de.Code != domain.CodeInternal {
		t.Fatalf("want internal error for missing cwd, got %v", err)
	}
}

func TestListPrompts(t *testing.T) {
	t.Parallel()
	prompts := mcp.ListPrompts()
	if len(prompts) != 5 {
		t.Fatalf("prompt count = %d, want 5", len(prompts))
	}
	want := map[string]bool{
		"pmmcp_start_safe": true, "pmmcp_debug_crash": true, "pmmcp_apply_stack": true,
		"pmmcp_import_compose": true, "pmmcp_oneshot_task": true,
	}
	for _, p := range prompts {
		if !want[p.Name] {
			t.Errorf("unexpected prompt %q", p.Name)
		}
		if p.Description == "" {
			t.Errorf("prompt %q missing description", p.Name)
		}
	}
}

func TestGetPrompt(t *testing.T) {
	t.Parallel()
	for _, p := range mcp.ListPrompts() {
		text, err := mcp.GetPrompt(p.Name, map[string]string{
			"name": "api", "argv_json": `["./bin/api"]`, "path": "./Procfile",
		})
		if err != nil {
			t.Fatalf("GetPrompt(%q): %v", p.Name, err)
		}
		if strings.TrimSpace(text) == "" {
			t.Fatalf("GetPrompt(%q) empty", p.Name)
		}
	}
}

func TestGetPromptNilArgs(t *testing.T) {
	t.Parallel()
	text, err := mcp.GetPrompt("pmmcp_debug_crash", nil)
	if err != nil {
		t.Fatalf("GetPrompt with nil args: %v", err)
	}
	if strings.TrimSpace(text) == "" {
		t.Fatal("GetPrompt with nil args returned empty text")
	}
}

func TestGetPromptWithOverrides(t *testing.T) {
	t.Parallel()
	text, err := mcp.GetPrompt("pmmcp_start_safe", map[string]string{
		"name": "api", "argv_json": `["./bin/api"]`, "project": "/proj",
	})
	if err != nil {
		t.Fatalf("GetPrompt: %v", err)
	}
	if !strings.Contains(text, "/proj") {
		t.Fatalf("GetPrompt with project override = %q, want substring %q", text, "/proj")
	}

	text, err = mcp.GetPrompt("pmmcp_apply_stack", map[string]string{"profile": "prod"})
	if err != nil {
		t.Fatalf("GetPrompt: %v", err)
	}
	if !strings.Contains(text, "prod") {
		t.Fatalf("GetPrompt with profile override = %q, want substring %q", text, "prod")
	}

	text, err = mcp.GetPrompt("pmmcp_oneshot_task", map[string]string{
		"argv_json": `["./bin/task"]`, "timeout": "30",
	})
	if err != nil {
		t.Fatalf("GetPrompt: %v", err)
	}
	if !strings.Contains(text, "30") {
		t.Fatalf("GetPrompt with timeout override = %q, want substring %q", text, "30")
	}
}

func TestGetPromptUnknown(t *testing.T) {
	t.Parallel()
	_, err := mcp.GetPrompt("nope", nil)
	var de *domain.Error
	if !errors.As(err, &de) || de.Code != domain.CodeNotFound {
		t.Fatalf("want not_found, got %v", err)
	}
}
