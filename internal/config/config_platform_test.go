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

package config_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/scrothers/pmmcp/internal/config"
)

func TestWindowsStateAndPipe(t *testing.T) {
	clearOverlayEnv(t)
	home := t.TempDir()
	// LOCALAPPDATA present drives the state dir; USERNAME with invalid chars is sanitized.
	cfg, err := config.Load(config.LoadOptions{
		GOOS: "windows", Home: home,
		LookupEnv: func(k string) (string, bool) {
			switch k {
			case "LOCALAPPDATA":
				return filepath.Join(home, "AppData", "Local"), true
			case "USERNAME":
				return `dom\ain user`, true
			default:
				return "", false
			}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(cfg.StateDir, "Local") {
		t.Fatalf("state = %q", cfg.StateDir)
	}
	if !strings.Contains(cfg.IPC.Endpoint, `pmmcpd-dom_ain_user`) {
		t.Fatalf("pipe = %q (user not sanitized)", cfg.IPC.Endpoint)
	}
}

func TestWindowsFallbacksNoEnv(t *testing.T) {
	clearOverlayEnv(t)
	home := t.TempDir()
	// No LOCALAPPDATA/USERNAME/USER: state falls back under home, pipe user → "user".
	cfg, err := config.Load(config.LoadOptions{
		GOOS: "windows", Home: home, LookupEnv: noEnv,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(cfg.StateDir, filepath.Join("AppData", "Local")) {
		t.Fatalf("state = %q", cfg.StateDir)
	}
	if !strings.Contains(cfg.IPC.Endpoint, "pmmcpd-user") {
		t.Fatalf("pipe = %q", cfg.IPC.Endpoint)
	}
}

func TestWindowsConfigCandidateAPPDATA(t *testing.T) {
	clearOverlayEnv(t)
	home := t.TempDir()
	appData := t.TempDir()
	writeConfig(t, filepath.Join(appData, "pmmcp", "daemon.toml"), "version = 1\n[log]\nlevel = \"warn\"\n")
	cfg, err := config.Load(config.LoadOptions{
		GOOS: "windows", Home: home,
		LookupEnv: func(k string) (string, bool) {
			if k == "APPDATA" {
				return appData, true
			}
			return "", false
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Log.Level != "warn" {
		t.Fatalf("level = %q, want warn (APPDATA config not found)", cfg.Log.Level)
	}
}

func TestLinuxXDGEnvFallbacks(t *testing.T) {
	clearOverlayEnv(t)
	home := t.TempDir()
	stateHome := t.TempDir()
	runtimeDir := t.TempDir()
	cfg, err := config.Load(config.LoadOptions{
		GOOS: "linux", Home: home,
		LookupEnv: func(k string) (string, bool) {
			switch k {
			case "XDG_STATE_HOME":
				return stateHome, true
			case "XDG_RUNTIME_DIR":
				return runtimeDir, true
			default:
				return "", false
			}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(cfg.StateDir, stateHome) {
		t.Fatalf("state = %q, want under %q", cfg.StateDir, stateHome)
	}
	// XDG_RUNTIME_DIR gets pmmcp nested under it.
	if !strings.HasPrefix(cfg.IPC.Endpoint, filepath.Join(runtimeDir, "pmmcp")) {
		t.Fatalf("endpoint = %q, want under %q", cfg.IPC.Endpoint, filepath.Join(runtimeDir, "pmmcp"))
	}
}

func TestLinuxNoXDGRuntimeDirNestsUnderState(t *testing.T) {
	clearOverlayEnv(t)
	home := t.TempDir()
	cfg, err := config.Load(config.LoadOptions{
		GOOS: "linux", Home: home, LookupEnv: noEnv,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(cfg.IPC.Endpoint, filepath.Join("pmmcp", "runtime")) {
		t.Fatalf("endpoint = %q, want state/runtime fallback", cfg.IPC.Endpoint)
	}
}

func TestRuntimeDirAlreadyPmmcpNotNested(t *testing.T) {
	clearOverlayEnv(t)
	home := t.TempDir()
	rd := filepath.Join(t.TempDir(), "pmmcp")
	cfg, err := config.Load(config.LoadOptions{
		GOOS: "linux", Home: home, RuntimeDir: rd, LookupEnv: noEnv,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(rd, "pmmcpd.sock")
	if cfg.IPC.Endpoint != want {
		t.Fatalf("endpoint = %q, want %q (should not double-nest pmmcp)", cfg.IPC.Endpoint, want)
	}
}

func TestNormalizeReDefaultsZeroValues(t *testing.T) {
	clearOverlayEnv(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "daemon.toml")
	// Explicit zero/empty values must be re-defaulted, not left broken.
	writeConfig(t, path, strings.Join([]string{
		"version = 0",
		"state_dir = \"\"",
		"[sandbox]",
		"default = \"\"",
		"[log]",
		"level = \"\"",
		"format = \"\"",
		"[logs]",
		"max_file_mb = 0",
		"max_files = 0",
	}, "\n")+"\n")
	cfg, err := config.Load(config.LoadOptions{
		Path: path, GOOS: "linux", Home: dir, LookupEnv: noEnv,
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Version != 1 || cfg.Sandbox.Default != config.SandboxStrict ||
		cfg.Log.Level != "info" || cfg.Log.Format != "json" ||
		cfg.Logs.MaxFileMB != 10 || cfg.Logs.MaxFiles != 5 || cfg.StateDir == "" {
		t.Fatalf("re-defaults not applied: %+v", cfg)
	}
}

func TestNilReceiverViews(t *testing.T) {
	t.Parallel()
	var c *config.Config
	if got := c.Redacted(); got.Version != 0 {
		t.Fatalf("nil Redacted = %+v", got)
	}
	if s := c.String(); !strings.Contains(s, "config{") {
		t.Fatalf("nil String = %q", s)
	}
	if d := c.DoctorView(); !strings.Contains(d, "version = 0") {
		t.Fatalf("nil DoctorView = %q", d)
	}
}
