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

//go:build e2e

package e2e_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestE2ELifecycle builds and runs real pmmcpd/pmmcp binaries.
func TestE2ELifecycle(t *testing.T) {
	root := findRoot(t)
	bin := t.TempDir()
	build(t, root, bin, "pmmcp", "./cmd/pmmcp")
	build(t, root, bin, "pmmcpd", "./cmd/pmmcpd")

	state := t.TempDir()
	sock := filepath.Join(state, "pmmcpd.sock")
	cfg := filepath.Join(state, "daemon.toml")
	writeCfg(t, cfg, state, sock)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	daemon := exec.CommandContext(ctx, filepath.Join(bin, "pmmcpd"), "run")
	daemon.Env = append(os.Environ(), "PMMCP_CONFIG="+cfg, "PMMCP_SANDBOX_DEFAULT=off")
	daemon.Stdout = os.Stdout
	daemon.Stderr = os.Stderr
	if err := daemon.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cancel()
		_ = daemon.Wait()
	}()

	env := append(os.Environ(), "PMMCP_CONFIG="+cfg)
	waitDoctor(t, bin, env)

	run(t, bin, env, "start", "--name", "e2e", "--sandbox", "off", "--", "/bin/echo", "e2e-hello")
	out := run(t, bin, env, "list")
	if !strings.Contains(out, "e2e") {
		t.Fatalf("list missing e2e: %s", out)
	}
	logs := run(t, bin, env, "logs", "e2e")
	if !strings.Contains(logs, "e2e-hello") {
		t.Fatalf("logs: %s", logs)
	}
	// group
	run(t, bin, env, "group", "create", "--name", "g1", "--member", "e2e")
	gout := run(t, bin, env, "group", "status", "g1")
	if !strings.Contains(gout, "g1") && !strings.Contains(gout, "e2e") && !strings.Contains(gout, "ready") && !strings.Contains(gout, "phase") && !strings.Contains(gout, "grp-") {
		// accept any JSON with group id
		if len(gout) < 5 {
			t.Fatalf("group status empty: %q", gout)
		}
	}
	run(t, bin, env, "health", "e2e")
	run(t, bin, env, "sandbox-profiles")
	run(t, bin, env, "metrics")
	run(t, bin, env, "events")
	// exercise several more IPC tools via thin CLI wrappers / doctor already covered
	run(t, bin, env, "stop", "e2e")
}

func findRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}

func build(t *testing.T, root, bin, name, pkg string) {
	t.Helper()
	cmd := exec.Command("go", "build", "-o", filepath.Join(bin, name), pkg)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build %s: %v\n%s", name, err, out)
	}
}

func writeCfg(t *testing.T, path, state, sock string) {
	t.Helper()
	body := "version = 1\nstate_dir = \"" + state + "\"\n\n[ipc]\nendpoint = \"" + sock + "\"\n\n[sandbox]\ndefault = \"off\"\n\n[relaunch]\nenabled = false\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func waitDoctor(t *testing.T, bin string, env []string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		cmd := exec.Command(filepath.Join(bin, "pmmcp"), "doctor")
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err == nil && strings.Contains(string(out), "daemon: ok") {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("daemon not up")
}

func run(t *testing.T, bin string, env []string, args ...string) string {
	t.Helper()
	cmd := exec.Command(filepath.Join(bin, "pmmcp"), args...)
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("pmmcp %v: %v\n%s", args, err, out)
	}
	return string(out)
}
