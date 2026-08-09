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
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestRegisterHelpers(t *testing.T) {
	t.Parallel()
	// The register* helpers must wire the server without panicking (they build
	// tool schemas, resource templates, and prompt argument lists).
	server := mcp.NewServer(&mcp.Implementation{Name: "pmmcp", Version: "test"}, nil)
	registerTools(server, "unused")
	registerResources(server, "unused")
	registerPrompts(server, "unused")
}

func TestResolveDaemonPath(t *testing.T) {
	t.Parallel()
	// --bin override is made absolute.
	got, err := resolveDaemonPath("bin/pmmcpd")
	if err != nil {
		t.Fatalf("resolveDaemonPath: %v", err)
	}
	if !strings.HasSuffix(got, filepath.Join("bin", "pmmcpd")) || !filepath.IsAbs(got) {
		t.Fatalf("resolveDaemonPath = %q, want absolute", got)
	}
	// No override: resolves via sibling/PATH or errors — either way an absolute
	// path or a clear error, never a bare unresolvable name.
	if p, err := resolveDaemonPath(""); err == nil && !filepath.IsAbs(p) {
		t.Fatalf("resolveDaemonPath(\"\") = %q, want absolute or error", p)
	}
}

func TestMethodSet(t *testing.T) {
	t.Parallel()
	set := MethodSet()
	if len(set) == 0 {
		t.Fatal("MethodSet empty")
	}
}

func TestCommandForToolOmitted(t *testing.T) {
	t.Parallel()
	if CommandForTool("pm_logs_subscribe") != "" {
		t.Fatal("omitted tool should return empty verb")
	}
	if CommandForTool("pm_start") != "start" {
		t.Fatal("pm_start should map to start")
	}
}

// withStdin replaces os.Stdin with a pipe carrying content for the duration of fn.
func withStdin(t *testing.T, content string, fn func()) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = orig })
	go func() {
		_, _ = w.WriteString(content)
		_ = w.Close()
	}()
	fn()
	_ = r.Close()
}

func TestSecretSetStdin(t *testing.T) {
	endpoint := startMCPDaemon(t)
	t.Setenv("PMMCP_IPC_ENDPOINT", endpoint)
	ctx := context.Background()

	withStdin(t, "hunter2\n", func() {
		if err := (&rootState{}).secretSet(ctx, []string{"--name", "db_password"}); err != nil {
			t.Errorf("secretSet stdin: %v", err)
		}
	})
}

func TestSecretSetRejectsValueOnArgv(t *testing.T) {
	t.Setenv("PMMCP_IPC_ENDPOINT", startMCPDaemon(t))
	if err := (&rootState{}).secretSet(context.Background(), []string{"--name", "x", "--value", "leak"}); err == nil {
		t.Fatal("value on argv must be rejected")
	}
}

func TestSecretSetMissingName(t *testing.T) {
	t.Setenv("PMMCP_IPC_ENDPOINT", startMCPDaemon(t))
	if err := (&rootState{}).secretSet(context.Background(), nil); err == nil {
		t.Fatal("missing name must error")
	}
}

func TestSecretSetEmptyStdin(t *testing.T) {
	t.Setenv("PMMCP_IPC_ENDPOINT", startMCPDaemon(t))
	withStdin(t, "\n", func() {
		if err := (&rootState{}).secretSet(context.Background(), []string{"--name", "x"}); err == nil {
			t.Error("empty stdin must error")
		}
	})
}
