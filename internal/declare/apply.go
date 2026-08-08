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

package declare

import (
	"fmt"
	"sort"
	"strings"
)

// DiffEntry describes one declarative drift item.
type DiffEntry struct {
	Name   string
	Action string // create|update|delete|noop
	Detail string
}

// DiffServices compares desired service names to running names. Output is sorted
// by name so pm_diff and golden tests are stable.
func DiffServices(doc *Document, runningNames []string) []DiffEntry {
	if doc == nil {
		return nil
	}
	run := map[string]bool{}
	for _, n := range runningNames {
		run[n] = true
	}
	var out []DiffEntry
	for name := range doc.Services {
		if run[name] {
			out = append(out, DiffEntry{Name: name, Action: "noop", Detail: "present"})
			delete(run, name)
		} else {
			out = append(out, DiffEntry{Name: name, Action: "create", Detail: "missing runtime"})
		}
	}
	for name := range run {
		out = append(out, DiffEntry{Name: name, Action: "delete", Detail: "not in declare"})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// shellMetaChars are the characters whose presence in a Procfile command forces
// a shell wrapper (otherwise the command is split into a plain argv).
const shellMetaChars = "|&;<>()$`\\\"'*?[]{}~#!"

// ImportProcfile parses a simple Procfile into a Document draft. A command with
// no shell features is split into a plain argv; a command that needs
// the shell is wrapped as ["/bin/sh", "-c", cmd] — a non-login shell, not -lc.
// Duplicate process names are an error rather than a silent overwrite.
func ImportProcfile(data []byte) (*Document, error) {
	doc := &Document{
		APIVersion: CanonicalAPIVersion,
		Kind:       "Project",
		Services:   map[string]ServiceSpec{},
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, cmd, ok := strings.Cut(line, ":")
		if !ok {
			return nil, fmt.Errorf("declare: bad procfile line %q", line)
		}
		name = strings.TrimSpace(name)
		cmd = strings.TrimSpace(cmd)
		if name == "" || cmd == "" {
			return nil, fmt.Errorf("declare: bad procfile line %q", line)
		}
		if _, exists := doc.Services[name]; exists {
			return nil, fmt.Errorf("declare: duplicate procfile process %q", name)
		}
		doc.Services[name] = ServiceSpec{Name: name, Argv: procfileArgv(cmd)}
	}
	// A draft may legitimately contain a shell-wrapped fallback argv; the
	// shell-risk gate applies when the draft is later applied, not at import.
	return doc, doc.Validate(WithAllowShellArgv())
}

// procfileArgv splits a plain command into argv, falling back to a non-login
// shell wrapper only when shell metacharacters are present.
func procfileArgv(cmd string) []string {
	if strings.ContainsAny(cmd, shellMetaChars) {
		return []string{"/bin/sh", "-c", cmd}
	}
	return strings.Fields(cmd)
}
