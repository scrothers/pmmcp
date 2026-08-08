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

package declare_test

import (
	"errors"
	"testing"

	"github.com/scrothers/pmmcp/internal/declare"
)

func TestParseAndValidateSample(t *testing.T) {
	t.Parallel()
	const sample = `
apiVersion: pmmcp.dev/v1alpha1
kind: Project
metadata:
  name: marketplace
services:
  db:
    image: docker.io/library/postgres:16
    runtime: podman
  api:
    argv: ["./bin/api", "--listen", "127.0.0.1:8080"]
    runtime: local
    sandbox: strict
    depends_on:
      db:
        condition: service_started
  worker:
    name: worker
    argv: ["./bin/worker"]
    oneshot: false
`
	doc, err := declare.Parse([]byte(sample))
	if err != nil {
		t.Fatal(err)
	}
	if doc.APIVersion != declare.CanonicalAPIVersion {
		t.Fatalf("apiVersion %q", doc.APIVersion)
	}
	if doc.Kind != "Project" {
		t.Fatalf("kind %q", doc.Kind)
	}
	if doc.Metadata["name"] != "marketplace" {
		t.Fatalf("metadata.name %v", doc.Metadata["name"])
	}
	if len(doc.Services) != 3 {
		t.Fatalf("services len %d", len(doc.Services))
	}
	api := doc.Services["api"]
	if api.Name != "api" {
		t.Fatalf("api name defaulted to %q", api.Name)
	}
	if len(api.Argv) != 3 {
		t.Fatalf("api argv %v", api.Argv)
	}
	if _, ok := api.DependsOn["db"]; !ok {
		t.Fatalf("api depends_on missing db: %v", api.DependsOn)
	}
	if err := doc.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
}

func TestEmptyServicesOK(t *testing.T) {
	t.Parallel()
	const sample = `
apiVersion: pmmcp.dev/v1alpha1
kind: Project
metadata:
  name: empty
`
	doc, err := declare.Parse([]byte(sample))
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.Validate(); err != nil {
		t.Fatalf("empty services should be ok: %v", err)
	}
}

func TestMissingAPIVersion(t *testing.T) {
	t.Parallel()
	const sample = `
kind: Project
services:
  api:
    argv: ["./bin/api"]
`
	doc, err := declare.Parse([]byte(sample))
	if err != nil {
		t.Fatal(err)
	}
	err = doc.Validate()
	if !errors.Is(err, declare.ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid", err)
	}
	if err == nil || err.Error() == "" {
		t.Fatal("expected message")
	}
}

func TestBadAPIVersion(t *testing.T) {
	t.Parallel()
	const sample = `
apiVersion: pmmcp.dev/v0
kind: Project
`
	doc, err := declare.Parse([]byte(sample))
	if err != nil {
		t.Fatal(err)
	}
	err = doc.Validate()
	if !errors.Is(err, declare.ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid", err)
	}
}

func TestUnknownDependency(t *testing.T) {
	t.Parallel()
	const sample = `
apiVersion: pmmcp.dev/v1alpha1
services:
  api:
    argv: ["./bin/api"]
    depends_on:
      missing: {}
`
	doc, err := declare.Parse([]byte(sample))
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.Validate(); err == nil {
		t.Fatal("expected unknown depends_on error")
	}
}

func TestLocalNeedsArgvOrImage(t *testing.T) {
	t.Parallel()
	const sample = `
apiVersion: pmmcp.dev/v1alpha1
services:
  api:
    runtime: local
`
	doc, err := declare.Parse([]byte(sample))
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.Validate(); err == nil {
		t.Fatal("expected argv/image required")
	}
}

func TestParseEmptyBytes(t *testing.T) {
	t.Parallel()
	_, err := declare.Parse(nil)
	if !errors.Is(err, declare.ErrInvalid) {
		t.Fatalf("err = %v", err)
	}
}

func TestParseMalformedYAML(t *testing.T) {
	t.Parallel()
	_, err := declare.Parse([]byte("apiVersion: [\n"))
	if !errors.Is(err, declare.ErrInvalid) {
		t.Fatalf("err = %v", err)
	}
}

func TestValidateNilDocument(t *testing.T) {
	t.Parallel()
	var doc *declare.Document
	err := doc.Validate()
	if !errors.Is(err, declare.ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid", err)
	}
}

func TestValidateUnsupportedKindRejected(t *testing.T) {
	t.Parallel()
	const sample = `
apiVersion: pmmcp.dev/v1alpha1
kind: Deployment
`
	doc, err := declare.Parse([]byte(sample))
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.Validate(); !errors.Is(err, declare.ErrInvalid) {
		t.Fatalf("unsupported kind not rejected: %v", err)
	}
}

func TestPortSpecHostFacingPort(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		spec declare.PortSpec
		want int
	}{
		{"host_field", declare.PortSpec{Host: 8080, HostPort: 9090, Port: 3000}, 8080},
		{"host_port_field", declare.PortSpec{HostPort: 9090, Port: 3000}, 9090},
		{"port_field", declare.PortSpec{Port: 3000}, 3000},
		{"none_declared", declare.PortSpec{}, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.spec.HostFacingPort(); got != tc.want {
				t.Errorf("HostFacingPort() = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestDependsOnDecodeErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		yaml string
	}{
		{
			name: "sequence_of_mappings",
			yaml: `apiVersion: pmmcp.dev/v1alpha1
kind: Project
services:
  api:
    argv: ["./bin/api"]
    depends_on:
      - foo: bar
`,
		},
		{
			name: "scalar_not_mapping",
			yaml: `apiVersion: pmmcp.dev/v1alpha1
kind: Project
services:
  api:
    argv: ["./bin/api"]
    depends_on: "oops"
`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, err := declare.Parse([]byte(tc.yaml)); !errors.Is(err, declare.ErrInvalid) {
				t.Fatalf("err = %v, want ErrInvalid", err)
			}
		})
	}
}

func TestValidateServicesStructuralRejections(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		doc     *declare.Document
		wantErr bool
	}{
		{
			name: "name_defaults_from_key_when_unset",
			doc: &declare.Document{
				APIVersion: declare.CanonicalAPIVersion,
				Kind:       "Project",
				Services: map[string]declare.ServiceSpec{
					"api": {Argv: []string{"./bin/api"}, Sandbox: "strict"},
				},
			},
			wantErr: false,
		},
		{
			name: "both_key_and_name_empty",
			doc: &declare.Document{
				APIVersion: declare.CanonicalAPIVersion,
				Kind:       "Project",
				Services:   map[string]declare.ServiceSpec{"": {}},
			},
			wantErr: true,
		},
		{
			name: "local_runtime_missing_argv_and_image",
			doc: &declare.Document{
				APIVersion: declare.CanonicalAPIVersion,
				Kind:       "Project",
				Services: map[string]declare.ServiceSpec{
					"api": {Name: "api", Runtime: "local"},
				},
			},
			wantErr: true,
		},
		{
			name: "container_runtime_missing_image",
			doc: &declare.Document{
				APIVersion: declare.CanonicalAPIVersion,
				Kind:       "Project",
				Services: map[string]declare.ServiceSpec{
					"api": {Name: "api", Runtime: "docker"},
				},
			},
			wantErr: true,
		},
		{
			name: "unknown_dependency",
			doc: &declare.Document{
				APIVersion: declare.CanonicalAPIVersion,
				Kind:       "Project",
				Services: map[string]declare.ServiceSpec{
					"api": {
						Name:      "api",
						Argv:      []string{"./bin/api"},
						Sandbox:   "strict",
						DependsOn: declare.DependsOn{"missing": map[string]any{}},
					},
				},
			},
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.doc.Validate()
			if tc.wantErr && !errors.Is(err, declare.ErrInvalid) {
				t.Fatalf("err = %v, want ErrInvalid", err)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
		})
	}
}
