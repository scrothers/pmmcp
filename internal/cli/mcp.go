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
	"fmt"

	"github.com/scrothers/pmmcp/internal/api"
	"github.com/scrothers/pmmcp/internal/ipc"
)

// This file holds the tool-call bridge and JSON-schema helpers used by the
// official MCP SDK server (mcp_sdk.go). The tools map to daemon IPC methods;
// there is no business logic here.

// genericInputSchema is used when a tool has no specialized schema.
var genericInputSchema = map[string]any{
	"type":       "object",
	"properties": map[string]any{},
}

// specializedSchemas holds richer inputSchema for well-known tools.
func specializedSchemas() map[string]map[string]any {
	return map[string]map[string]any{
		"pm_start": {
			"type": "object",
			"properties": map[string]any{
				"name":    map[string]any{"type": "string"},
				"command": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				"cwd":     map[string]any{"type": "string"},
				"sandbox": map[string]any{"type": "string"},
			},
			"required": []string{"name", "command"},
		},
		"pm_stop":         idNameSchema(),
		"pm_restart":      idNameSchema(),
		"pm_remove":       idNameSchema(),
		"pm_status":       idNameSchema(),
		"pm_wait":         idNameSchema(),
		"pm_enable":       idNameSchema(),
		"pm_disable":      idNameSchema(),
		"pm_health_check": idNameSchema(),
		"pm_list": {
			"type": "object",
			"properties": map[string]any{
				"project": map[string]any{"type": "string"},
				"status":  map[string]any{"type": "string"},
			},
		},
		"pm_logs": {
			"type": "object",
			"properties": map[string]any{
				"id": map[string]any{"type": "string"}, "name": map[string]any{"type": "string"},
				"lines": map[string]any{"type": "integer"}, "stream": map[string]any{"type": "string"},
			},
		},
		"pm_grep": {
			"type": "object",
			"properties": map[string]any{
				"id": map[string]any{"type": "string"}, "name": map[string]any{"type": "string"},
				"pattern": map[string]any{"type": "string"},
			},
			"required": []string{"pattern"},
		},
		"pm_errors": idNameSchema(),
		"pm_events": {
			"type": "object",
			"properties": map[string]any{
				"process_id": map[string]any{"type": "string"},
				"limit":      map[string]any{"type": "integer"},
			},
		},
	}
}

func idNameSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"id": map[string]any{"type": "string"}, "name": map[string]any{"type": "string"},
		},
	}
}

// schemaForTool returns the input schema for a tool, defaulting to the generic
// object schema when no specialized schema exists.
func schemaForTool(name string) map[string]any {
	if s, ok := specializedSchemas()[name]; ok {
		return s
	}
	return genericInputSchema
}

func mcpCall(ctx context.Context, endpoint, name string, args map[string]any) (string, error) {
	method, ok := ToolMethod[name]
	if !ok {
		return "", fmt.Errorf("unknown tool %s", name)
	}

	c, err := ipc.Dial(ctx, endpoint)
	if err != nil {
		// Dial already returns domain.Error with daemon_unavailable.
		return "", err
	}
	defer func() { _ = c.Close() }()

	// Optimized paths for tools with typed payloads / text-only results.
	switch name {
	case "pm_start":
		var pl api.StartPayload
		pl.Name, _ = args["name"].(string)
		if arr, ok := args["command"].([]any); ok {
			for _, a := range arr {
				pl.Command = append(pl.Command, fmt.Sprint(a))
			}
		}
		pl.Cwd, _ = args["cwd"].(string)
		pl.Sandbox, _ = args["sandbox"].(string)
		var out api.StartResult
		if err := c.Call(ctx, method, pl, &out); err != nil {
			return "", err
		}
		return prettyJSON(out)
	case "pm_stop", "pm_restart", "pm_remove", "pm_status", "pm_wait", "pm_enable", "pm_disable", "pm_health_check":
		pl := api.IDPayload{}
		pl.ID, _ = args["id"].(string)
		pl.Name, _ = args["name"].(string)
		if n, ok := args["timeout_sec"].(float64); ok {
			pl.TimeoutSec = int(n)
		}
		var out any
		if err := c.Call(ctx, method, pl, &out); err != nil {
			return "", err
		}
		return prettyJSON(out)
	case "pm_list":
		pl := api.ListPayload{}
		pl.Project, _ = args["project"].(string)
		pl.Status, _ = args["status"].(string)
		var out []api.ProcessView
		if err := c.Call(ctx, method, pl, &out); err != nil {
			return "", err
		}
		return prettyJSON(out)
	case "pm_logs", "pm_grep", "pm_errors":
		pl := api.LogsPayload{}
		pl.ID, _ = args["id"].(string)
		pl.Name, _ = args["name"].(string)
		pl.Pattern, _ = args["pattern"].(string)
		pl.Stream, _ = args["stream"].(string)
		if n, ok := args["lines"].(float64); ok {
			pl.Lines = int(n)
		}
		var out api.LogsResult
		if err := c.Call(ctx, method, pl, &out); err != nil {
			return "", err
		}
		return out.Text, nil
	case "pm_daemon_info":
		var out api.DaemonInfoResult
		if err := c.Call(ctx, method, nil, &out); err != nil {
			return "", err
		}
		return prettyJSON(out)
	case "pm_whoami":
		var out api.WhoamiResult
		if err := c.Call(ctx, method, nil, &out); err != nil {
			return "", err
		}
		return prettyJSON(out)
	case "pm_events":
		pl := api.EventsPayload{}
		pl.ProcessID, _ = args["process_id"].(string)
		if n, ok := args["limit"].(float64); ok {
			pl.Limit = int(n)
		}
		var out []api.EventView
		if err := c.Call(ctx, method, pl, &out); err != nil {
			return "", err
		}
		return prettyJSON(out)
	}

	// Generic path: marshal arguments as JSON payload and Call the mapped method.
	var out any
	if err := c.Call(ctx, method, args, &out); err != nil {
		return "", err
	}
	if out == nil {
		return "{}", nil
	}
	return prettyJSON(out)
}

func prettyJSON(v any) (string, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}
