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

package prompts_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/scrothers/pmmcp/internal/domain"
	"github.com/scrothers/pmmcp/internal/prompts"
)

func TestList(t *testing.T) {
	t.Parallel()
	list := prompts.List()
	if len(list) != 5 {
		t.Fatalf("List len = %d, want 5", len(list))
	}
	want := map[string]bool{
		"pmmcp_start_safe": true, "pmmcp_debug_crash": true, "pmmcp_apply_stack": true,
		"pmmcp_import_compose": true, "pmmcp_oneshot_task": true,
	}
	for _, s := range list {
		if !want[s.Name] {
			t.Errorf("unexpected prompt %q", s.Name)
		}
		if s.Description == "" {
			t.Errorf("prompt %q missing description", s.Name)
		}
		if s.File == "" {
			t.Errorf("prompt %q missing File", s.Name)
		}
	}
}

func TestRenderAll(t *testing.T) {
	t.Parallel()
	for _, s := range prompts.List() {
		text, err := prompts.Render(s.Name, map[string]string{
			"name": "api", "argv_json": `["./bin/api"]`, "path": "./Procfile",
		})
		if err != nil {
			t.Fatalf("Render(%q): %v", s.Name, err)
		}
		if strings.TrimSpace(text) == "" {
			t.Fatalf("Render(%q) empty", s.Name)
		}
		if strings.Contains(text, "{{") {
			t.Fatalf("Render(%q) left placeholders: %q", s.Name, text)
		}
	}
}

func TestRenderDefaults(t *testing.T) {
	t.Parallel()
	text, err := prompts.Render("pmmcp_start_safe", map[string]string{"name": "web"})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(text, `["<program>", "<arg>", ...]`) {
		t.Fatalf("want argv default, got %q", text)
	}
	if !strings.Contains(text, "(current)") {
		t.Fatalf("want project default, got %q", text)
	}
	if !strings.Contains(text, `"web"`) {
		t.Fatalf("want name web, got %q", text)
	}

	text, err = prompts.Render("pmmcp_apply_stack", nil)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(text, "(default)") {
		t.Fatalf("want profile default, got %q", text)
	}

	text, err = prompts.Render("pmmcp_oneshot_task", map[string]string{"timeout": "30", "argv_json": `["./t"]`})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(text, "30") {
		t.Fatalf("want timeout 30, got %q", text)
	}
	if !strings.Contains(text, `["./t"]`) {
		t.Fatalf("want argv override, got %q", text)
	}
}

func TestRenderUnknown(t *testing.T) {
	t.Parallel()
	_, err := prompts.Render("nope", nil)
	var de *domain.Error
	if !errors.As(err, &de) || de.Code != domain.CodeNotFound {
		t.Fatalf("want not_found, got %v", err)
	}
}

func TestDoc(t *testing.T) {
	t.Parallel()
	codes, err := prompts.Doc(prompts.DocErrorCodes)
	if err != nil {
		t.Fatalf("Doc error codes: %v", err)
	}
	if !strings.Contains(codes, "daemon_unavailable") {
		t.Fatalf("error codes doc missing daemon_unavailable: %q", codes)
	}
	index, err := prompts.Doc(prompts.DocToolIndex)
	if err != nil {
		t.Fatalf("Doc tool index: %v", err)
	}
	if !strings.Contains(index, "pm_start") {
		t.Fatalf("tool index missing pm_start: %q", index)
	}
	// Must not run placeholder substitution on docs.
	if strings.Contains(index, "{{") {
		t.Fatalf("tool index has unresolved placeholders")
	}
}

func TestDocRejectsPathTraversal(t *testing.T) {
	t.Parallel()
	_, err := prompts.Doc("../prompts.go")
	if err == nil {
		t.Fatal("want error for path traversal")
	}
	_, err = prompts.Doc("md/pmmcp_start_safe.md")
	if err == nil {
		t.Fatal("want error for nested path")
	}
}

func TestMustDoc(t *testing.T) {
	t.Parallel()
	s := prompts.MustDoc(prompts.DocErrorCodes)
	if s == "" {
		t.Fatal("MustDoc empty")
	}
}

func TestLookup(t *testing.T) {
	t.Parallel()
	s, ok := prompts.Lookup("pmmcp_debug_crash")
	if !ok || s.Name != "pmmcp_debug_crash" {
		t.Fatalf("Lookup failed: %+v ok=%v", s, ok)
	}
	if s.Description == "" {
		t.Fatal("Lookup description empty — lines.toml prompt_desc missing?")
	}
	if len(s.Arguments) == 0 || s.Arguments[0].Description == "" {
		t.Fatal("Lookup arg description empty — lines.toml prompt_arg missing?")
	}
	if _, ok := prompts.Lookup("missing"); ok {
		t.Fatal("Lookup(missing) want false")
	}
}

func TestToolDescriptions(t *testing.T) {
	t.Parallel()
	all := prompts.ToolDescriptions()
	if len(all) < 60 {
		t.Fatalf("ToolDescriptions len = %d, want ≥60", len(all))
	}
	if got := prompts.ToolDescription("pm_start"); got == "" {
		t.Fatal("ToolDescription(pm_start) empty")
	}
	if got := prompts.ToolDescription("nope"); got != "" {
		t.Fatalf("ToolDescription(nope) = %q, want empty", got)
	}
	// Copy is independent of further mutation of the returned map.
	all["pm_start"] = "mutated"
	if prompts.ToolDescription("pm_start") == "mutated" {
		t.Fatal("ToolDescriptions should return a copy")
	}
}

func TestResourceLines(t *testing.T) {
	t.Parallel()
	if prompts.ResourceDescription("processes") == "" {
		t.Fatal("ResourceDescription(processes) empty")
	}
	if prompts.ResourceTemplateDescription("process") == "" {
		t.Fatal("ResourceTemplateDescription(process) empty")
	}
	got := prompts.ResourceDynDescription(prompts.DynProcessStatus, "web")
	if got != "Status for process web" {
		t.Fatalf("ResourceDynDescription = %q, want %q", got, "Status for process web")
	}
	got = prompts.ResourceDynDescription(prompts.DynProcessLog, "api")
	if got != "Recent logs for api" {
		t.Fatalf("ResourceDynDescription log = %q", got)
	}
	got = prompts.ResourceDynDescription(prompts.DynGroupStatus, "stack")
	if got != "Status for group stack" {
		t.Fatalf("ResourceDynDescription group = %q", got)
	}
}

func TestPromptBodiesTeachRealAPI(t *testing.T) {
	t.Parallel()
	// Regression: apply must not invent a start flag; start/run use field `command`.
	apply, err := prompts.Render("pmmcp_apply_stack", map[string]string{"profile": "prod"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(apply, "start=true") || strings.Contains(apply, "start = true") {
		t.Fatalf("apply prompt must not invent start=true: %q", apply)
	}
	if !strings.Contains(apply, "pm_validate") || !strings.Contains(apply, "pm_diff") || !strings.Contains(apply, "pm_apply") {
		t.Fatalf("apply prompt missing validate/diff/apply: %q", apply)
	}
	if !strings.Contains(apply, "prod") {
		t.Fatalf("apply prompt missing profile: %q", apply)
	}

	start, err := prompts.Render("pmmcp_start_safe", map[string]string{
		"name": "web", "argv_json": `["npm","run","dev"]`, "project": "/proj",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, needle := range []string{"pm_start", "`command`", `["npm","run","dev"]`, "/proj", "daemon_unavailable", "strict"} {
		if !strings.Contains(start, needle) {
			t.Errorf("start_safe missing %q in: %q", needle, start)
		}
	}

	run, err := prompts.Render("pmmcp_oneshot_task", map[string]string{
		"argv_json": `["./bin/task"]`, "timeout": "30",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, needle := range []string{"pm_run", "wait", "timeout_sec", "30", `["./bin/task"]`} {
		if !strings.Contains(run, needle) {
			t.Errorf("oneshot missing %q in: %q", needle, run)
		}
	}

	dbg, err := prompts.Render("pmmcp_debug_crash", map[string]string{"name": "api"})
	if err != nil {
		t.Fatal(err)
	}
	for _, needle := range []string{"pm_status", "pm_errors", "pm_logs", "pm_events", "api"} {
		if !strings.Contains(dbg, needle) {
			t.Errorf("debug_crash missing %q in: %q", needle, dbg)
		}
	}
}
