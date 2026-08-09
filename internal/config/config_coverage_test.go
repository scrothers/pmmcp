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
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scrothers/pmmcp/internal/config"
)

// TestSanitizePipeUserEmpty covers the "user" fallback when the input
// sanitizes to empty, a case the public API never reaches (every caller
// already substitutes a non-empty default first).
func TestSanitizePipeUserEmpty(t *testing.T) {
	t.Parallel()
	if got := config.SanitizePipeUserForTest(""); got != "user" {
		t.Fatalf("SanitizePipeUserForTest(%q) = %q, want %q", "", got, "user")
	}
}

// TestValidateEmptyFields covers validate's state_dir/ipc.endpoint empty
// checks directly, since Load always fills both via normalizeAndDefault
// before validate ever runs.
func TestValidateEmptyFields(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		cfg  config.Config
	}{
		{"empty_state_dir", config.Config{Version: 1, StateDir: "", IPC: config.IPCConfig{Endpoint: "/tmp/x.sock"}, Sandbox: config.SandboxConfig{Default: config.SandboxStrict}}},
		{"empty_endpoint", config.Config{Version: 1, StateDir: "/tmp/state", IPC: config.IPCConfig{Endpoint: ""}, Sandbox: config.SandboxConfig{Default: config.SandboxStrict}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := tc.cfg
			if err := cfg.ValidateForTest(); !errors.Is(err, config.ErrInvalid) {
				t.Fatalf("ValidateForTest() = %v, want ErrInvalid", err)
			}
		})
	}
}

// TestLoadDefaultsFromOSLookupEnv covers the LoadOptions.lookup fallback to
// os.LookupEnv (no injected LookupEnv) with all overlays cleared, so defaults
// hold and the path-discovery lookups still resolve.
func TestLoadDefaultsFromOSLookupEnv(t *testing.T) {
	clearOverlayEnv(t)
	t.Setenv("XDG_CONFIG_HOME", "")
	home := t.TempDir()
	cfg, err := config.Load(config.LoadOptions{
		GOOS:       "linux",
		Home:       home,
		StateHome:  filepath.Join(home, "state"),
		RuntimeDir: filepath.Join(home, "run"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Sandbox.Default != config.SandboxStrict {
		t.Fatalf("sandbox = %q, want strict (env overlay unexpectedly applied)", cfg.Sandbox.Default)
	}
}

// TestIPCEndpointEnvOverlay covers the PMMCP_IPC_ENDPOINT overlay.
func TestIPCEndpointEnvOverlay(t *testing.T) {
	clearOverlayEnv(t)
	home := t.TempDir()
	want := filepath.Join(home, "custom-from-env.sock")
	t.Setenv("PMMCP_IPC_ENDPOINT", want)
	cfg, err := config.Load(config.LoadOptions{GOOS: "linux", Home: home, LookupEnv: noEnv})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.IPC.Endpoint != want {
		t.Fatalf("endpoint = %q, want %q", cfg.IPC.Endpoint, want)
	}
}

// TestLogLevelEnvOverlay covers the PMMCP_LOG_LEVEL overlay.
func TestLogLevelEnvOverlay(t *testing.T) {
	clearOverlayEnv(t)
	home := t.TempDir()
	t.Setenv("PMMCP_LOG_LEVEL", "warn")
	cfg, err := config.Load(config.LoadOptions{GOOS: "linux", Home: home, LookupEnv: noEnv})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Log.Level != "warn" {
		t.Fatalf("log.level = %q, want warn", cfg.Log.Level)
	}
}

// TestExplicitConfigHomeFromEnv covers explicitConfigHome's XDG_CONFIG_HOME
// lookup branch (as opposed to the opts.ConfigHome override, which
// TestLoadFindsXDGConfig already exercises).
func TestExplicitConfigHomeFromEnv(t *testing.T) {
	clearOverlayEnv(t)
	confHome := t.TempDir()
	writeConfig(t, filepath.Join(confHome, "pmmcp", "daemon.toml"),
		"version = 1\n[sandbox]\ndefault = \"standard\"\n")
	cfg, err := config.Load(config.LoadOptions{
		GOOS: "linux",
		Home: t.TempDir(),
		LookupEnv: func(k string) (string, bool) {
			if k == "XDG_CONFIG_HOME" {
				return confHome, true
			}
			return "", false
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Sandbox.Default != config.SandboxStandard {
		t.Fatalf("sandbox = %q, want standard (XDG_CONFIG_HOME env not read)", cfg.Sandbox.Default)
	}
}

// TestEmptyHomeAndGOOSResolveFromRuntime covers the opts.Home=="" and
// opts.GOOS=="" fallback branches, which every other test bypasses by
// always supplying both explicitly.
func TestEmptyHomeAndGOOSResolveFromRuntime(t *testing.T) {
	clearOverlayEnv(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")

	cfg, err := config.Load(config.LoadOptions{LookupEnv: noEnv})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(cfg.StateDir, home) && !strings.HasSuffix(cfg.StateDir, "pmmcp") {
		t.Fatalf("state_dir = %q, want derived from HOME=%q", cfg.StateDir, home)
	}
}

// TestLoadHomeDirFailure covers Load's and normalizeAndDefault's
// os.UserHomeDir error branches, and (per GOOS) the "no candidates found"
// branches in configCandidates, none of which any other test reaches
// because they all supply opts.Home explicitly.
func TestLoadHomeDirFailure(t *testing.T) {
	for _, goos := range []string{"linux", "windows", "darwin"} {
		t.Run(goos, func(t *testing.T) {
			clearOverlayEnv(t)
			t.Setenv("HOME", "")
			t.Setenv("USERPROFILE", "")
			_, err := config.Load(config.LoadOptions{
				GOOS:      goos,
				LookupEnv: noEnv,
			})
			if err == nil || !strings.Contains(err.Error(), "home") {
				t.Fatalf("err = %v, want it to mention home resolution failure", err)
			}
		})
	}
}

// TestMalformedFilesRejected covers viper decode failures across formats: a
// malformed TOML body, a JSON body in a .toml file, and a type mismatch that
// even weakly-typed decoding cannot coerce.
func TestMalformedFilesRejected(t *testing.T) {
	cases := []struct {
		name string
		ext  string
		body string
	}{
		{"toml_bad_table", "toml", "[[[\n"},
		{"toml_json_body", "toml", `{"version": 1}`},
		{"toml_type_mismatch", "toml", `version = "not-an-int"`},
		{"json_malformed", "json", `{"version":`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clearOverlayEnv(t)
			dir := t.TempDir()
			path := filepath.Join(dir, "bad."+tc.ext)
			writeConfig(t, path, tc.body)
			_, err := config.Load(config.LoadOptions{
				Path: path, GOOS: "linux", Home: dir, LookupEnv: noEnv,
			})
			if !errors.Is(err, config.ErrInvalid) {
				t.Fatalf("err = %v, want ErrInvalid", err)
			}
		})
	}
}
