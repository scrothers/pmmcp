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

package secret

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestWarnLooseEnvFileModeStatError exercises the os.Stat failure branch,
// which is unreachable via the exported LoadEnvFile path because ReadFile
// already succeeded on the same path just before this call.
func TestWarnLooseEnvFileModeStatError(t *testing.T) {
	t.Parallel()
	// Must not panic; the branch just returns early on a stat failure.
	warnLooseEnvFileMode(filepath.Join(t.TempDir(), "does-not-exist"))
}

// TestParseEnvBytesScannerError forces bufio.Scanner's token-too-long error
// by feeding a single line larger than the configured 1MiB buffer, which is
// not reachable through ordinary dotenv content used elsewhere in the suite.
func TestParseEnvBytesScannerError(t *testing.T) {
	t.Parallel()
	huge := "FOO=" + strings.Repeat("a", 2*1024*1024)
	if _, err := parseEnvBytes([]byte(huge)); err == nil {
		t.Fatal("want scanner error for an oversized line")
	}
}

// TestIsValidEnvKeyEmpty covers isValidEnvKey's own empty-string guard. It is
// unreachable via parseEnvBytes, whose caller already short-circuits on
// key == "" before ever calling isValidEnvKey.
func TestIsValidEnvKeyEmpty(t *testing.T) {
	t.Parallel()
	if isValidEnvKey("") {
		t.Fatal("empty key should be invalid")
	}
}

// TestResolvePathEmptyString covers resolvePath's own empty-path guard. It is
// unreachable via the public Resolve/ParseRef path, since ParseRef trims and
// rejects an empty ref path before resolvePath ever sees it.
func TestResolvePathEmptyString(t *testing.T) {
	t.Parallel()
	if _, err := resolvePath("", ResolveOptions{}); err == nil {
		t.Fatal("empty path should be rejected")
	}
}

// TestRedactAssignAndJSONGuards covers the defensive "not enough submatches"
// branch in redactAssign/redactJSON. In production these are only ever
// invoked by regexp.ReplaceAllStringFunc with a match of their own regexp, so
// the submatch slice is always long enough; calling them directly with
// arbitrary input is the only way to exercise the guard.
func TestRedactAssignAndJSONGuards(t *testing.T) {
	t.Parallel()
	if got := redactAssign("no match here"); got != "no match here" {
		t.Fatalf("redactAssign guard = %q", got)
	}
	if got := redactJSON("no match here"); got != "no match here" {
		t.Fatalf("redactJSON guard = %q", got)
	}
}

// TestEvalExistingRecursesToParent covers evalExisting's recursive branch,
// taken when EvalSymlinks fails on a not-yet-existing path.
func TestEvalExistingRecursesToParent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	missing := filepath.Join(dir, "a", "b", "missing.txt")
	got := evalExisting(missing)
	// evalExisting recurses up to the nearest existing ancestor (dir) and
	// resolves it via EvalSymlinks before rejoining the missing suffix, so
	// the expectation must resolve dir the same way (e.g. macOS's
	// /var -> /private/var means dir itself is a symlink target).
	resolvedDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		resolvedDir = dir
	}
	want := filepath.Join(resolvedDir, "a", "b", "missing.txt")
	if got != want {
		t.Fatalf("evalExisting(%q) = %q, want %q", missing, got, want)
	}
}

// TestResolveSOPSViaFakeDecrypt substitutes sopsDecryptFile so the sops
// scheme's success paths (with and without a #key fragment) can be exercised
// without a real age/PGP key. See FABLE_REVIEW.md for why the genuine
// go.mozilla.org/sops decrypt call itself is not exercised.
func TestResolveSOPSViaFakeDecrypt(t *testing.T) {
	orig := sopsDecryptFile
	t.Cleanup(func() { sopsDecryptFile = orig })

	sopsDecryptFile = func(_, _ string) ([]byte, error) {
		return []byte("db: test-secret-do-not-use\n"), nil
	}

	// resolvePath requires filepath.IsAbs, which needs a drive letter on
	// windows — a unix-style "/fake/..." literal is not absolute there.
	fakePath := "/fake/whatever.enc.yaml"
	if runtime.GOOS == "windows" {
		fakePath = `C:\fake\whatever.enc.yaml`
	}

	v, err := Resolve("secret://sops:"+fakePath, ResolveOptions{AllowFileOutsideProject: true})
	if err != nil || v != "db: test-secret-do-not-use" {
		t.Fatalf("sops no-key resolve: %q %v", v, err)
	}

	v, err = Resolve("secret://sops:"+fakePath+"#db", ResolveOptions{AllowFileOutsideProject: true})
	if err != nil || v != "test-secret-do-not-use" {
		t.Fatalf("sops keyed resolve: %q %v", v, err)
	}
}

// TestDecryptFileViaFakeDecryptSucceeds covers DecryptFile's success return
// (cleartext, nil), which real SOPS content cannot reach in this suite
// without a committed age/PGP key.
func TestDecryptFileViaFakeDecryptSucceeds(t *testing.T) {
	orig := sopsDecryptFile
	t.Cleanup(func() { sopsDecryptFile = orig })

	sopsDecryptFile = func(_, _ string) ([]byte, error) {
		return []byte("FOO=test-secret-do-not-use\n"), nil
	}

	got, err := DecryptFile("fake.enc.env")
	if err != nil {
		t.Fatalf("DecryptFile: %v", err)
	}
	if string(got) != "FOO=test-secret-do-not-use\n" {
		t.Fatalf("DecryptFile = %q", got)
	}
}

// TestLoadEnvFileMaybeSOPSFakeDecryptSucceeds covers
// LoadEnvFileMaybeSOPS's post-decrypt success return, unreachable in this
// suite without a real age/PGP key and a genuine SOPS payload.
func TestLoadEnvFileMaybeSOPSFakeDecryptSucceeds(t *testing.T) {
	orig := sopsDecryptFile
	t.Cleanup(func() { sopsDecryptFile = orig })

	sopsDecryptFile = func(_, _ string) ([]byte, error) {
		return []byte("FOO=test-secret-do-not-use\n"), nil
	}

	path := filepath.Join(t.TempDir(), "secrets.enc.yaml")
	// Content is irrelevant: sopsDecryptFile is faked above, and the .enc.yaml
	// suffix alone routes LoadEnvFileMaybeSOPS to the decrypt path.
	if err := os.WriteFile(path, []byte("placeholder\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	m, err := LoadEnvFileMaybeSOPS(path)
	if err != nil {
		t.Fatalf("LoadEnvFileMaybeSOPS: %v", err)
	}
	if m["FOO"] != "test-secret-do-not-use" {
		t.Fatalf("m = %+v", m)
	}
}

// TestExtractKeyNotFoundInPlainCleartext covers extractKey's final
// "not found in cleartext" fallback: content that is neither valid JSON nor
// valid dotenv, and has no matching "key: value" line either.
func TestExtractKeyNotFoundInPlainCleartext(t *testing.T) {
	t.Parallel()
	_, err := extractKey([]byte("plain text\nno assignment here\n"), "missing")
	if err == nil {
		t.Fatal("want error when key is not found in any form")
	}
	want := fmt.Sprintf("secret: sops key %q not found in cleartext", "missing")
	if err.Error() != want {
		t.Fatalf("err = %q, want %q", err.Error(), want)
	}
}
