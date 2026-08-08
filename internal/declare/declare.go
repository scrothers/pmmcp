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
	"bytes"
	"errors"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// CanonicalAPIVersion is the only supported document version.
const CanonicalAPIVersion = "pmmcp.dev/v1alpha1"

// ErrInvalid is returned for malformed or unsupported declarations.
var ErrInvalid = errors.New("declare: invalid")

// Document is a parsed pmmcp.yaml project declaration.
//
// Two authored shapes are accepted: a top-level services map
// (top-level services map) and a spec-wrapped form with groups/members/webhooks
// (examples 016/023). Both are validated by Validate.
type Document struct {
	APIVersion string                 `yaml:"apiVersion"`
	Kind       string                 `yaml:"kind"`
	Metadata   map[string]any         `yaml:"metadata"`
	Services   map[string]ServiceSpec `yaml:"services"`
	Spec       *Spec                  `yaml:"spec"`
}

// Spec is the spec-wrapped project body (examples 016/023).
type Spec struct {
	Profile  string        `yaml:"profile"`
	Defaults *Defaults     `yaml:"defaults"`
	Groups   []GroupSpec   `yaml:"groups"`
	Jobs     []ServiceSpec `yaml:"jobs"`
	Webhooks []WebhookSpec `yaml:"webhooks"`
}

// Defaults holds project-wide defaults applied to members (subset validated).
type Defaults struct {
	Sandbox string `yaml:"sandbox"`
}

// GroupSpec is a named group of member service specs.
type GroupSpec struct {
	Name        string        `yaml:"name"`
	Description string        `yaml:"description"`
	Members     []ServiceSpec `yaml:"members"`
}

// WebhookSpec is a declared outbound webhook (subject to SSRF policy, ).
type WebhookSpec struct {
	URL    string   `yaml:"url"`
	Events []string `yaml:"events"`
}

// ServiceSpec describes one service (or group member / job) unit.
type ServiceSpec struct {
	Name      string     `yaml:"name"`
	Argv      []string   `yaml:"argv"`
	Image     string     `yaml:"image"`
	Runtime   string     `yaml:"runtime"`
	Cwd       string     `yaml:"cwd"`
	DependsOn DependsOn  `yaml:"depends_on"`
	Sandbox   string     `yaml:"sandbox"`
	Oneshot   bool       `yaml:"oneshot"`
	Ports     []PortSpec `yaml:"ports"`
	Watch     *WatchSpec `yaml:"watch"`
}

// PortSpec is one declared port mapping. Multiple authored spellings are
// accepted; HostFacingPort collapses them to the host-facing value.
type PortSpec struct {
	Name          string `yaml:"name"`
	Port          int    `yaml:"port"`
	Host          int    `yaml:"host"`
	HostPort      int    `yaml:"host_port"`
	Container     int    `yaml:"container"`
	ContainerPort int    `yaml:"container_port"`
	Protocol      string `yaml:"protocol"`
	Bind          string `yaml:"bind"`
}

// HostFacingPort returns the host-facing port number (0 if none declared).
func (p PortSpec) HostFacingPort() int {
	switch {
	case p.Host > 0:
		return p.Host
	case p.HostPort > 0:
		return p.HostPort
	case p.Port > 0:
		return p.Port
	default:
		return 0
	}
}

// WatchSpec describes a hot-reload path watch.
type WatchSpec struct {
	Enabled bool     `yaml:"enabled"`
	Paths   []string `yaml:"paths"`
	Ignore  []string `yaml:"ignore"`
	Action  string   `yaml:"action"`
}

// DependsOn accepts both a mapping (name -> condition object) and a plain
// sequence of names. It is stored as a name-keyed map for uniform validation.
type DependsOn map[string]any

// UnmarshalYAML decodes depends_on from either a mapping or a sequence.
func (d *DependsOn) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.SequenceNode {
		var names []string
		if err := n.Decode(&names); err != nil {
			return err
		}
		m := make(map[string]any, len(names))
		for _, name := range names {
			m[name] = map[string]any{}
		}
		*d = m
		return nil
	}
	m := map[string]any{}
	if err := n.Decode(&m); err != nil {
		return err
	}
	*d = m
	return nil
}

// Parse unmarshals YAML bytes into a Document, ignoring unknown fields.
// It does not validate semantics; call Validate after Parse.
func Parse(data []byte) (*Document, error) {
	return parse(data, false)
}

// ParseStrict is Parse with unknown-field rejection (yaml.v3 KnownFields).
// A typo or a misplaced document (e.g. an unexpected top-level key) is an error
// rather than being silently dropped.
func ParseStrict(data []byte) (*Document, error) {
	return parse(data, true)
}

func parse(data []byte, strict bool) (*Document, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("%w: empty document", ErrInvalid)
	}
	var d Document
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(strict)
	if err := dec.Decode(&d); err != nil {
		return nil, fmt.Errorf("%w: yaml: %w", ErrInvalid, err)
	}
	// Normalize map keys onto Name when the name field is empty.
	for key, spec := range d.Services {
		if spec.Name == "" {
			spec.Name = key
			d.Services[key] = spec
		}
	}
	return &d, nil
}

// Validate checks required fields, supported apiVersion, and security policy.
//
// Structural problems (apiVersion, kind, missing argv, unknown dependency)
// return an ErrInvalid-wrapped error. Policy violations across every service,
// group member, job, and webhook are collected into a ValidationErrors so the
// caller sees all of them at once (hostile declare fixture). The variadic options are
// additive; Validate with no options applies the shipped default-deny policy.
func (d *Document) Validate(opts ...ValidateOption) error {
	if d == nil {
		return fmt.Errorf("%w: nil document", ErrInvalid)
	}
	cfg := newValidateConfig(opts)
	if strings.TrimSpace(d.APIVersion) == "" {
		return fmt.Errorf("%w: missing apiVersion", ErrInvalid)
	}
	if d.APIVersion != CanonicalAPIVersion {
		return fmt.Errorf("%w: unsupported apiVersion %q (want %s)", ErrInvalid, d.APIVersion, CanonicalAPIVersion)
	}
	if strings.TrimSpace(d.Kind) == "" {
		return fmt.Errorf("%w: missing kind (want Project)", ErrInvalid)
	}
	if d.Kind != "Project" {
		return fmt.Errorf("%w: unsupported kind %q", ErrInvalid, d.Kind)
	}
	if err := d.validateServices(); err != nil {
		return err
	}
	if errs := d.policyCheck(cfg); len(errs) > 0 {
		return errs
	}
	return nil
}

// validateServices checks structural requirements of the top-level services map.
func (d *Document) validateServices() error {
	for key, spec := range d.Services {
		name := spec.Name
		if name == "" {
			name = key
		}
		if name == "" {
			return fmt.Errorf("%w: service with empty name", ErrInvalid)
		}
		runtime := strings.TrimSpace(spec.Runtime)
		if runtime == "" || runtime == "local" {
			if len(spec.Argv) == 0 && spec.Image == "" {
				return fmt.Errorf("%w: service %q: argv or image required", ErrInvalid, name)
			}
		}
		if runtime == "podman" || runtime == "docker" || runtime == "container" {
			if spec.Image == "" {
				return fmt.Errorf("%w: service %q: image required for runtime %q", ErrInvalid, name, runtime)
			}
		}
		for dep := range spec.DependsOn {
			if _, ok := d.Services[dep]; !ok {
				return fmt.Errorf("%w: service %q depends_on unknown service %q", ErrInvalid, name, dep)
			}
		}
	}
	return nil
}
