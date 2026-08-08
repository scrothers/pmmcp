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

package secret_test

import (
	"strings"
	"testing"

	"github.com/scrothers/pmmcp/internal/secret"
)

func TestRedactorRegisteredValue(t *testing.T) {
	t.Parallel()
	r := secret.NewRedactor()
	r.RegisterNamedValue("DB_PASSWORD", "hunter2xyz")
	// The registered value is scrubbed even in free text with no KEY=.
	got := r.RedactLine("connecting with hunter2xyz to db")
	want := "connecting with " + secret.RedactedFor("DB_PASSWORD") + " to db"
	if got != want {
		t.Fatalf("registered value not redacted: %q", got)
	}
	// Short values are not registered (would corrupt logs).
	r.RegisterNamedValue("TINY", "ab")
	if got := r.RedactLine("value ab here"); got != "value ab here" {
		t.Fatalf("short value should not be registered: %q", got)
	}
}

func TestRedactorGlobalPatterns(t *testing.T) {
	t.Parallel()
	r := secret.NewRedactor()
	cases := []string{
		"aws key AKIAIOSFODNN7EXAMPLE here",
		"token ghp_0123456789abcdefghijklmnopqrstuvwxyz done",
		"jwt eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY." + strings.Repeat("a", 10) + " tail",
		"-----BEGIN RSA PRIVATE KEY-----",
	}
	for _, in := range cases {
		got := r.RedactLine(in)
		if !strings.Contains(got, secret.Redacted) {
			t.Errorf("global pattern not redacted: RedactLine(%q) = %q", in, got)
		}
	}
	// A benign line is untouched.
	if got := r.RedactLine("just a normal log line"); got != "just a normal log line" {
		t.Fatalf("benign line changed: %q", got)
	}
}

func TestPackageRegisterValue(t *testing.T) {
	t.Parallel()
	// Distinctive value so it cannot collide with other tests in this binary.
	const v = "zzq-package-secret-9f3k"
	secret.RegisterValue(v)
	got := secret.RedactLine("leaking " + v + " oops")
	if strings.Contains(got, v) {
		t.Fatalf("package RegisterValue did not scrub: %q", got)
	}
	if !strings.Contains(got, secret.Redacted) {
		t.Fatalf("expected placeholder: %q", got)
	}
}

func TestRedactorMapValueScan(t *testing.T) {
	t.Parallel()
	r := secret.NewRedactor()
	r.RegisterNamedValue("API_TOKEN", "supersecretvalue")
	// A registered secret hidden under an innocuous key is still scrubbed.
	out := r.RedactMap(map[string]string{
		"INNOCENT": "supersecretvalue",
		"SAFE":     "plain",
	})
	if out["INNOCENT"] == "supersecretvalue" {
		t.Fatalf("registered value leaked via innocuous key: %q", out["INNOCENT"])
	}
	if out["SAFE"] != "plain" {
		t.Fatalf("SAFE changed: %q", out["SAFE"])
	}
}

func TestRegisterNamedValueDeduplicates(t *testing.T) {
	t.Parallel()
	r := secret.NewRedactor()
	r.RegisterNamedValue("API_TOKEN", "duplicate-value-1")
	// Re-registering the same value (even under a different name) is a no-op;
	// the first registration's placeholder wins.
	r.RegisterNamedValue("OTHER_NAME", "duplicate-value-1")
	got := r.RedactLine("has duplicate-value-1 in it")
	want := "has " + secret.RedactedFor("API_TOKEN") + " in it"
	if got != want {
		t.Fatalf("RedactLine after duplicate registration = %q, want %q", got, want)
	}
}

func TestRedactMapEmptyNonSensitiveValue(t *testing.T) {
	t.Parallel()
	out := secret.RedactMap(map[string]string{"PLAIN": ""})
	if out["PLAIN"] != "" {
		t.Fatalf("empty non-sensitive value changed: %q", out["PLAIN"])
	}
}

func TestRedactedFor(t *testing.T) {
	t.Parallel()
	if got := secret.RedactedFor("API_TOKEN"); got != "***REDACTED:API_TOKEN***" {
		t.Fatalf("placeholder = %q", got)
	}
	if got := secret.RedactedFor(""); got != secret.Redacted {
		t.Fatalf("empty name placeholder = %q, want %q", got, secret.Redacted)
	}
}
