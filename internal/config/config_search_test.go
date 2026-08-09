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

func TestLoadFindsXDGConfig(t *testing.T) {
	clearOverlayEnv(t)
	home := t.TempDir()
	confHome := t.TempDir()
	writeConfig(t, filepath.Join(confHome, "pmmcp", "daemon.toml"),
		"version = 1\n[sandbox]\ndefault = \"standard\"\n")
	cfg, err := config.Load(config.LoadOptions{
		GOOS:       "linux",
		Home:       home,
		ConfigHome: confHome,
		LookupEnv:  noEnv,
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Sandbox.Default != config.SandboxStandard {
		t.Fatalf("sandbox = %q, want standard (XDG config not read)", cfg.Sandbox.Default)
	}
}

func TestLoadFindsPMMCPConfigEnv(t *testing.T) {
	clearOverlayEnv(t)
	home := t.TempDir()
	path := filepath.Join(home, "explicit.toml")
	writeConfig(t, path, "version = 1\n[log]\nlevel = \"debug\"\n")
	cfg, err := config.Load(config.LoadOptions{
		GOOS: "linux",
		Home: home,
		LookupEnv: func(k string) (string, bool) {
			if k == "PMMCP_CONFIG" {
				return path, true
			}
			return "", false
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Log.Level != "debug" {
		t.Fatalf("level = %q, want debug (PMMCP_CONFIG not read)", cfg.Log.Level)
	}
}

func TestDefaultPathPrefersXDGOverPlatform(t *testing.T) {
	t.Parallel()
	confHome := t.TempDir()
	want := filepath.Join(confHome, "pmmcp", "daemon.toml")
	writeConfig(t, want, "version = 1\n")
	got, ok := config.DefaultPath(config.LoadOptions{
		GOOS:       "darwin",
		Home:       t.TempDir(),
		ConfigHome: confHome,
		LookupEnv:  noEnv,
	})
	if !ok || got != want {
		t.Fatalf("DefaultPath = %q,%v want %q,true", got, ok, want)
	}
}

// TestDefaultPathMultiFormat covers extension precedence: with several supported
// extensions present, .toml wins, and with only .yaml present it is found.
func TestDefaultPathMultiFormat(t *testing.T) {
	t.Parallel()
	confHome := t.TempDir()
	yamlOnly := t.TempDir()
	writeConfig(t, filepath.Join(confHome, "pmmcp", "daemon.yaml"), "version: 1\n")
	writeConfig(t, filepath.Join(confHome, "pmmcp", "daemon.toml"), "version = 1\n")
	got, ok := config.DefaultPath(config.LoadOptions{GOOS: "linux", Home: t.TempDir(), ConfigHome: confHome, LookupEnv: noEnv})
	if !ok || filepath.Ext(got) != ".toml" {
		t.Fatalf("DefaultPath = %q,%v want a .toml (extension precedence)", got, ok)
	}
	writeConfig(t, filepath.Join(yamlOnly, "pmmcp", "daemon.yaml"), "version: 1\n")
	got, ok = config.DefaultPath(config.LoadOptions{GOOS: "linux", Home: t.TempDir(), ConfigHome: yamlOnly, LookupEnv: noEnv})
	if !ok || filepath.Ext(got) != ".yaml" {
		t.Fatalf("DefaultPath = %q,%v want the .yaml", got, ok)
	}
}

func TestDefaultPathNoneFound(t *testing.T) {
	t.Parallel()
	if got, ok := config.DefaultPath(config.LoadOptions{
		GOOS:      "linux",
		Home:      t.TempDir(),
		LookupEnv: noEnv,
	}); ok {
		t.Fatalf("DefaultPath = %q,true want empty,false", got)
	}
}

func TestUnknownKeysRejected(t *testing.T) {
	clearOverlayEnv(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "daemon.toml")
	writeConfig(t, path, "version = 1\n[sandbox]\nunknown_key = \"off\"\n")
	_, err := config.Load(config.LoadOptions{
		Path: path, GOOS: "linux", Home: dir, LookupEnv: noEnv,
	})
	if !errors.Is(err, config.ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid", err)
	}
	if !strings.Contains(err.Error(), "unknown_key") {
		t.Fatalf("err should name the unknown key: %v", err)
	}
}

func TestIPCTokenFileCanonicalLocation(t *testing.T) {
	clearOverlayEnv(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "daemon.toml")
	secret := filepath.Join(dir, "ipc.token")
	writeConfig(t, path, "version = 1\n[ipc]\ntoken_file = \""+filepath.ToSlash(secret)+"\"\n")
	cfg, err := config.Load(config.LoadOptions{
		Path: path, GOOS: "linux", Home: dir, LookupEnv: noEnv,
	})
	if err != nil {
		t.Fatal(err)
	}
	// The TOML value is written with forward slashes (writeConfig below uses
	// filepath.ToSlash so the literal round-trips through TOML string syntax
	// without escaping); config.Load folds it in verbatim, so compare after
	// converting back to native separators rather than assuming the loader
	// normalizes the path itself.
	if filepath.Clean(filepath.FromSlash(cfg.TokenFile)) != secret {
		t.Fatalf("TokenFile = %q, want %q ([ipc].token_file folded in)", cfg.TokenFile, secret)
	}
	if strings.Contains(cfg.String(), secret) || strings.Contains(cfg.DoctorView(), secret) {
		t.Fatal("secret token path leaked in a view")
	}
}

// TestTokenFileEnvBeatsCanonical covers the fold precedence: an explicit
// PMMCP_TOKEN_FILE wins over a file's [ipc].token_file.
func TestTokenFileEnvBeatsCanonical(t *testing.T) {
	clearOverlayEnv(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "daemon.toml")
	canonical := filepath.Join(dir, "canonical.token")
	fromEnv := filepath.Join(dir, "from-env.token")
	writeConfig(t, path, "version = 1\n[ipc]\ntoken_file = \""+filepath.ToSlash(canonical)+"\"\n")
	t.Setenv("PMMCP_TOKEN_FILE", fromEnv)
	cfg, err := config.Load(config.LoadOptions{Path: path, GOOS: "linux", Home: dir, LookupEnv: noEnv})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TokenFile != fromEnv {
		t.Fatalf("TokenFile = %q, want %q (env must beat [ipc].token_file)", cfg.TokenFile, fromEnv)
	}
}

func TestWebhookAllowlist(t *testing.T) {
	clearOverlayEnv(t)
	dir := t.TempDir()
	def, err := config.Load(config.LoadOptions{GOOS: "linux", Home: dir, LookupEnv: noEnv})
	if err != nil {
		t.Fatal(err)
	}
	if len(def.Webhook.Allowlist) != 0 {
		t.Fatalf("default allowlist = %v, want empty", def.Webhook.Allowlist)
	}
	path := filepath.Join(dir, "daemon.toml")
	writeConfig(t, path, "version = 1\n[webhook]\nallowlist = [\"*.example.com\", \"https://hooks.slack.com/\"]\n")
	cfg, err := config.Load(config.LoadOptions{Path: path, GOOS: "linux", Home: dir, LookupEnv: noEnv})
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Webhook.Allowlist) != 2 || cfg.Webhook.Allowlist[0] != "*.example.com" {
		t.Fatalf("allowlist = %v", cfg.Webhook.Allowlist)
	}
}

func TestUnsupportedVersionRejected(t *testing.T) {
	clearOverlayEnv(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "daemon.toml")
	writeConfig(t, path, "version = 2\n")
	_, err := config.Load(config.LoadOptions{
		Path: path, GOOS: "linux", Home: dir, LookupEnv: noEnv,
	})
	if !errors.Is(err, config.ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid", err)
	}
}

func TestLogFormatEnvOverlay(t *testing.T) {
	clearOverlayEnv(t)
	home := t.TempDir()
	t.Setenv("PMMCP_LOG_FORMAT", "text")
	cfg, err := config.Load(config.LoadOptions{GOOS: "linux", Home: home, LookupEnv: noEnv})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Log.Format != "text" {
		t.Fatalf("log.format = %q, want text", cfg.Log.Format)
	}
}

func TestLogCompressExplicitFalse(t *testing.T) {
	clearOverlayEnv(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "daemon.toml")
	writeConfig(t, path, "version = 1\n[logs]\ncompress = false\n")
	cfg, err := config.Load(config.LoadOptions{
		Path: path, GOOS: "linux", Home: dir, LookupEnv: noEnv,
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Logs.Compress {
		t.Fatal("compress = true, want false (explicit override ignored)")
	}
}
