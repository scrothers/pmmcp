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

package prompts

import (
	"embed"
	"fmt"
	"strings"

	"github.com/scrothers/pmmcp/internal/domain"
)

//go:embed md/*.md
var mdFS embed.FS

// DocErrorCodes is the embedded filename for the error-code reference.
const DocErrorCodes = "docs_error_codes.md"

// DocToolIndex is the embedded filename for the short tool index.
const DocToolIndex = "docs_tool_index.md"

// Arg describes one prompt template argument.
type Arg struct {
	Name        string
	Description string
	Required    bool
	// Default is substituted when the argument is missing or empty.
	Default string
}

// Spec is a built-in MCP prompt template descriptor and its embedded file.
type Spec struct {
	Name        string
	Description string
	Arguments   []Arg
	// File is the basename under md/ (for example pmmcp_start_safe.md).
	File string
}

// catalog is structural metadata only. Descriptions live in lines.toml;
// bodies live in md/*.md (embedded prompt sources).
var catalog = []Spec{
	{
		Name: "pmmcp_start_safe",
		File: "pmmcp_start_safe.md",
		Arguments: []Arg{
			{Name: "name", Required: true},
			{Name: "argv_json", Required: true, Default: `["<program>", "<arg>", ...]`},
			{Name: "project", Default: "(current)"},
		},
	},
	{
		Name: "pmmcp_debug_crash",
		File: "pmmcp_debug_crash.md",
		Arguments: []Arg{
			{Name: "name", Required: true},
		},
	},
	{
		Name: "pmmcp_apply_stack",
		File: "pmmcp_apply_stack.md",
		Arguments: []Arg{
			{Name: "profile", Default: "(default)"},
		},
	},
	{
		Name: "pmmcp_import_compose",
		File: "pmmcp_import_compose.md",
		Arguments: []Arg{
			{Name: "path", Required: true},
		},
	},
	{
		Name: "pmmcp_oneshot_task",
		File: "pmmcp_oneshot_task.md",
		Arguments: []Arg{
			{Name: "argv_json", Required: true, Default: `["<program>", "<arg>", ...]`},
			{Name: "timeout", Default: "60"},
		},
	},
}

// List returns the built-in MCP prompt catalog in stable order, with
// descriptions filled from lines.toml.
func List() []Spec {
	out := make([]Spec, len(catalog))
	for i, s := range catalog {
		out[i] = hydrate(s)
	}
	return out
}

// Lookup returns the Spec for name (hydrated), or false if unknown.
func Lookup(name string) (Spec, bool) {
	for _, s := range catalog {
		if s.Name == name {
			return hydrate(s), true
		}
	}
	return Spec{}, false
}

func hydrate(s Spec) Spec {
	s.Description = PromptDescription(s.Name)
	args := make([]Arg, len(s.Arguments))
	copy(args, s.Arguments)
	for i := range args {
		args[i].Description = PromptArgDescription(s.Name, args[i].Name)
	}
	s.Arguments = args
	return s
}

// Render returns the prompt body for name with {{arg}} placeholders filled.
// Missing or empty args use Spec Arg.Default when set.
func Render(name string, args map[string]string) (string, error) {
	spec, ok := Lookup(name)
	if !ok {
		return "", domain.NewError(domain.CodeNotFound, "unknown prompt "+name, false)
	}
	raw, err := readMD(spec.File)
	if err != nil {
		return "", err
	}
	vals := resolveArgs(spec, args)
	return substitute(raw, vals), nil
}

// Doc returns a static embedded markdown document by basename under md/
// (for example DocErrorCodes). No placeholder substitution is applied.
func Doc(filename string) (string, error) {
	return readMD(filename)
}

// MustDoc returns a static doc or panics if the embed is missing.
// Intended for compile-time-known files wired at package init sites.
func MustDoc(filename string) string {
	s, err := Doc(filename)
	if err != nil {
		panic(err)
	}
	return s
}

func readMD(filename string) (string, error) {
	if filename == "" || strings.Contains(filename, "..") || strings.Contains(filename, "/") || strings.Contains(filename, "\\") {
		return "", domain.NewError(domain.CodeInvalidArgument, "invalid prompt file "+filename, false)
	}
	b, err := mdFS.ReadFile("md/" + filename)
	if err != nil {
		return "", fmt.Errorf("prompts: read %s: %w", filename, err)
	}
	return string(b), nil
}

func resolveArgs(spec Spec, args map[string]string) map[string]string {
	out := make(map[string]string, len(spec.Arguments))
	for _, a := range spec.Arguments {
		v := ""
		if args != nil {
			v = args[a.Name]
		}
		if v == "" && a.Default != "" {
			v = a.Default
		}
		out[a.Name] = v
	}
	// Pass through any extra keys so templates can use them if added later.
	for k, v := range args {
		if _, ok := out[k]; !ok {
			out[k] = v
		}
	}
	return out
}

// substitute replaces {{key}} with vals[key]. Unknown keys become empty.
func substitute(tmpl string, vals map[string]string) string {
	var b strings.Builder
	b.Grow(len(tmpl))
	for i := 0; i < len(tmpl); {
		if i+3 < len(tmpl) && tmpl[i] == '{' && tmpl[i+1] == '{' {
			end := strings.Index(tmpl[i+2:], "}}")
			if end >= 0 {
				key := strings.TrimSpace(tmpl[i+2 : i+2+end])
				b.WriteString(vals[key])
				i += 2 + end + 2
				continue
			}
		}
		b.WriteByte(tmpl[i])
		i++
	}
	return b.String()
}
