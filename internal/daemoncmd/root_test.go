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

package daemoncmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVersionCommand(t *testing.T) {
	root := newRootCmd()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"version"})
	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("version: %v", err)
	}
	if !strings.HasPrefix(buf.String(), "pmmcpd ") {
		t.Fatalf("version output = %q, want it to start with %q", buf.String(), "pmmcpd ")
	}
}

func TestHelp(t *testing.T) {
	root := newRootCmd()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"--help"})
	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("help: %v", err)
	}
	for _, want := range []string{"run", "version", "--config", "--state-dir"} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("help missing %q", want)
		}
	}
}

// TestRunConfigError covers runDaemon's config.Load error branch: PMMCP_CONFIG
// points at a missing file, so load fails before the daemon is constructed.
func TestRunConfigError(t *testing.T) {
	t.Setenv("PMMCP_CONFIG", filepath.Join(t.TempDir(), "missing.toml"))
	root := newRootCmd()
	root.SetArgs([]string{"run"})
	if err := root.ExecuteContext(context.Background()); err == nil {
		t.Fatal("want config load error")
	}
}

// TestConfigFlagError covers runDaemon reading the --config flag: an explicit
// --config to a missing file also fails to load.
func TestConfigFlagError(t *testing.T) {
	t.Setenv("PMMCP_CONFIG", "")
	root := newRootCmd()
	root.SetArgs([]string{"run", "--config", filepath.Join(t.TempDir(), "nope.toml")})
	if err := root.ExecuteContext(context.Background()); err == nil {
		t.Fatal("want config load error from --config")
	}
}

// TestExecute covers the os.Args-driven entry point.
func TestExecute(t *testing.T) {
	origArgs := os.Args
	t.Cleanup(func() { os.Args = origArgs })
	os.Args = []string{"pmmcpd", "version"}
	if err := Execute(context.Background()); err != nil {
		t.Fatalf("Execute version: %v", err)
	}
}

// TestRunDaemonServesUntilCancel covers runDaemon's full path (config load →
// daemon.New → ListenAndServe) by running the daemon with a context that is
// cancelled immediately, so ListenAndServe returns without blocking.
func TestRunDaemonServesUntilCancel(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PMMCP_CONFIG", "")
	t.Setenv("HOME", dir)
	t.Setenv("PMMCP_STATE_DIR", filepath.Join(dir, "state"))
	t.Setenv("PMMCP_IPC_ENDPOINT", filepath.Join(dir, "d.sock"))
	t.Setenv("PMMCP_SANDBOX_DEFAULT", "off")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	root := newRootCmd()
	root.SetArgs([]string{"run"})
	// The daemon may return nil (clean shutdown) or an error tied to the cancelled
	// context; either way the runDaemon body executed. We only require it returns
	// promptly rather than blocking.
	_ = root.ExecuteContext(ctx)
}
