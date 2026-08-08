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

// Package mcp implements MCP resource and prompt helpers for pmmcp.
// The stdio MCP server entrypoint lives in package cli (pmmcp mcp). Tools dispatch through
// cli.ToolMethod to private IPC; this package only lists/reads resources and renders prompts.
// Multi-line bodies and tool description strings are embedded in package prompts. Dynamic resources
// fetch live data from the daemon over IPC.
package mcp
