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

// Package mcp provides MCP resources and prompts for the pmmcp server.
//
// It is a thin adapter: every dynamic resource is fetched from the
// daemon over IPC and only marshaling/formatting happens here. No business
// logic, no process spawning.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/scrothers/pmmcp/internal/api"
	"github.com/scrothers/pmmcp/internal/domain"
	"github.com/scrothers/pmmcp/internal/ipc"
	"github.com/scrothers/pmmcp/internal/prompts"
)

// Resource is an MCP resource descriptor.
type Resource struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType,omitempty"`
}

// ResourceTemplate is a parameterized MCP resource URI.
type ResourceTemplate struct {
	URITemplate string `json:"uriTemplate"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType,omitempty"`
}

// StaticResources returns the fixed singleton resources (no daemon required to
// enumerate). These plus the templates from ResourceTemplates cover the surface
// Resource catalog for MCP. Descriptions come from prompts/lines.toml.
func StaticResources() []Resource {
	return []Resource{
		{URI: "pmmcp://processes", Name: "processes", Description: prompts.ResourceDescription("processes"), MimeType: "application/json"},
		{URI: "pmmcp://daemon", Name: "daemon", Description: prompts.ResourceDescription("daemon"), MimeType: "application/json"},
		{URI: "pmmcp://project/current", Name: "project-current", Description: prompts.ResourceDescription("project-current"), MimeType: "application/json"},
		{URI: "pmmcp://declare", Name: "declare", Description: prompts.ResourceDescription("declare"), MimeType: "application/yaml"},
		{URI: "pmmcp://ports", Name: "ports", Description: prompts.ResourceDescription("ports"), MimeType: "application/json"},
		{URI: "pmmcp://events/recent", Name: "events-recent", Description: prompts.ResourceDescription("events-recent"), MimeType: "application/json"},
		{URI: "pmmcp://docs/error-codes", Name: "docs-error-codes", Description: prompts.ResourceDescription("docs-error-codes"), MimeType: "text/markdown"},
		{URI: "pmmcp://docs/tool-index", Name: "docs-tool-index", Description: prompts.ResourceDescription("docs-tool-index"), MimeType: "text/markdown"},
	}
}

// ResourceTemplates returns the parameterized resource URIs.
func ResourceTemplates() []ResourceTemplate {
	return []ResourceTemplate{
		{URITemplate: "pmmcp://project/{id}", Name: "project", Description: prompts.ResourceTemplateDescription("project"), MimeType: "application/json"},
		{URITemplate: "pmmcp://process/{name_or_id}", Name: "process", Description: prompts.ResourceTemplateDescription("process"), MimeType: "application/json"},
		{URITemplate: "pmmcp://process/{name_or_id}/log", Name: "process-log", Description: prompts.ResourceTemplateDescription("process-log"), MimeType: "text/plain"},
		{URITemplate: "pmmcp://group/{name}", Name: "group", Description: prompts.ResourceTemplateDescription("group"), MimeType: "application/json"},
	}
}

// ListResources returns static singletons plus dynamic per-process/group/project
// resources from the daemon. When the daemon is unreachable it returns the
// static set together with the dial error, so callers can distinguish "daemon
// down" from "no processes" rather than having the signal silently dropped.
func ListResources(ctx context.Context, endpoint string) ([]Resource, error) {
	base := StaticResources()
	c, err := ipc.Dial(ctx, endpoint)
	if err != nil {
		return base, err
	}
	defer func() { _ = c.Close() }()

	var list []api.ProcessView
	if err := c.Call(ctx, api.MethodList, api.ListPayload{}, &list); err != nil {
		return base, err
	}
	for _, p := range list {
		base = append(base,
			Resource{URI: "pmmcp://process/" + p.ID, Name: p.Name, Description: prompts.ResourceDynDescription(prompts.DynProcessStatus, p.Name), MimeType: "application/json"},
			Resource{URI: "pmmcp://process/" + p.ID + "/log", Name: p.Name + "-log", Description: prompts.ResourceDynDescription(prompts.DynProcessLog, p.Name), MimeType: "text/plain"},
		)
	}
	var groups []api.GroupView
	if err := c.Call(ctx, api.MethodGroupList, nil, &groups); err == nil {
		for _, g := range groups {
			base = append(base, Resource{
				URI: "pmmcp://group/" + g.Name, Name: "group-" + g.Name,
				Description: prompts.ResourceDynDescription(prompts.DynGroupStatus, g.Name), MimeType: "application/json",
			})
		}
	}
	return base, nil
}

// ReadResource fetches resource contents for a URI.
func ReadResource(ctx context.Context, endpoint, uri string) (string, error) {
	// Non-daemon resources first (no dial needed).
	switch uri {
	case "pmmcp://declare":
		return readDeclare()
	case "pmmcp://docs/error-codes":
		return prompts.Doc(prompts.DocErrorCodes)
	case "pmmcp://docs/tool-index":
		return prompts.Doc(prompts.DocToolIndex)
	}

	c, err := ipc.Dial(ctx, endpoint)
	if err != nil {
		return "", err
	}
	defer func() { _ = c.Close() }()
	return readDaemonResource(ctx, c, uri)
}

func readDaemonResource(ctx context.Context, c *ipc.Client, uri string) (string, error) {
	switch {
	case uri == "pmmcp://processes":
		return callJSON(ctx, c, api.MethodList, api.ListPayload{})
	case uri == "pmmcp://daemon":
		return callJSON(ctx, c, api.MethodDaemonInfo, nil)
	case uri == "pmmcp://project/current":
		return callJSON(ctx, c, api.MethodProjectCurrent, nil)
	case uri == "pmmcp://ports":
		return portsTable(ctx, c)
	case uri == "pmmcp://events/recent":
		return callJSON(ctx, c, api.MethodEvents, api.EventsPayload{Limit: 50})
	case strings.HasPrefix(uri, "pmmcp://project/"):
		return projectByID(ctx, c, strings.TrimPrefix(uri, "pmmcp://project/"))
	case strings.HasPrefix(uri, "pmmcp://group/"):
		return callJSON(ctx, c, api.MethodGroupStatus, api.GroupPayload{Name: strings.TrimPrefix(uri, "pmmcp://group/")})
	case strings.HasPrefix(uri, "pmmcp://process/") && strings.HasSuffix(uri, "/log"):
		id := strings.TrimSuffix(strings.TrimPrefix(uri, "pmmcp://process/"), "/log")
		var out api.LogsResult
		if err := c.Call(ctx, api.MethodLogs, logsPayload(id, 200), &out); err != nil {
			return "", err
		}
		return out.Text, nil
	case strings.HasPrefix(uri, "pmmcp://process/"):
		id := strings.TrimPrefix(uri, "pmmcp://process/")
		return callJSON(ctx, c, api.MethodStatus, idOrName(id))
	default:
		return "", domain.NewError(domain.CodeInvalidArgument, fmt.Sprintf("unknown resource %s", uri), false)
	}
}

// callJSON calls method and returns the indented JSON result.
func callJSON(ctx context.Context, c *ipc.Client, method string, payload any) (string, error) {
	var out any
	if err := c.Call(ctx, method, payload, &out); err != nil {
		return "", err
	}
	b, err := json.MarshalIndent(out, "", " ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// projectByID lists projects and returns the one whose key matches id.
func projectByID(ctx context.Context, c *ipc.Client, id string) (string, error) {
	var out api.ProjectListResult
	if err := c.Call(ctx, api.MethodProjectList, nil, &out); err != nil {
		return "", err
	}
	for _, p := range out.Projects {
		if p.Key == id {
			b, _ := json.MarshalIndent(p, "", " ")
			return string(b), nil
		}
	}
	return "", domain.NewError(domain.CodeNotFound, "project "+id+" not found", false)
}

// portsTable builds a compact ports table from the process list.
func portsTable(ctx context.Context, c *ipc.Client) (string, error) {
	var list []api.ProcessView
	if err := c.Call(ctx, api.MethodList, api.ListPayload{}, &list); err != nil {
		return "", err
	}
	type row struct {
		ID         string   `json:"id"`
		Name       string   `json:"name"`
		Ports      []string `json:"ports,omitempty"`
		Discovered []string `json:"discovered,omitempty"`
	}
	rows := make([]row, 0, len(list))
	for _, p := range list {
		if len(p.Ports) == 0 && len(p.Discovered) == 0 {
			continue
		}
		rows = append(rows, row{ID: p.ID, Name: p.Name, Ports: p.Ports, Discovered: p.Discovered})
	}
	b, err := json.MarshalIndent(rows, "", " ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// readDeclare returns the raw pmmcp.yaml (or.yml) from the current directory.
func readDeclare() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", domain.WrapError(domain.CodeInternal, "getwd", false, err)
	}
	for _, name := range []string{"pmmcp.yaml", "pmmcp.yml"} {
		b, err := os.ReadFile(filepath.Join(cwd, name))
		if err == nil {
			return string(b), nil
		}
	}
	return "", domain.NewError(domain.CodeNotFound, "no pmmcp.yaml in current project", false)
}

func idOrName(seg string) api.IDPayload {
	if strings.HasPrefix(seg, "proc-") {
		return api.IDPayload{ID: seg}
	}
	return api.IDPayload{Name: seg}
}

func logsPayload(seg string, lines int) api.LogsPayload {
	if strings.HasPrefix(seg, "proc-") {
		return api.LogsPayload{ID: seg, Lines: lines}
	}
	return api.LogsPayload{Name: seg, Lines: lines}
}
