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

package secret

import (
	"regexp"
	"sort"
	"strings"
	"sync"
)

// Redacted is the placeholder used when no key name is associated with a value.
const Redacted = "***REDACTED***"

// minRegisteredValueLen is the shortest secret value that is registered for
// value-based redaction. Registering very short values (e.g. "1") would redact
// them everywhere they incidentally appear and corrupt logs, so they are
// skipped — key-name and pattern redaction still apply.
const minRegisteredValueLen = 5

// RedactedFor returns the frozen, debuggable placeholder for a named secret:
// ***REDACTED:NAME***. Downstream tooling keys on this stable format.
func RedactedFor(name string) string {
	if name == "" {
		return Redacted
	}
	return "***REDACTED:" + name + "***"
}

// sensitiveKey matches env keys whose *values* should be redacted
// (case-insensitive substring: TOKEN, SECRET, PASSWORD, API_KEY).
var sensitiveKey = regexp.MustCompile(`(?i)(TOKEN|SECRET|PASSWORD|API_KEY)`)

// assignRe matches KEY=value anywhere in a line when KEY looks sensitive,
// including embedded forms (SERVICE_API_TOKEN=…) and mid-line occurrences.
var assignRe = regexp.MustCompile(`(?i)\b([A-Za-z0-9_]*(?:TOKEN|SECRET|PASSWORD|API_KEY)[A-Za-z0-9_]*)\s*=\s*("[^"]*"|'[^']*'|[^\s]+)`)

// jsonRe matches "key":"value" JSON pairs when the key looks sensitive.
var jsonRe = regexp.MustCompile(`(?i)"([^"]*(?:token|secret|password|api_key)[^"]*)"\s*:\s*"([^"]*)"`)

// globalPatterns are always-on secret shapes redacted wholesale, independent of
// key names or registration (AWS keys, GitHub tokens, JWTs, PEM private keys).
var globalPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?:AKIA|ASIA)[0-9A-Z]{16}`),
	regexp.MustCompile(`gh[pousr]_[A-Za-z0-9]{20,255}`),
	regexp.MustCompile(`eyJ[A-Za-z0-9_-]{5,}\.[A-Za-z0-9_-]{5,}\.[A-Za-z0-9_-]{5,}`),
	regexp.MustCompile(`-----BEGIN[A-Z ]*PRIVATE KEY-----`),
}

// regValue is a registered secret value and its placeholder.
type regValue struct {
	value       string
	placeholder string
}

// Redactor holds a process-local set of registered secret values and applies
// value-based, key-name, and global-pattern redaction. The zero value is not
// usable; construct with NewRedactor. A Redactor is safe for concurrent use.
type Redactor struct {
	mu     sync.RWMutex
	values []regValue // kept sorted longest-value-first
}

// NewRedactor creates an empty Redactor.
func NewRedactor() *Redactor { return &Redactor{} }

// RegisterNamedValue records a resolved secret value so it is replaced by
// RedactedFor(name) wherever it appears. Values shorter than
// minRegisteredValueLen and duplicates are ignored.
func (r *Redactor) RegisterNamedValue(name, value string) {
	if len(value) < minRegisteredValueLen {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, rv := range r.values {
		if rv.value == value {
			return
		}
	}
	r.values = append(r.values, regValue{value: value, placeholder: RedactedFor(name)})
	sort.Slice(r.values, func(i, j int) bool {
		return len(r.values[i].value) > len(r.values[j].value)
	})
}

// RegisterValue records a resolved secret value with the generic placeholder.
func (r *Redactor) RegisterValue(value string) { r.RegisterNamedValue("", value) }

// RedactLine scrubs a single line: registered values first, then global
// patterns, then sensitive KEY=value and JSON "key":"value" assignments.
func (r *Redactor) RedactLine(s string) string {
	if s == "" {
		return s
	}
	out := s
	r.mu.RLock()
	for _, rv := range r.values {
		if strings.Contains(out, rv.value) {
			out = strings.ReplaceAll(out, rv.value, rv.placeholder)
		}
	}
	r.mu.RUnlock()
	for _, re := range globalPatterns {
		out = re.ReplaceAllString(out, Redacted)
	}
	out = assignRe.ReplaceAllStringFunc(out, redactAssign)
	out = jsonRe.ReplaceAllStringFunc(out, redactJSON)
	return out
}

// redactValue scrubs a bare value (no KEY=): registered values + global patterns.
func (r *Redactor) redactValue(v string) string {
	if v == "" {
		return v
	}
	out := v
	r.mu.RLock()
	for _, rv := range r.values {
		if strings.Contains(out, rv.value) {
			out = strings.ReplaceAll(out, rv.value, rv.placeholder)
		}
	}
	r.mu.RUnlock()
	for _, re := range globalPatterns {
		out = re.ReplaceAllString(out, Redacted)
	}
	return out
}

// RedactMap returns a copy of m with sensitive keys' values replaced by the
// per-key placeholder; non-sensitive values are still scanned for registered
// secret values and global patterns so a secret under an innocuous key cannot leak.
func (r *Redactor) RedactMap(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		if sensitiveKey.MatchString(k) {
			out[k] = RedactedFor(k)
			continue
		}
		out[k] = r.redactValue(v)
	}
	return out
}

func redactAssign(match string) string {
	sm := assignRe.FindStringSubmatch(match)
	if len(sm) < 3 {
		return match
	}
	return sm[1] + "=" + RedactedFor(sm[1])
}

func redactJSON(match string) string {
	sm := jsonRe.FindStringSubmatch(match)
	if len(sm) < 3 {
		return match
	}
	return `"` + sm[1] + `":"` + RedactedFor(sm[1]) + `"`
}

// defaultRedactor backs the package-level redaction functions. It is a
// deliberate process-local singleton (like the redaction set the spec mandates):
// the log-capture and webhook pipelines call the package RedactLine, so registered
// values must be visible through one shared instance.
var defaultRedactor = NewRedactor()

// RegisterValue registers a resolved secret value with the package redactor so
// RedactLine/RedactMap scrub it. Callers that resolve secrets (daemon, telemetry)
// register values here after ResolveEnvMap.
func RegisterValue(value string) { defaultRedactor.RegisterValue(value) }

// RegisterNamedValue registers value under name with the package redactor.
func RegisterNamedValue(name, value string) { defaultRedactor.RegisterNamedValue(name, value) }

// RedactLine redacts a single line using the package redactor.
func RedactLine(s string) string { return defaultRedactor.RedactLine(s) }

// RedactMap redacts a map using the package redactor.
func RedactMap(m map[string]string) map[string]string { return defaultRedactor.RedactMap(m) }
