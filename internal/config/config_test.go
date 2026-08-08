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
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scrothers/pmmcp/internal/config"
)

func TestLoadDefaultsLinux(t *testing.T) {
	clearOverlayEnv(t)
	home := t.TempDir()
	cfg, err := config.Load(config.LoadOptions{
		GOOS:       "linux",
		Home:       home,
		RuntimeDir: filepath.Join(home, "run"),
		StateHome:  filepath.Join(home, "state"),
		LookupEnv:  noEnv,
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Sandbox.Default != config.SandboxStrict {
		t.Fatalf("sandbox = %q, want strict", cfg.Sandbox.Default)
	}
	if cfg.StateDir == "" {
		t.Fatal("empty state_dir")
	}
	if !strings.HasSuffix(cfg.StateDir, "pmmcp") {
		t.Fatalf("state_dir = %q", cfg.StateDir)
	}
	if !strings.HasSuffix(cfg.IPC.Endpoint, "pmmcpd.sock") {
		t.Fatalf("endpoint = %q", cfg.IPC.Endpoint)
	}
	if cfg.Logs.MaxFileMB != 10 {
		t.Fatalf("max_file_mb = %d, want 10", cfg.Logs.MaxFileMB)
	}
	if cfg.Logs.MaxFiles != 5 {
		t.Fatalf("max_files = %d, want 5", cfg.Logs.MaxFiles)
	}
	if !cfg.Logs.Compress {
		t.Fatal("compress = false, want true (gzip on)")
	}
	if !cfg.Relaunch.Enabled {
		t.Fatal("relaunch = false, want true (default)")
	}
}

func TestLoadDefaultsDarwinWindows(t *testing.T) {
	clearOverlayEnv(t)
	home := t.TempDir()
	darwin, err := config.Load(config.LoadOptions{
		GOOS: "darwin", Home: home, LookupEnv: noEnv,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(darwin.StateDir, "Application Support") {
		t.Fatalf("darwin state = %q", darwin.StateDir)
	}
	win, err := config.Load(config.LoadOptions{
		GOOS: "windows", Home: home, Username: "alice", LookupEnv: noEnv,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(win.IPC.Endpoint, `pmmcpd-alice`) {
		t.Fatalf("windows pipe = %q", win.IPC.Endpoint)
	}
}

func TestLoadTOMLFile(t *testing.T) {
	clearOverlayEnv(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "daemon.toml")
	body := `
version = 1
state_dir = "` + filepath.ToSlash(filepath.Join(dir, "mystate")) + `"
token_file = "` + filepath.ToSlash(filepath.Join(dir, "secret.token")) + `"

[ipc]
endpoint = "` + filepath.ToSlash(filepath.Join(dir, "custom.sock")) + `"

[sandbox]
default = "standard"

[log]
level = "debug"
`
	writeConfig(t, path, body)
	cfg, err := config.Load(config.LoadOptions{
		Path: path, GOOS: "linux", Home: dir, LookupEnv: noEnv,
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Sandbox.Default != config.SandboxStandard {
		t.Fatalf("sandbox = %q", cfg.Sandbox.Default)
	}
	if cfg.Log.Level != "debug" {
		t.Fatalf("level = %q", cfg.Log.Level)
	}
	if !strings.HasSuffix(cfg.StateDir, "mystate") {
		t.Fatalf("state = %q", cfg.StateDir)
	}
	if !strings.HasSuffix(cfg.IPC.Endpoint, "custom.sock") {
		t.Fatalf("endpoint = %q", cfg.IPC.Endpoint)
	}
}

// TestLoadYAMLFile and TestLoadJSONFile cover multi-format decoding (viper
// auto-detects by extension), which replaced the TOML-only loader.
func TestLoadYAMLFile(t *testing.T) {
	clearOverlayEnv(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "daemon.yaml")
	body := "version: 1\n" +
		"sandbox:\n  default: permissive\n" +
		"log:\n  level: warn\n" +
		"webhook:\n  allowlist:\n    - \"*.example.com\"\n"
	writeConfig(t, path, body)
	cfg, err := config.Load(config.LoadOptions{
		Path: path, GOOS: "linux", Home: dir, LookupEnv: noEnv,
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Sandbox.Default != config.SandboxPermissive {
		t.Fatalf("sandbox = %q, want permissive", cfg.Sandbox.Default)
	}
	if cfg.Log.Level != "warn" {
		t.Fatalf("level = %q, want warn", cfg.Log.Level)
	}
	if len(cfg.Webhook.Allowlist) != 1 || cfg.Webhook.Allowlist[0] != "*.example.com" {
		t.Fatalf("allowlist = %v", cfg.Webhook.Allowlist)
	}
}

func TestLoadJSONFile(t *testing.T) {
	clearOverlayEnv(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "daemon.json")
	writeConfig(t, path, `{"version":1,"sandbox":{"default":"off"},"log":{"format":"text"}}`)
	cfg, err := config.Load(config.LoadOptions{
		Path: path, GOOS: "linux", Home: dir, LookupEnv: noEnv,
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Sandbox.Default != config.SandboxOff {
		t.Fatalf("sandbox = %q, want off", cfg.Sandbox.Default)
	}
	if cfg.Log.Format != "text" {
		t.Fatalf("format = %q, want text", cfg.Log.Format)
	}
}

func TestEnvOverlay(t *testing.T) {
	clearOverlayEnv(t)
	home := t.TempDir()
	t.Setenv("PMMCP_SANDBOX_DEFAULT", "permissive")
	t.Setenv("PMMCP_STATE_DIR", filepath.Join(home, "from-env"))
	cfg, err := config.Load(config.LoadOptions{GOOS: "linux", Home: home, LookupEnv: noEnv})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Sandbox.Default != config.SandboxPermissive {
		t.Fatalf("sandbox = %q", cfg.Sandbox.Default)
	}
	if !strings.HasSuffix(cfg.StateDir, "from-env") {
		t.Fatalf("state = %q", cfg.StateDir)
	}
}

// TestEnvOverlayNestedKeys covers the AutomaticEnv keys newly reachable now that
// every config key is env-overridable (int, bool, and slice coercion via viper's
// default decode hooks).
func TestEnvOverlayNestedKeys(t *testing.T) {
	clearOverlayEnv(t)
	home := t.TempDir()
	t.Setenv("PMMCP_LOGS_MAX_FILE_MB", "25")
	t.Setenv("PMMCP_LOGS_COMPRESS", "false")
	t.Setenv("PMMCP_RELAUNCH_ENABLED", "false")
	t.Setenv("PMMCP_WEBHOOK_ALLOWLIST", "a.example.com,b.example.com")
	cfg, err := config.Load(config.LoadOptions{GOOS: "linux", Home: home, LookupEnv: noEnv})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Logs.MaxFileMB != 25 {
		t.Fatalf("max_file_mb = %d, want 25", cfg.Logs.MaxFileMB)
	}
	if cfg.Logs.Compress {
		t.Fatal("compress = true, want false (env override)")
	}
	if cfg.Relaunch.Enabled {
		t.Fatal("relaunch = true, want false (env override)")
	}
	if len(cfg.Webhook.Allowlist) != 2 || cfg.Webhook.Allowlist[1] != "b.example.com" {
		t.Fatalf("allowlist = %v, want two entries", cfg.Webhook.Allowlist)
	}
}

// TestPrecedenceFlagEnvFile proves the flag > env > file > default ordering the
// viper migration introduced.
func TestPrecedenceFlagEnvFile(t *testing.T) {
	clearOverlayEnv(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "daemon.toml")
	writeConfig(t, path, "version = 1\n[log]\nlevel = \"warn\"\n")

	// env beats file when no flag is set.
	t.Setenv("PMMCP_LOG_LEVEL", "error")
	envCfg, err := config.Load(config.LoadOptions{Path: path, GOOS: "linux", Home: dir, LookupEnv: noEnv})
	if err != nil {
		t.Fatal(err)
	}
	if envCfg.Log.Level != "error" {
		t.Fatalf("env precedence: level = %q, want error", envCfg.Log.Level)
	}

	// a changed flag beats env and file.
	fs := newDaemonFlags(t, "--log-level", "debug")
	flagCfg, err := config.Load(config.LoadOptions{Path: path, GOOS: "linux", Home: dir, LookupEnv: noEnv, Flags: fs})
	if err != nil {
		t.Fatal(err)
	}
	if flagCfg.Log.Level != "debug" {
		t.Fatalf("flag precedence: level = %q, want debug", flagCfg.Log.Level)
	}

	// an unchanged flag does not clobber the file value.
	fsUnset := newDaemonFlags(t)
	t.Setenv("PMMCP_LOG_LEVEL", "")
	fileCfg, err := config.Load(config.LoadOptions{Path: path, GOOS: "linux", Home: dir, LookupEnv: noEnv, Flags: fsUnset})
	if err != nil {
		t.Fatal(err)
	}
	if fileCfg.Log.Level != "warn" {
		t.Fatalf("file precedence: level = %q, want warn (unchanged flag must not override)", fileCfg.Log.Level)
	}
}

func TestRedaction(t *testing.T) {
	clearOverlayEnv(t)
	home := t.TempDir()
	secretPath := filepath.Join(home, "super-secret-token-value")
	t.Setenv("PMMCP_TOKEN_FILE", secretPath)
	cfg, err := config.Load(config.LoadOptions{GOOS: "linux", Home: home, LookupEnv: noEnv})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TokenFile != secretPath {
		t.Fatalf("token not loaded: %q", cfg.TokenFile)
	}
	s := cfg.String()
	if strings.Contains(s, "super-secret-token-value") {
		t.Fatalf("String leaked secret path: %s", s)
	}
	if !strings.Contains(s, "[redacted]") {
		t.Fatalf("String missing redaction: %s", s)
	}
	doc := cfg.DoctorView()
	if strings.Contains(doc, "super-secret-token-value") {
		t.Fatalf("DoctorView leaked: %s", doc)
	}
	if !strings.Contains(doc, "[redacted]") {
		t.Fatalf("DoctorView missing redaction: %s", doc)
	}
	if cfg.TokenFile != secretPath {
		t.Fatal("Redacted mutated original")
	}
}

func TestInvalidSandbox(t *testing.T) {
	clearOverlayEnv(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "daemon.toml")
	writeConfig(t, path, "[sandbox]\ndefault = \"nope\"\n")
	_, err := config.Load(config.LoadOptions{Path: path, Home: dir, GOOS: "linux", LookupEnv: noEnv})
	if err == nil || !strings.Contains(err.Error(), "sandbox") {
		t.Fatalf("err = %v, want sandbox validation error", err)
	}
}

// TestLoadMissingExplicitFile covers the read-error path for an explicit path
// that does not exist (DefaultPath-resolved paths always exist).
func TestLoadMissingExplicitFile(t *testing.T) {
	clearOverlayEnv(t)
	dir := t.TempDir()
	_, err := config.Load(config.LoadOptions{
		Path: filepath.Join(dir, "missing.toml"), GOOS: "linux", Home: dir, LookupEnv: noEnv,
	})
	if err == nil || !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("err = %v, want a not-exist read error", err)
	}
}
