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

package daemon_test

import (
	"context"
	"strings"
	"testing"

	"github.com/scrothers/pmmcp/internal/api"
	"github.com/scrothers/pmmcp/internal/config"
	"github.com/scrothers/pmmcp/internal/secret"
)

// TestStartRegistersResolvedSecretForRedaction is the #4 regression: doStart
// must register values resolved from secret:// refs with the redactor so they
// are scrubbed from captured process logs. Global patterns already cover
// AWS/JWT/PEM shapes; a custom token would otherwise leak in clear.
func TestStartRegistersResolvedSecretForRedaction(t *testing.T) {
	const secretVal = "s3cr3t-token-abc123-xyz"
	t.Setenv("PMMCP_TEST_SECRET_SRC", secretVal)

	// Precondition: the value is not already scrubbed by a global pattern, so a
	// pass genuinely exercises named-value registration, not incidental cover.
	if secret.RedactLine(secretVal) != secretVal {
		t.Skip("value matches a global redaction pattern; choose another")
	}

	c, mgr := newTestDaemon(t, func(cfg *config.Config) { cfg.Sandbox.Default = "off" })
	c.SetSession("sess-full", "full")
	ctx := context.Background()

	var start api.StartResult
	if err := c.Call(ctx, api.MethodStart, api.StartPayload{
		Name:    "svc",
		Command: []string{"/bin/true"},
		Env:     map[string]string{"TOKEN": "secret://env:PMMCP_TEST_SECRET_SRC"},
		Sandbox: "off",
	}, &start); err != nil {
		t.Fatalf("start: %v", err)
	}

	// Resolution must have produced the plaintext for the child; otherwise the
	// redaction assertion below would be vacuous.
	if got := envValue(mgr, "svc", "TOKEN"); got != secretVal {
		t.Fatalf("resolved TOKEN = %q, want %q (resolution failed)", got, secretVal)
	}

	// The daemon must have registered the resolved value with the redactor.
	line := "connecting db TOKEN=" + secretVal + " done"
	if got := secret.RedactLine(line); strings.Contains(got, secretVal) {
		t.Fatalf("resolved secret leaked into logs (not registered for redaction): %q", got)
	}
}

// envValue returns the resolved value of key in the recorded StartSpec for the
// named process, or "" if absent.
func envValue(mgr *fakeMgr, name, key string) string {
	mgr.mu.Lock()
	defer mgr.mu.Unlock()
	prefix := key + "="
	for _, sp := range mgr.specs {
		if sp.Name != name {
			continue
		}
		for _, kv := range sp.Env {
			if strings.HasPrefix(kv, prefix) {
				return strings.TrimPrefix(kv, prefix)
			}
		}
	}
	return ""
}
