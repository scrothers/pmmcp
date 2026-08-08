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

package secret_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scrothers/pmmcp/internal/secret"
)

func TestLoadEnvFileLooseModeStillLoads(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("FOO=bar\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := secret.LoadEnvFile(path)
	if err != nil {
		t.Fatalf("loose-mode env file should still load: %v", err)
	}
	if m["FOO"] != "bar" {
		t.Fatalf("FOO = %q", m["FOO"])
	}
}

func TestResolveErrorPaths(t *testing.T) {
	t.Parallel()
	// env var not set.
	if _, err := secret.Resolve("secret://env:PMMCP_DEFINITELY_UNSET_VAR", secret.ResolveOptions{}); err == nil {
		t.Fatal("unset env var should error")
	}
	// keyring backend not configured.
	if _, err := secret.Resolve("secret://keyring/pmmcp/x", secret.ResolveOptions{}); err == nil {
		t.Fatal("keyring with nil backend should error")
	}
	// keyring name that does not exist.
	kr, err := secret.NewFileBackend(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := secret.Resolve("secret://keyring/pmmcp/missing", secret.ResolveOptions{Keyring: kr}); err == nil {
		t.Fatal("missing keyring entry should error")
	}
}

func TestCheckReportsFailure(t *testing.T) {
	t.Parallel()
	ok, msg := secret.Check("secret://env:PMMCP_DEFINITELY_UNSET_VAR", secret.ResolveOptions{})
	if ok || msg == "" {
		t.Fatalf("Check on unresolved ref: ok=%v msg=%q", ok, msg)
	}
}

func TestParseRefErrors(t *testing.T) {
	t.Parallel()
	bad := []string{
		"",
		"   ",
		"not-a-ref",
		"secret://",
		"secret://bogus/x",
		"secret://file:",
		"secret://file",
	}
	for _, raw := range bad {
		if _, err := secret.ParseRef(raw); err == nil {
			t.Errorf("ParseRef(%q) should error", raw)
		}
	}
}

func TestResolveEnvMapPropagatesError(t *testing.T) {
	t.Parallel()
	_, err := secret.ResolveEnvMap(map[string]string{
		"BAD": "secret://env:PMMCP_DEFINITELY_UNSET_VAR",
	}, secret.ResolveOptions{})
	if err == nil {
		t.Fatal("ResolveEnvMap should propagate a resolve failure")
	}
	if out, err := secret.ResolveEnvMap(nil, secret.ResolveOptions{}); err != nil || len(out) != 0 {
		t.Fatalf("nil env: out=%v err=%v", out, err)
	}
}

func TestLoadEnvFileMaybeSOPSPlainEncNameFails(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// A .enc.yaml suffix routes to the decrypt path; a plain body fails clearly.
	path := filepath.Join(dir, "secrets.enc.yaml")
	if err := os.WriteFile(path, []byte("k: v\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := secret.LoadEnvFileMaybeSOPS(path); err == nil {
		t.Fatal("plain body with .enc.yaml suffix should fail to decrypt")
	}
}

func TestPackageRegisterNamedValue(t *testing.T) {
	t.Parallel()
	const v = "qqx-named-package-secret-7z"
	secret.RegisterNamedValue("MY_TOKEN", v)
	got := secret.RedactLine("emitting " + v + " now")
	if strings.Contains(got, v) {
		t.Fatalf("RegisterNamedValue did not scrub: %q", got)
	}
	if !strings.Contains(got, secret.RedactedFor("MY_TOKEN")) {
		t.Fatalf("expected named placeholder: %q", got)
	}
}
