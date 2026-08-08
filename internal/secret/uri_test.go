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
	"testing"

	"github.com/scrothers/pmmcp/internal/secret"
)

func TestParseRef(t *testing.T) {
	t.Parallel()
	cases := []struct {
		raw, scheme, path, key string
	}{
		{"secret://keyring/pmmcp/tok", "keyring", "tok", ""},
		{"secret://keyring/tok", "keyring", "tok", ""},
		{"secret://sops:secrets.enc.yaml#db", "sops", "secrets.enc.yaml", "db"},
		{"secret://sops/secrets.enc.yaml#db", "sops", "secrets.enc.yaml", "db"},
		{"secret://file:/tmp/x", "file", "/tmp/x", ""},
		{"secret://env:HOME", "env", "HOME", ""},
	}
	for _, tc := range cases {
		r, err := secret.ParseRef(tc.raw)
		if err != nil {
			t.Fatalf("%s: %v", tc.raw, err)
		}
		if r.Scheme != tc.scheme || r.Path != tc.path || r.Key != tc.key {
			t.Fatalf("%s: got scheme=%q path=%q key=%q", tc.raw, r.Scheme, r.Path, r.Key)
		}
	}
	if _, err := secret.ParseRef("not-a-ref"); err == nil {
		t.Fatal("expected error")
	}
}

func TestResolveEnvAndFile(t *testing.T) {
	t.Setenv("PMMCP_TEST_SECRET", "from-env")
	v, err := secret.Resolve("secret://env:PMMCP_TEST_SECRET", secret.ResolveOptions{})
	if err != nil || v != "from-env" {
		t.Fatalf("env resolve: %q %v", v, err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "secret.txt")
	if err := os.WriteFile(path, []byte("file-val\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	v, err = secret.Resolve("secret://file:"+path, secret.ResolveOptions{AllowFileOutsideProject: true})
	if err != nil || v != "file-val" {
		t.Fatalf("file resolve: %q %v", v, err)
	}
	// Outside project denied by default.
	_, err = secret.Resolve("secret://file:"+path, secret.ResolveOptions{ProjectRoot: t.TempDir()})
	if err == nil {
		t.Fatal("expected outside project deny")
	}
}

func TestResolveKeyring(t *testing.T) {
	t.Parallel()
	kr, err := secret.NewFileBackend(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := kr.Set("dbpass", "s3cret"); err != nil {
		t.Fatal(err)
	}
	v, err := secret.Resolve("secret://keyring/pmmcp/dbpass", secret.ResolveOptions{Keyring: kr})
	if err != nil || v != "s3cret" {
		t.Fatalf("got %q %v", v, err)
	}
	ok, msg := secret.Check("secret://keyring/dbpass", secret.ResolveOptions{Keyring: kr})
	if !ok || msg != "" {
		t.Fatalf("check: ok=%v msg=%s", ok, msg)
	}
}

func TestResolveEnvMap(t *testing.T) {
	t.Setenv("PMMCP_MAP_SECRET", "mapped")
	out, err := secret.ResolveEnvMap(map[string]string{
		"A": "plain",
		"B": "secret://env:PMMCP_MAP_SECRET",
	}, secret.ResolveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if out["A"] != "plain" || out["B"] != "mapped" {
		t.Fatalf("%v", out)
	}
}

func TestParseRefRejectsKeyringTraversal(t *testing.T) {
	t.Parallel()
	bad := []string{
		"secret://keyring/../../etc/passwd",
		"secret://keyring/pmmcp/../escape",
		"secret://keyring/a/b",
	}
	for _, raw := range bad {
		if _, err := secret.ParseRef(raw); err == nil {
			t.Errorf("ParseRef(%q) should reject traversal", raw)
		}
	}
	// A normal name still parses.
	if _, err := secret.ParseRef("secret://keyring/pmmcp/ok_name"); err != nil {
		t.Fatalf("valid keyring ref rejected: %v", err)
	}
}

func TestResolveFileNoRootDeniedByDefault(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "s.txt")
	if err := os.WriteFile(path, []byte("v\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// No ProjectRoot and AllowFileOutsideProject false → fail closed.
	if _, err := secret.Resolve("secret://file:"+path, secret.ResolveOptions{}); err == nil {
		t.Fatal("absolute file with no project root should be denied by default")
	}
	// Explicit opt-out allows it.
	if _, err := secret.Resolve("secret://file:"+path, secret.ResolveOptions{AllowFileOutsideProject: true}); err != nil {
		t.Fatalf("AllowFileOutsideProject should permit: %v", err)
	}
}

func TestResolveInvalidRefPropagatesParseError(t *testing.T) {
	t.Parallel()
	if _, err := secret.Resolve("not-a-secret-ref", secret.ResolveOptions{}); err == nil {
		t.Fatal("Resolve should propagate a ParseRef error")
	}
}

func TestResolveFileMissingFileErrors(t *testing.T) {
	t.Parallel()
	missing := filepath.Join(t.TempDir(), "nope.txt")
	if _, err := secret.Resolve("secret://file:"+missing, secret.ResolveOptions{AllowFileOutsideProject: true}); err == nil {
		t.Fatal("resolving a missing file should error")
	}
}

func TestResolveFileRelativePathNeedsProjectRoot(t *testing.T) {
	t.Parallel()
	// A relative path with no ProjectRoot is rejected before any containment
	// check, distinct from the absolute-path-needs-root case.
	if _, err := secret.Resolve("secret://file:relative/sub.txt", secret.ResolveOptions{}); err == nil {
		t.Fatal("relative file path with no project root should be denied")
	}
}

func TestResolveFileRelativePathUnderProjectRoot(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "sub.txt"), []byte("val\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	v, err := secret.Resolve("secret://file:sub.txt", secret.ResolveOptions{ProjectRoot: root})
	if err != nil || v != "val" {
		t.Fatalf("relative in-project file: %q %v", v, err)
	}
}

func TestResolveSOPSRelativePathNeedsProjectRoot(t *testing.T) {
	t.Parallel()
	if _, err := secret.Resolve("secret://sops:relative.enc.yaml", secret.ResolveOptions{}); err == nil {
		t.Fatal("relative sops path with no project root should be denied")
	}
}

func TestResolveSOPSDecryptFailureOnPlainFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "plain.enc.yaml")
	if err := os.WriteFile(path, []byte("k: v\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := secret.Resolve("secret://sops:"+path, secret.ResolveOptions{AllowFileOutsideProject: true}); err == nil {
		t.Fatal("decrypting a non-SOPS file should error")
	}
}

func TestResolveFileSymlinkEscapeDenied(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	root := filepath.Join(base, "project")
	outside := filepath.Join(base, "outside")
	for _, d := range []string{root, outside} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	target := filepath.Join(outside, "id_secret")
	if err := os.WriteFile(target, []byte("PRIVATE\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// A symlink inside the project pointing outside must not escape containment.
	link := filepath.Join(root, "leak")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	_, err := secret.Resolve("secret://file:"+link, secret.ResolveOptions{ProjectRoot: root})
	if err == nil {
		t.Fatal("symlink escaping project root should be denied")
	}
	// A real file inside the project resolves.
	inside := filepath.Join(root, "ok.txt")
	if err := os.WriteFile(inside, []byte("val\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	v, err := secret.Resolve("secret://file:"+inside, secret.ResolveOptions{ProjectRoot: root})
	if err != nil || v != "val" {
		t.Fatalf("in-project file: %q %v", v, err)
	}
}
