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

package mcp

import (
	"github.com/scrothers/pmmcp/internal/prompts"
)

// PromptArg describes one prompt argument.
type PromptArg struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
}

// Prompt is an MCP prompt template descriptor.
type Prompt struct {
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	Arguments   []PromptArg `json:"arguments,omitempty"`
}

// ListPrompts returns the built-in prompt templates
// Prompt catalog for MCP. Bodies live in internal/prompts.
func ListPrompts() []Prompt {
	specs := prompts.List()
	out := make([]Prompt, 0, len(specs))
	for _, s := range specs {
		args := make([]PromptArg, 0, len(s.Arguments))
		for _, a := range s.Arguments {
			args = append(args, PromptArg{
				Name:        a.Name,
				Description: a.Description,
				Required:    a.Required,
			})
		}
		out = append(out, Prompt{
			Name:        s.Name,
			Description: s.Description,
			Arguments:   args,
		})
	}
	return out
}

// GetPrompt renders prompt message text for name using the provided arguments.
func GetPrompt(name string, args map[string]string) (string, error) {
	return prompts.Render(name, args)
}
