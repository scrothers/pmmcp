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
	"reflect"
	"testing"

	"github.com/scrothers/pmmcp/internal/api"
)

func TestPayloadFromArgsJSONPayload(t *testing.T) {
	t.Parallel()
	pl := payloadFromArgs([]string{"--json", `{"name":"web","replace":true}`})
	if pl["name"] != "web" {
		t.Fatalf("name = %v", pl["name"])
	}
	if pl["replace"] != true {
		t.Fatalf("replace = %v (%T), want bool true", pl["replace"], pl["replace"])
	}
}

func TestPayloadFromArgsCoercion(t *testing.T) {
	t.Parallel()
	pl := payloadFromArgs([]string{"--lines", "50", "--follow", "true", "--name", "api"})
	if pl["lines"] != 50 {
		t.Fatalf("lines = %v (%T), want int 50", pl["lines"], pl["lines"])
	}
	if pl["follow"] != true {
		t.Fatalf("follow = %v (%T), want bool true", pl["follow"], pl["follow"])
	}
	if pl["name"] != "api" {
		t.Fatalf("name = %v", pl["name"])
	}
}

func TestPayloadFromArgsTypedColonEquals(t *testing.T) {
	t.Parallel()
	pl := payloadFromArgs([]string{`ports:=["8080","9090"]`, "count:=3"})
	ports, ok := pl["ports"].([]any)
	if !ok || len(ports) != 2 || ports[0] != "8080" {
		t.Fatalf("ports = %v (%T)", pl["ports"], pl["ports"])
	}
	if pl["count"] != float64(3) {
		t.Fatalf("count = %v (%T), want json number 3", pl["count"], pl["count"])
	}
}

func TestCoerce(t *testing.T) {
	t.Parallel()
	cases := map[string]any{"5": 5, "true": true, "false": false, "hello": "hello", "8080": 8080}
	for in, want := range cases {
		if got := coerce(in); got != want {
			t.Errorf("coerce(%q) = %v (%T), want %v", in, got, got, want)
		}
	}
}

// TestStartPayload covers startPayload's field mapping: name, cwd, sandbox,
// project, and command must each land in the returned api.StartPayload
// unchanged, with cwd and project kept distinct (neither clobbers the other).
func TestStartPayload(t *testing.T) {
	t.Parallel()
	got := startPayload("x", "/a", "strict", "/b", []string{"cmd", "arg1"})
	want := api.StartPayload{Name: "x", Cwd: "/a", Sandbox: "strict", Project: "/b", Command: []string{"cmd", "arg1"}}
	if got.Name != want.Name {
		t.Errorf("name = %q, want %q", got.Name, want.Name)
	}
	if got.Cwd != want.Cwd {
		t.Errorf("cwd = %q, want %q (project must not clobber cwd)", got.Cwd, want.Cwd)
	}
	if got.Sandbox != want.Sandbox {
		t.Errorf("sandbox = %q, want %q", got.Sandbox, want.Sandbox)
	}
	if got.Project != want.Project {
		t.Errorf("project = %q, want %q", got.Project, want.Project)
	}
	if !reflect.DeepEqual(got.Command, want.Command) {
		t.Errorf("command = %v, want %v", got.Command, want.Command)
	}
}

func TestIDOrNamePayload(t *testing.T) {
	t.Parallel()
	if pl := idOrNamePayload([]string{"grp-123"}); pl["id"] != "grp-123" {
		t.Errorf("grp- prefix should map to id: %v", pl)
	}
	if pl := idOrNamePayload([]string{"web"}); pl["name"] != "web" {
		t.Errorf("bare token should map to name: %v", pl)
	}
	if pl := idOrNamePayload(nil); len(pl) != 0 {
		t.Errorf("empty args should yield empty payload: %v", pl)
	}
}

func TestSchemaForTool(t *testing.T) {
	t.Parallel()
	s := schemaForTool("pm_start")
	req, ok := s["required"].([]string)
	if !ok || len(req) != 2 {
		t.Fatalf("pm_start schema required = %v", s["required"])
	}
	// Unknown tool falls back to the generic object schema.
	if schemaForTool("pm_unknown")["type"] != "object" {
		t.Fatalf("generic schema missing type=object")
	}
}
