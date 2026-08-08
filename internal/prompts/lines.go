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
	_ "embed"
	"fmt"
	"sync"

	"github.com/BurntSushi/toml"
)

//go:embed lines.toml
var linesTOML string

// linesFile is the on-disk shape of lines.toml.
type linesFile struct {
	Tools             map[string]string `toml:"tools"`
	Resources         map[string]string `toml:"resources"`
	ResourceTemplates map[string]string `toml:"resource_templates"`
	ResourceDyn       map[string]string `toml:"resource_dyn"`
	PromptDesc        map[string]string `toml:"prompt_desc"`
	PromptArg         map[string]string `toml:"prompt_arg"`
}

var (
	linesOnce sync.Once
	linesData linesFile
	linesErr  error
)

func loadLines() {
	linesOnce.Do(func() {
		linesErr = toml.Unmarshal([]byte(linesTOML), &linesData)
		if linesErr != nil {
			linesErr = fmt.Errorf("prompts: parse lines.toml: %w", linesErr)
			return
		}
		if linesData.Tools == nil {
			linesData.Tools = map[string]string{}
		}
		if linesData.Resources == nil {
			linesData.Resources = map[string]string{}
		}
		if linesData.ResourceTemplates == nil {
			linesData.ResourceTemplates = map[string]string{}
		}
		if linesData.ResourceDyn == nil {
			linesData.ResourceDyn = map[string]string{}
		}
		if linesData.PromptDesc == nil {
			linesData.PromptDesc = map[string]string{}
		}
		if linesData.PromptArg == nil {
			linesData.PromptArg = map[string]string{}
		}
	})
}

// mustLines returns the parsed single-line catalog or panics on embed/parse failure.
func mustLines() *linesFile {
	loadLines()
	if linesErr != nil {
		panic(linesErr)
	}
	return &linesData
}

// ToolDescription returns the tools/list one-liner for a pm_* tool, or "".
func ToolDescription(name string) string {
	return mustLines().Tools[name]
}

// ToolDescriptions returns a copy of all tool one-liners (for CLI catalog maps).
func ToolDescriptions() map[string]string {
	src := mustLines().Tools
	out := make(map[string]string, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

// ResourceDescription returns the static resource description for a resource Name.
func ResourceDescription(name string) string {
	return mustLines().Resources[name]
}

// ResourceTemplateDescription returns the description for a resource template Name.
func ResourceTemplateDescription(name string) string {
	return mustLines().ResourceTemplates[name]
}

// ResourceDyn keys for per-instance list descriptions.
const (
	DynProcessStatus = "process_status"
	DynProcessLog    = "process_log"
	DynGroupStatus   = "group_status"
)

// ResourceDynDescription renders a dynamic resource description with {{name}} etc.
func ResourceDynDescription(key, name string) string {
	tmpl := mustLines().ResourceDyn[key]
	if tmpl == "" {
		return name
	}
	return substitute(tmpl, map[string]string{"name": name})
}

// PromptDescription returns the prompts/list description for a prompt name.
func PromptDescription(name string) string {
	return mustLines().PromptDesc[name]
}

// PromptArgDescription returns the argument description for prompt.arg.
func PromptArgDescription(prompt, arg string) string {
	return mustLines().PromptArg[prompt+"."+arg]
}
