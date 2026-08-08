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
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/getsops/sops/v3/decrypt"
)

// sopsDecryptFile is decrypt.File indirected through a package-level variable
// solely so tests can substitute a fake decryptor and exercise DecryptFile's
// success path without a real age/PGP key. Production code never reassigns
// it; it always resolves to decrypt.File.
var sopsDecryptFile = decrypt.File

// DecryptFile decrypts a SOPS-encrypted file and returns cleartext bytes.
//
// Uses github.com/getsops/sops/v3 (successor to go.mozilla.org/sops/v3).
// Returns a clear error when the file is not encrypted or keys are missing.
func DecryptFile(path string) ([]byte, error) {
	if path == "" {
		return nil, fmt.Errorf("secret: sops decrypt: empty path")
	}
	cleartext, err := sopsDecryptFile(path, "")
	if err != nil {
		return nil, fmt.Errorf("secret: sops decrypt %s: %w", path, err)
	}
	return cleartext, nil
}

// LooksLikeSOPS reports whether path or its content looks SOPS-encrypted.
// True when the basename ends with .enc.yaml / .enc.yml / .enc.json / .enc.env,
// or the file body carries a top-level SOPS metadata key or an ENC[ marker.
func LooksLikeSOPS(path string) bool {
	if sopsPathHint(path) {
		return true
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return bytesLookLikeSOPS(data)
}

// LoadEnvFileMaybeSOPS loads a dotenv file, decrypting with SOPS when the file
// looks encrypted. Non-SOPS files parse as ordinary dotenv.
func LoadEnvFileMaybeSOPS(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("secret: load env file: %w", err)
	}
	if !bytesLookLikeSOPS(data) && !sopsPathHint(path) {
		warnLooseEnvFileMode(path)
		return parseEnvBytes(data)
	}
	plain, err := DecryptFile(path)
	if err != nil {
		return nil, err
	}
	return parseEnvBytes(plain)
}

// sopsPathHint reports filename conventions that imply SOPS without reading body.
// Only the .enc.* suffixes are honored; a bare "sops" substring in the name
// (e.g. my-sops-notes.env) is too loose and is not treated as a hint.
func sopsPathHint(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	return strings.HasSuffix(base, ".enc.yaml") || strings.HasSuffix(base, ".enc.yml") ||
		strings.HasSuffix(base, ".enc.json") || strings.HasSuffix(base, ".enc.env")
}

// sopsMetaRe matches a top-level SOPS metadata key (YAML `sops:` at line start,
// or a `"sops":` JSON key), anchored so a marker inside a value does not match.
var sopsMetaRe = regexp.MustCompile(`(?m)^\s*("sops"|'sops'|sops)\s*:`)

func bytesLookLikeSOPS(data []byte) bool {
	// ENC[ is a SOPS-specific ciphertext marker anywhere in the body.
	if bytes.Contains(data, []byte("ENC[")) {
		return true
	}
	return sopsMetaRe.Match(data)
}
