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

func TestLoadEnvFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	content := `# comment
FOO=bar
export BAZ="qux quux"
EMPTY=
API_TOKEN=supersecret
DB_PASSWORD='p@ss'
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	m, err := secret.LoadEnvFile(path)
	if err != nil {
		t.Fatalf("LoadEnvFile: %v", err)
	}
	if m["FOO"] != "bar" {
		t.Fatalf("FOO = %q", m["FOO"])
	}
	if m["BAZ"] != "qux quux" {
		t.Fatalf("BAZ = %q", m["BAZ"])
	}
	if m["EMPTY"] != "" {
		t.Fatalf("EMPTY = %q", m["EMPTY"])
	}
	if m["API_TOKEN"] != "supersecret" {
		t.Fatalf("API_TOKEN = %q", m["API_TOKEN"])
	}
	if m["DB_PASSWORD"] != "p@ss" {
		t.Fatalf("DB_PASSWORD = %q", m["DB_PASSWORD"])
	}
}

func TestLoadEnvFileInvalidKeys(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		content string
	}{
		{"starts with digit", "1FOO=bar\n"},
		{"contains hyphen", "FOO-BAR=bar\n"},
		{"empty key", "=bar\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			path := filepath.Join(dir, ".env")
			if err := os.WriteFile(path, []byte(tc.content), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := secret.LoadEnvFile(path); err == nil {
				t.Fatalf("content %q: want invalid-key error", tc.content)
			}
		})
	}
}

func TestLoadEnvFileMissing(t *testing.T) {
	t.Parallel()
	_, err := secret.LoadEnvFile(filepath.Join(t.TempDir(), "nope.env"))
	if err == nil {
		t.Fatal("want error for missing file")
	}
}

func TestLoadEnvFileMaybeSOPS_PlainFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("FOO=bar\nBAZ=qux\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	m, err := secret.LoadEnvFileMaybeSOPS(path)
	if err != nil {
		t.Fatalf("LoadEnvFileMaybeSOPS plain: %v", err)
	}
	if m["FOO"] != "bar" || m["BAZ"] != "qux" {
		t.Fatalf("map = %+v", m)
	}
}

func TestDecryptFile_PlainFileError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "plain.env")
	if err := os.WriteFile(path, []byte("FOO=bar\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Must return an error (not panic) for non-SOPS content.
	_, err := secret.DecryptFile(path)
	if err == nil {
		t.Fatal("DecryptFile on plain file: want error")
	}
	if !strings.Contains(err.Error(), "sops") && !strings.Contains(err.Error(), "SOPS") &&
		!strings.Contains(err.Error(), "decrypt") {
		// Accept any clear error; just ensure message is non-empty.
		if err.Error() == "" {
			t.Fatal("empty error")
		}
	}
}

func TestDecryptFileEmptyPath(t *testing.T) {
	t.Parallel()
	if _, err := secret.DecryptFile(""); err == nil {
		t.Fatal("empty path should error")
	}
}

func TestLooksLikeSOPSMissingFile(t *testing.T) {
	t.Parallel()
	// No .enc.* suffix hint, and the file does not exist: the ReadFile
	// failure inside LooksLikeSOPS must report "not SOPS" rather than panic.
	if secret.LooksLikeSOPS(filepath.Join(t.TempDir(), "nope.env")) {
		t.Fatal("missing file should not look like sops")
	}
}

func TestLoadEnvFileMaybeSOPSMissingFile(t *testing.T) {
	t.Parallel()
	if _, err := secret.LoadEnvFileMaybeSOPS(filepath.Join(t.TempDir(), "nope.env")); err == nil {
		t.Fatal("missing file should error")
	}
}

func TestLooksLikeSOPS(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	plain := filepath.Join(dir, "plain.env")
	if err := os.WriteFile(plain, []byte("A=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if secret.LooksLikeSOPS(plain) {
		t.Fatal("plain file should not look like sops")
	}
	encName := filepath.Join(dir, "secrets.enc.yaml")
	if err := os.WriteFile(encName, []byte("x: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !secret.LooksLikeSOPS(encName) {
		t.Fatal(".enc.yaml should look like sops")
	}
	marked := filepath.Join(dir, "marked.env")
	if err := os.WriteFile(marked, []byte("FOO=ENC[AES256_GCM,data:xx]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !secret.LooksLikeSOPS(marked) {
		t.Fatal("ENC[ content should look like sops")
	}
}

func TestRedactMap(t *testing.T) {
	t.Parallel()
	in := map[string]string{
		"HOME":            "/home/u",
		"API_TOKEN":       "abc",
		"my_secret_key":   "x",
		"db_Password":     "y",
		"SERVICE_API_KEY": "z",
		"PLAIN":           "ok",
	}
	out := secret.RedactMap(in)
	if out["HOME"] != "/home/u" || out["PLAIN"] != "ok" {
		t.Fatalf("non-sensitive changed: %+v", out)
	}
	for _, k := range []string{"API_TOKEN", "my_secret_key", "db_Password", "SERVICE_API_KEY"} {
		if out[k] != secret.RedactedFor(k) {
			t.Fatalf("%s = %q, want %s", k, out[k], secret.RedactedFor(k))
		}
	}
	// Input not mutated.
	if in["API_TOKEN"] != "abc" {
		t.Fatal("input map mutated")
	}
	if secret.RedactMap(nil) != nil {
		t.Fatal("nil map should stay nil")
	}
}

func TestRedactLine(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in, want string
	}{
		{"FOO=bar", "FOO=bar"},
		{"API_TOKEN=abc", "API_TOKEN=" + secret.RedactedFor("API_TOKEN")},
		{"export DB_PASSWORD=secret", "export DB_PASSWORD=" + secret.RedactedFor("DB_PASSWORD")},
		{"not an assignment", "not an assignment"},
		{"", ""},
		// Mid-line KEY=value is redacted, not just full-line dotenv shapes.
		{"retrying with API_TOKEN=abc123 now", "retrying with API_TOKEN=" + secret.RedactedFor("API_TOKEN") + " now"},
		// Embedded token in the key name.
		{"SERVICE_API_KEY=zzz", "SERVICE_API_KEY=" + secret.RedactedFor("SERVICE_API_KEY")},
		// JSON-shaped sensitive pair.
		{`{"api_token":"abc"}`, `{"api_token":"` + secret.RedactedFor("api_token") + `"}`},
	}
	for _, tc := range cases {
		if got := secret.RedactLine(tc.in); got != tc.want {
			t.Fatalf("RedactLine(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
