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

//go:build darwin

package local_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/scrothers/pmmcp/internal/process"
	"github.com/scrothers/pmmcp/internal/process/local"
	sandboxdarwin "github.com/scrothers/pmmcp/internal/sandbox/darwin"
)

// TestStrictDeniesDotSSHDarwin: strict child cannot read ~/.ssh on macOS via sandbox-exec.
func TestStrictDeniesDotSSHDarwin(t *testing.T) {
	if !sandboxdarwin.SandboxExecAvailable() {
		if os.Getenv("PMMCP_REQUIRE_SANDBOX") != "" {
			t.Fatal("sandbox-exec required (PMMCP_REQUIRE_SANDBOX set)")
		}
		t.Skip("sandbox-exec required for strict isolation")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		t.Fatal(err)
	}
	secretPath := filepath.Join(sshDir, "id_test")
	const marker = "SECRET_KEY_MATERIAL_DO_NOT_LEAK"
	if err := os.WriteFile(secretPath, []byte(marker+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	project := t.TempDir()
	logDir := t.TempDir()
	m := local.New()
	ctx := context.Background()
	h, err := m.Start(ctx, process.StartSpec{
		ID:      "proc-01STRICTDARWIN0000000001",
		Command: []string{"/bin/cat", secretPath},
		Cwd:     project,
		LogDir:  logDir,
		Sandbox: "strict",
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	waited, err := m.Wait(waitCtx, h.ID)
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	out, _ := os.ReadFile(filepath.Join(logDir, "stdout.log"))
	errb, _ := os.ReadFile(filepath.Join(logDir, "stderr.log"))
	combined := string(out) + string(errb)
	if strings.Contains(combined, marker) {
		t.Fatalf("strict child leaked secret material; logs=%q exit=%v", combined, waited.ExitCode)
	}
	if waited.ExitCode != nil && *waited.ExitCode == 0 && strings.TrimSpace(string(out)) != "" {
		t.Fatalf("strict child exited 0 with output %q", out)
	}
}

func TestStrictFailClosedWithoutSandboxExec(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if sandboxdarwin.SandboxExecAvailable() {
		t.Fatal("expected sandbox-exec unavailable with empty PATH")
	}
	m := local.New()
	_, err := m.Start(context.Background(), process.StartSpec{
		ID:      "proc-01STRICTFAILDARWIN0000001",
		Command: []string{"/usr/bin/true"},
		Cwd:     t.TempDir(),
		Sandbox: "strict",
	})
	if err == nil {
		t.Fatal("expected sandbox fail closed")
	}
	if !errors.Is(err, process.ErrSandboxFailed) && !strings.Contains(err.Error(), "sandbox") {
		t.Fatalf("err = %v", err)
	}
}
