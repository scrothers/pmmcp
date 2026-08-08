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
	"bufio"
	"bytes"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"unicode"
)

// LoadEnvFile parses a simple dotenv file into a KEY→value map.
//
// Supported lines:
//   - KEY=VAL
//   - KEY="quoted val" or KEY='quoted val'
//   - blank lines and # comments (full-line) are ignored
//   - optional export prefix: export KEY=VAL
//
// Values are not expanded (${VAR}). Duplicate keys: last wins. A quoted value
// must open and close on one line; multiline values are not supported.
//
// A file readable by group or other is loaded but logs a warning (env files
// should be mode 0600 per secrets-env-files.md).
func LoadEnvFile(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("secret: load env file: %w", err)
	}
	warnLooseEnvFileMode(path)
	return parseEnvBytes(data)
}

// warnLooseEnvFileMode logs a warning when an env file is group/other-readable.
func warnLooseEnvFileMode(path string) {
	info, err := os.Stat(path)
	if err != nil {
		return
	}
	if info.Mode().Perm()&0o077 != 0 {
		slog.Warn("env file has loose permissions; expected 0600",
			"path", path, "mode", info.Mode().Perm().String())
	}
}

// parseEnvBytes parses dotenv content from bytes (used by LoadEnvFile and SOPS).
func parseEnvBytes(data []byte) (map[string]string, error) {
	out := make(map[string]string)
	sc := bufio.NewScanner(bytes.NewReader(data))
	// Allow long lines (secrets can be large JWTs).
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("secret: load env file: line %d: expected KEY=VAL", lineNo)
		}
		key = strings.TrimSpace(key)
		if key == "" || !isValidEnvKey(key) {
			return nil, fmt.Errorf("secret: load env file: line %d: invalid key %q", lineNo, key)
		}
		val = strings.TrimSpace(val)
		val = unquote(val)
		out[key] = val
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("secret: load env file: %w", err)
	}
	return out, nil
}

func isValidEnvKey(k string) bool {
	if k == "" {
		return false
	}
	for i, r := range k {
		if i == 0 {
			if !unicode.IsLetter(r) && r != '_' {
				return false
			}
			continue
		}
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' {
			return false
		}
	}
	return true
}

func unquote(v string) string {
	if len(v) < 2 {
		return v
	}
	if (v[0] == '"' && v[len(v)-1] == '"') || (v[0] == '\'' && v[len(v)-1] == '\'') {
		return v[1 : len(v)-1]
	}
	return v
}
