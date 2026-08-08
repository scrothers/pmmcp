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
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestE2EDaemonUnavailableCLI exercises the real CLI when pmmcpd is not running.
func TestE2EDaemonUnavailableCLI(t *testing.T) {
	root := findRoot(t)
	bin := t.TempDir()
	build(t, root, bin, "pmmcp", "./cmd/pmmcp")

	state := t.TempDir()
	sock := filepath.Join(state, "missing.sock")
	cfg := filepath.Join(state, "daemon.toml")
	writeCfg(t, cfg, state, sock)

	cmd := exec.Command(filepath.Join(bin, "pmmcp"), "doctor")
	cmd.Env = append(os.Environ(), "PMMCP_CONFIG="+cfg)
	out, err := cmd.CombinedOutput()
	// doctor returns non-zero when daemon unavailable
	if err == nil {
		t.Fatalf("expected doctor error when daemon down, out=%s", out)
	}
	s := string(out)
	if !strings.Contains(s, "daemon_unavailable") && !strings.Contains(s, "unavailable") && !strings.Contains(s, "remediation") {
		t.Fatalf("expected daemon_unavailable messaging, got: %s", s)
	}
}
