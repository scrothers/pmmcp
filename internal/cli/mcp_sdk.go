// Copyright 2026 Steven Crothers
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
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

	"github.com/modelcontextprotocol/go-sdk/mcp"
	mcpres "github.com/scrothers/pmmcp/internal/mcp"
)

// runMCPSDK serves MCP over stdio using the official modelcontextprotocol/go-sdk.
func (st *rootState) runMCPSDK(ctx context.Context) error {
	cfg, err := st.loadCfg()
	if err != nil {
		return err
	}
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "pmmcp",
		Version: "0.0.0-dev",
	}, nil)

	endpoint := cfg.IPC.Endpoint
	registerTools(server, endpoint)
	registerResources(server, endpoint)
	registerPrompts(server, endpoint)

	return server.Run(ctx, &mcp.StdioTransport{})
}

// registerTools registers every catalog tool with its specialized input schema
// . Server.AddTool takes a raw
// schema and leaves argument decoding to the handler.
func registerTools(server *mcp.Server, endpoint string) {
	for _, name := range ToolNames() {
		name := name
		desc := ToolDescription[name]
		if desc == "" {
			desc = name
		}
		server.AddTool(&mcp.Tool{
			Name:        name,
			Description: desc,
			InputSchema: schemaForTool(name),
		}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			args := map[string]any{}
			if len(req.Params.Arguments) > 0 {
				if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
					//nolint:nilerr // MCP maps tool failure into CallToolResult.IsError
					return &mcp.CallToolResult{
						Content: []mcp.Content{&mcp.TextContent{Text: "invalid arguments: " + err.Error()}},
						IsError: true,
					}, nil
				}
			}
			text, err := mcpCall(ctx, endpoint, name, args)
			if err != nil {
				//nolint:nilerr // MCP maps tool failure into CallToolResult.IsError
				return &mcp.CallToolResult{
					Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}},
					IsError: true,
				}, nil
			}
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: text}},
			}, nil
		})
	}
}

// registerResources registers the fixed singleton resources and the
// parameterized templates (per-process, per-group, per-project) so dynamic URIs
// are reachable on the shipping SDK path, not only the static two.
func registerResources(server *mcp.Server, endpoint string) {
	handler := func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		uri := req.Params.URI
		text, err := mcpres.ReadResource(ctx, endpoint, uri)
		if err != nil {
			return nil, err
		}
		return &mcp.ReadResourceResult{
			Contents: []*mcp.ResourceContents{{URI: uri, Text: text}},
		}, nil
	}
	for _, r := range mcpres.StaticResources() {
		server.AddResource(&mcp.Resource{
			URI:         r.URI,
			Name:        r.Name,
			Description: r.Description,
			MIMEType:    r.MimeType,
		}, handler)
	}
	for _, t := range mcpres.ResourceTemplates() {
		server.AddResourceTemplate(&mcp.ResourceTemplate{
			URITemplate: t.URITemplate,
			Name:        t.Name,
			Description: t.Description,
			MIMEType:    t.MimeType,
		}, handler)
	}
}

// registerPrompts registers the built-in prompts with their argument schemas.
func registerPrompts(server *mcp.Server, _ string) {
	for _, p := range mcpres.ListPrompts() {
		p := p
		args := make([]*mcp.PromptArgument, 0, len(p.Arguments))
		for _, a := range p.Arguments {
			args = append(args, &mcp.PromptArgument{
				Name:        a.Name,
				Description: a.Description,
				Required:    a.Required,
			})
		}
		server.AddPrompt(&mcp.Prompt{
			Name:        p.Name,
			Description: p.Description,
			Arguments:   args,
		}, func(_ context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
			var pargs map[string]string
			if req != nil && req.Params != nil {
				pargs = req.Params.Arguments
			}
			text, err := mcpres.GetPrompt(p.Name, pargs)
			if err != nil {
				return nil, err
			}
			return &mcp.GetPromptResult{
				Messages: []*mcp.PromptMessage{
					{Role: "user", Content: &mcp.TextContent{Text: text}},
				},
			}, nil
		})
	}
}
