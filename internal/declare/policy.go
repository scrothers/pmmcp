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
	"net"
	"net/url"
	"path/filepath"
	"slices"
	"strings"
)

// Policy violation codes for hostile/deny-by-default declare validation.
const (
	CodeSandboxRequired  = "sandbox_required"
	CodeArgvShellRisk    = "argv_shell_risk"
	CodePathOutsideProj  = "path_outside_project"
	CodePortPrivileged   = "port_privileged"
	CodeWebhookURLDenied = "webhook_url_denied"
)

// ValidationError is one structured policy violation with a document path.
type ValidationError struct {
	Code    string `json:"code"`
	Path    string `json:"path"`
	Message string `json:"message"`
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("%s at %s: %s", e.Code, e.Path, e.Message)
}

// ValidationErrors is the collected set of policy violations for a document.
// It reports errors.Is(err, ErrInvalid) so callers that key off ErrInvalid keep
// working, while errors.As(err, &ValidationErrors{}) recovers the full list.
type ValidationErrors []ValidationError

func (e ValidationErrors) Error() string {
	if len(e) == 0 {
		return "declare: validation failed"
	}
	parts := make([]string, 0, len(e))
	for _, v := range e {
		parts = append(parts, v.Error())
	}
	return fmt.Sprintf("declare: validation failed (%d): %s", len(e), strings.Join(parts, "; "))
}

// Unwrap ties ValidationErrors to ErrInvalid for errors.Is.
func (e ValidationErrors) Unwrap() error { return ErrInvalid }

// Codes returns the distinct violation codes present, in first-seen order.
func (e ValidationErrors) Codes() []string {
	seen := map[string]bool{}
	var out []string
	for _, v := range e {
		if !seen[v.Code] {
			seen[v.Code] = true
			out = append(out, v.Code)
		}
	}
	return out
}

// ValidateOption tunes the policy pass. All options relax the default-deny
// posture for explicitly trusted inputs (daemon config / power users).
type ValidateOption func(*validateConfig)

type validateConfig struct {
	projectRoot      string
	allowSandboxOff  bool
	allowShellArgv   bool
	allowPrivPorts   bool
	webhookAllowlist []string
}

func newValidateConfig(opts []ValidateOption) validateConfig {
	var c validateConfig
	for _, o := range opts {
		o(&c)
	}
	return c
}

// WithProjectRoot sets the project root used for watch-path containment.
func WithProjectRoot(root string) ValidateOption {
	return func(c *validateConfig) { c.projectRoot = root }
}

// WithAllowSandboxOff permits sandbox: "off" (daemon non-strict mode only).
func WithAllowSandboxOff() ValidateOption {
	return func(c *validateConfig) { c.allowSandboxOff = true }
}

// WithAllowShellArgv permits shell-invoking argv (allow_shell_argv=true).
func WithAllowShellArgv() ValidateOption {
	return func(c *validateConfig) { c.allowShellArgv = true }
}

// WithAllowPrivilegedPorts permits host ports below 1024.
func WithAllowPrivilegedPorts() ValidateOption {
	return func(c *validateConfig) { c.allowPrivPorts = true }
}

// WithWebhookAllowlist restricts webhook hosts to the given allowlist.
func WithWebhookAllowlist(hosts ...string) ValidateOption {
	return func(c *validateConfig) { c.webhookAllowlist = hosts }
}

// unit is one validatable service-like node with its document path.
type unit struct {
	path string
	spec ServiceSpec
}

// policyCheck walks every service, member, job, default, and webhook and
// collects policy violations.
func (d *Document) policyCheck(cfg validateConfig) ValidationErrors {
	errs := make(ValidationErrors, 0, 8)
	errs = append(errs, d.checkSandboxDefaults(cfg)...)
	for _, u := range d.units() {
		errs = append(errs, checkUnit(u, cfg)...)
	}
	errs = append(errs, d.checkWebhooks(cfg)...)
	return errs
}

// units flattens all service-like nodes from both authored shapes.
func (d *Document) units() []unit {
	var out []unit
	for key, spec := range d.Services {
		out = append(out, unit{path: "services." + key, spec: spec})
	}
	if d.Spec != nil {
		for gi, g := range d.Spec.Groups {
			for mi, m := range g.Members {
				out = append(out, unit{
					path: fmt.Sprintf("spec.groups[%d].members[%d]", gi, mi),
					spec: m,
				})
			}
		}
		for ji, j := range d.Spec.Jobs {
			out = append(out, unit{path: fmt.Sprintf("spec.jobs[%d]", ji), spec: j})
		}
	}
	return out
}

func (d *Document) checkSandboxDefaults(cfg validateConfig) ValidationErrors {
	if cfg.allowSandboxOff || d.Spec == nil || d.Spec.Defaults == nil {
		return nil
	}
	if strings.EqualFold(strings.TrimSpace(d.Spec.Defaults.Sandbox), "off") {
		return ValidationErrors{{
			Code:    CodeSandboxRequired,
			Path:    "spec.defaults.sandbox",
			Message: "sandbox 'off' forbidden by policy",
		}}
	}
	return nil
}

func checkUnit(u unit, cfg validateConfig) ValidationErrors {
	var errs ValidationErrors
	if !cfg.allowSandboxOff && strings.EqualFold(strings.TrimSpace(u.spec.Sandbox), "off") {
		errs = append(errs, ValidationError{
			Code:    CodeSandboxRequired,
			Path:    u.path + ".sandbox",
			Message: "sandbox 'off' forbidden by policy",
		})
	}
	if !cfg.allowShellArgv && shellRiskArgv(u.spec.Argv) {
		errs = append(errs, ValidationError{
			Code:    CodeArgvShellRisk,
			Path:    u.path + ".argv",
			Message: "argv invokes a shell with an inline command; blocked (allow_shell_argv=false)",
		})
	}
	errs = append(errs, checkPorts(u, cfg)...)
	errs = append(errs, checkWatch(u, cfg)...)
	return errs
}

func checkPorts(u unit, cfg validateConfig) ValidationErrors {
	if cfg.allowPrivPorts {
		return nil
	}
	var errs ValidationErrors
	for i, p := range u.spec.Ports {
		host := p.HostFacingPort()
		if host > 0 && host < 1024 {
			errs = append(errs, ValidationError{
				Code:    CodePortPrivileged,
				Path:    fmt.Sprintf("%s.ports[%d].port", u.path, i),
				Message: fmt.Sprintf("host port %d requires elevated privileges; denied for local driver", host),
			})
		}
	}
	return errs
}

func checkWatch(u unit, cfg validateConfig) ValidationErrors {
	if u.spec.Watch == nil {
		return nil
	}
	var errs ValidationErrors
	for i, p := range u.spec.Watch.Paths {
		if pathOutsideProject(p, cfg.projectRoot) {
			errs = append(errs, ValidationError{
				Code:    CodePathOutsideProj,
				Path:    fmt.Sprintf("%s.watch.paths[%d]", u.path, i),
				Message: fmt.Sprintf("watch path %s is outside the project root", p),
			})
		}
	}
	return errs
}

func (d *Document) checkWebhooks(cfg validateConfig) ValidationErrors {
	if d.Spec == nil {
		return nil
	}
	var errs ValidationErrors
	for i, w := range d.Spec.Webhooks {
		if reason, denied := webhookDenied(w.URL, cfg.webhookAllowlist); denied {
			errs = append(errs, ValidationError{
				Code:    CodeWebhookURLDenied,
				Path:    fmt.Sprintf("spec.webhooks[%d].url", i),
				Message: reason,
			})
		}
	}
	return errs
}

// shellRiskArgv reports whether argv invokes a shell with an inline command
// string. bash -c, sh -lc, powershell -Command,
// cmd /c are all treated as smuggling risks.
func shellRiskArgv(argv []string) bool {
	if len(argv) == 0 {
		return false
	}
	base := strings.ToLower(filepath.Base(argv[0]))
	base = strings.TrimSuffix(base, ".exe")
	rest := argv[1:]
	switch base {
	case "sh", "bash", "dash", "zsh", "ksh", "ash", "fish":
		for _, a := range rest {
			// -c, -lc, -ic and similar combined short flags carry a command.
			if strings.HasPrefix(a, "-") && strings.Contains(a, "c") {
				return true
			}
		}
	case "powershell", "pwsh":
		for _, a := range rest {
			la := strings.ToLower(a)
			if strings.HasPrefix(la, "-c") || strings.HasPrefix(la, "-enc") {
				return true
			}
		}
	case "cmd":
		for _, a := range rest {
			la := strings.ToLower(a)
			if la == "/c" || la == "/k" {
				return true
			}
		}
	}
	return false
}

// pathOutsideProject reports whether p escapes the project root. With no root,
// any absolute path or parent-escaping relative path is treated as outside.
func pathOutsideProject(p, root string) bool {
	if strings.TrimSpace(p) == "" {
		return false
	}
	if root == "" {
		if filepath.IsAbs(p) {
			return true
		}
		clean := filepath.Clean(p)
		return clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator))
	}
	abs := p
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(root, p)
	}
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(abs))
	if err != nil {
		return true
	}
	return rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// webhookDenied applies the SSRF policy. Loopback, link-local,
// metadata, unspecified, multicast, and private addresses are denied; when an
// allowlist is set, only listed hosts pass.
func webhookDenied(raw string, allowlist []string) (string, bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return "webhook url is empty or malformed", true
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Sprintf("webhook scheme %q not allowed (http/https only)", u.Scheme), true
	}
	host := u.Hostname()
	if len(allowlist) > 0 {
		// An explicit allowlist is the operator's trust decision; membership
		// bypasses the address heuristics, absence is an outright denial.
		if slices.Contains(allowlist, host) {
			return "", false
		}
		return "host not allowlisted", true
	}
	if ip := net.ParseIP(host); ip != nil {
		if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
			ip.IsPrivate() || ip.IsUnspecified() || ip.IsMulticast() {
			return "host not allowlisted / private link-local address forbidden", true
		}
		return "", false
	}
	lower := strings.ToLower(host)
	if lower == "localhost" || strings.HasSuffix(lower, ".localhost") ||
		lower == "metadata.google.internal" {
		return "loopback / metadata host forbidden", true
	}
	return "", false
}
