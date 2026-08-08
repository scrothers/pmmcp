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
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/scrothers/pmmcp/internal/api"
)

// connectedServer wires registerTools/registerResources/registerPrompts
// against endpoint and returns a live client session connected to it over an
// in-memory transport, so the registered handler closures (not just the
// registration wiring) actually run.
func connectedServer(t *testing.T, endpoint string) *mcp.ClientSession {
	t.Helper()
	server := mcp.NewServer(&mcp.Implementation{Name: "pmmcp", Version: "test"}, nil)
	registerTools(server, endpoint)
	registerResources(server, endpoint)
	registerPrompts(server, endpoint)

	ctx := context.Background()
	ct, st := mcp.NewInMemoryTransports()
	ss, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server.Connect: %v", err)
	}
	t.Cleanup(func() { _ = ss.Close() })

	client := mcp.NewClient(&mcp.Implementation{Name: "pmmcp-test-client", Version: "test"}, nil)
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client.Connect: %v", err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

func textContent(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	if len(res.Content) == 0 {
		t.Fatal("CallToolResult has no content")
	}
	tc, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content[0] = %T, want *mcp.TextContent", res.Content[0])
	}
	return tc.Text
}

// TestRegisteredToolCallSuccess drives a registered tool handler end-to-end
// through a real MCP client/server pair, covering the JSON-decode-success and
// mcpCall-success branches.
func TestRegisteredToolCallSuccess(t *testing.T) {
	endpoint := startScriptedDaemon(t, nil)
	cs := connectedServer(t, endpoint)

	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "pm_whoami",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("CallTool returned IsError=true: %s", textContent(t, res))
	}
}

// TestRegisteredToolCallInvalidArguments covers registerTools' JSON-decode
// failure branch: sending a non-object arguments value fails to unmarshal
// into map[string]any.
func TestRegisteredToolCallInvalidArguments(t *testing.T) {
	endpoint := startScriptedDaemon(t, nil)
	cs := connectedServer(t, endpoint)

	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "pm_list",
		Arguments: []string{"not", "an", "object"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !res.IsError {
		t.Fatal("want IsError=true for non-object arguments")
	}
	if got := textContent(t, res); got == "" || !strings.Contains(got, "invalid arguments") {
		t.Errorf("content = %q, want mention of invalid arguments", got)
	}
}

// TestRegisteredToolCallDaemonError covers registerTools' mcpCall-error
// branch: the daemon reports a domain error for the mapped method.
func TestRegisteredToolCallDaemonError(t *testing.T) {
	endpoint := startScriptedDaemon(t, map[string]scriptedResponse{
		api.MethodWhoami: {errMsg: "boom", errCode: "internal"},
	})
	cs := connectedServer(t, endpoint)

	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "pm_whoami",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !res.IsError {
		t.Fatal("want IsError=true for daemon error")
	}
}

// TestRegisterToolsDescriptionFallback covers registerTools' `desc == ""`
// fallback (desc = name): every catalog tool has a non-empty description
// (TestToolDescriptionCoversAll), so this branch is only reachable by
// temporarily adding a fake catalog entry with an empty description. This
// mutates package-level catalog globals, so it cannot run in parallel with
// other tests that iterate ToolNames()/ToolMethod.
func TestRegisterToolsDescriptionFallback(t *testing.T) {
	const fake = "pm_test_fixture_only"
	ToolMethod[fake] = api.MethodWhoami
	t.Cleanup(func() { delete(ToolMethod, fake) })
	// ToolDescription[fake] is deliberately left unset (empty string).

	server := mcp.NewServer(&mcp.Implementation{Name: "pmmcp", Version: "test"}, nil)
	registerTools(server, "unused") // must not panic with the empty description
}

// TestRegisteredResourceReadStaticSuccess covers registerResources' success
// branch for a static, non-daemon-backed resource.
func TestRegisteredResourceReadStaticSuccess(t *testing.T) {
	cs := connectedServer(t, "unused")
	res, err := cs.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "pmmcp://docs/error-codes"})
	if err != nil {
		t.Fatalf("ReadResource: %v", err)
	}
	if len(res.Contents) == 0 || res.Contents[0].Text == "" {
		t.Fatal("want non-empty resource text")
	}
}

// TestRegisteredResourceReadTemplateSuccess covers registerResources' success
// branch for a parameterized (daemon-backed) resource template.
func TestRegisteredResourceReadTemplateSuccess(t *testing.T) {
	endpoint := startScriptedDaemon(t, nil)
	cs := connectedServer(t, endpoint)
	res, err := cs.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "pmmcp://process/proc-1"})
	if err != nil {
		t.Fatalf("ReadResource: %v", err)
	}
	if len(res.Contents) == 0 {
		t.Fatal("want resource contents")
	}
}

// TestRegisteredResourceReadError covers registerResources' error branch: a
// resource whose backing lookup genuinely fails (no pmmcp.yaml in the test's
// working directory).
func TestRegisteredResourceReadError(t *testing.T) {
	cs := connectedServer(t, "unused")
	if _, err := cs.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "pmmcp://declare"}); err == nil {
		t.Fatal("want error reading pmmcp://declare with no pmmcp.yaml present")
	}
}

// TestRegisteredPromptGetSuccess covers registerPrompts' success branch,
// including the with-arguments and without-arguments request shapes.
func TestRegisteredPromptGetSuccess(t *testing.T) {
	cs := connectedServer(t, "unused")

	res, err := cs.GetPrompt(context.Background(), &mcp.GetPromptParams{
		Name:      "pmmcp_debug_crash",
		Arguments: map[string]string{"name": "web"},
	})
	if err != nil {
		t.Fatalf("GetPrompt: %v", err)
	}
	if len(res.Messages) == 0 {
		t.Fatal("want at least one prompt message")
	}

	if _, err := cs.GetPrompt(context.Background(), &mcp.GetPromptParams{Name: "pmmcp_apply_stack"}); err != nil {
		t.Fatalf("GetPrompt with no arguments: %v", err)
	}
}
