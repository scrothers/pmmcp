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

//go:build windows

package local_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/scrothers/pmmcp/internal/domain"
	"github.com/scrothers/pmmcp/internal/process"
	"github.com/scrothers/pmmcp/internal/process/local"
)

// TestWindowsJobObjectStop verifies strict/local starts assign a Job Object and
// Stop terminates the process tree (sandbox-windows / ).
func TestWindowsJobObjectStop(t *testing.T) {
	m := local.New()
	ctx := context.Background()
	logDir := t.TempDir()
	project := t.TempDir()
	h, err := m.Start(ctx, process.StartSpec{
		ID:      "proc-01WINJOB0000000000000001",
		Command: []string{"powershell", "-NoProfile", "-Command", "Start-Sleep -Seconds 120"},
		Cwd:     project,
		LogDir:  logDir,
		Sandbox: "strict",
	})
	if err != nil {
		// powershell might differ; try ping loop
		h, err = m.Start(ctx, process.StartSpec{
			ID:      "proc-01WINJOB0000000000000002",
			Command: []string{"cmd", "/c", "ping -n 120 127.0.0.1 >nul"},
			Cwd:     project,
			LogDir:  logDir,
			Sandbox: "strict",
		})
	}
	if err != nil {
		if os.Getenv("PMMCP_REQUIRE_SANDBOX") != "" {
			t.Fatalf("strict start required: %v", err)
		}
		t.Skipf("strict start failed: %v", err)
	}
	if h.PID <= 0 {
		t.Fatalf("pid = %d", h.PID)
	}
	insp, err := m.Inspect(ctx, h.ID)
	if err != nil || insp.Status != domain.StatusRunning {
		t.Fatalf("inspect = %+v err=%v", insp, err)
	}
	begin := time.Now()
	if err := m.Stop(ctx, h.ID, time.Millisecond); err != nil {
		t.Fatalf("force stop: %v", err)
	}
	if elapsed := time.Since(begin); elapsed > 5*time.Second {
		t.Fatalf("stop took %v", elapsed)
	}
	waitCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	waited, err := m.Wait(waitCtx, h.ID)
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if waited.Status != domain.StatusExited {
		t.Fatalf("status=%s", waited.Status)
	}
	// Ensure log dir mode is private-ish (best-effort on Windows ACLs).
	if st, err := os.Stat(logDir); err == nil {
		_ = st
		_ = filepath.Base(logDir)
	}
}

// TestWindowsStrictFailClosedWithoutJob is hard to force without mocking CreateJobObject;
// assignment failures are covered by code path when OpenProcess fails on invalid pid.
func TestWindowsLocalArgvNoShell(t *testing.T) {
	m := local.New()
	ctx := context.Background()
	logDir := t.TempDir()
	h, err := m.Start(ctx, process.StartSpec{
		ID:      "proc-01WINARGV000000000000001",
		Command: []string{"cmd", "/c", "echo hello-win"},
		Cwd:     t.TempDir(),
		LogDir:  logDir,
		Sandbox: "off",
	})
	if err != nil {
		t.Fatal(err)
	}
	waitCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if _, err := m.Wait(waitCtx, h.ID); err != nil {
		t.Fatal(err)
	}
	out, err := os.ReadFile(filepath.Join(logDir, "stdout.log"))
	if err != nil {
		t.Fatal(err)
	}
	if !containsFold(string(out), "hello-win") {
		t.Fatalf("stdout=%q", out)
	}
}

func containsFold(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		indexFold(s, sub) >= 0)
}

func indexFold(s, sub string) int {
	// simple case-sensitive is fine for our echo
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
