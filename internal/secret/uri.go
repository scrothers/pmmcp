// Copyright 2026 Steven Crothers
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package secret

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Ref is a parsed secret:// URI.
type Ref struct {
	// Scheme is keyring, sops, file, or env.
	Scheme string
	// Path is the scheme-specific path (keyring account, file path, env var name).
	Path string
	// Key is an optional fragment for SOPS JSON/YAML field selection.
	Key string
	// Raw is the original URI.
	Raw string
}

// ParseRef parses secret:// URIs. Bare names (no scheme) are rejected.
//
// Supported:
//
//	secret://keyring/pmmcp/<name>
//	secret://keyring/<name>
//	secret://sops:<path>#<key>
//	secret://sops/<path>#<key>
//	secret://file:<path>
//	secret://file/<path>
//	secret://env:<VAR>
//	secret://env/<VAR>
func ParseRef(raw string) (Ref, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Ref{}, fmt.Errorf("secret: empty ref")
	}
	if !strings.HasPrefix(raw, "secret://") {
		return Ref{}, fmt.Errorf("secret: not a secret:// ref: %q", raw)
	}
	rest := strings.TrimPrefix(raw, "secret://")
	if rest == "" {
		return Ref{}, fmt.Errorf("secret: empty secret:// ref")
	}

	// Fragment key (#field) for SOPS.
	key := ""
	if i := strings.IndexByte(rest, '#'); i >= 0 {
		key = rest[i+1:]
		rest = rest[:i]
	}

	// Forms: scheme:path or scheme/path
	var scheme, path string
	if i := strings.IndexByte(rest, ':'); i >= 0 && !strings.Contains(rest[:i], "/") {
		scheme = strings.ToLower(rest[:i])
		path = rest[i+1:]
	} else if i := strings.IndexByte(rest, '/'); i >= 0 {
		scheme = strings.ToLower(rest[:i])
		path = rest[i+1:]
	} else {
		return Ref{}, fmt.Errorf("secret: invalid ref %q", raw)
	}
	scheme = strings.TrimSpace(scheme)
	path = strings.TrimSpace(path)
	if scheme == "" || path == "" {
		return Ref{}, fmt.Errorf("secret: invalid ref %q", raw)
	}
	switch scheme {
	case "keyring", "sops", "file", "env":
	default:
		return Ref{}, fmt.Errorf("secret: unknown scheme %q", scheme)
	}
	// Normalize keyring/pmmcp/<name> → name stored under service pmmcp, and
	// reject traversal so a ref like secret://keyring/../../etc/passwd fails at
	// parse time rather than reaching the filesystem.
	if scheme == "keyring" {
		path = strings.TrimPrefix(path, "pmmcp/")
		path = strings.TrimPrefix(path, "/")
		if err := validateKeyringName(path); err != nil {
			return Ref{}, err
		}
	}
	return Ref{Scheme: scheme, Path: path, Key: key, Raw: raw}, nil
}

// LooksLikeRef reports whether s is a secret:// URI.
func LooksLikeRef(s string) bool {
	return strings.HasPrefix(strings.TrimSpace(s), "secret://")
}

// ResolveOptions configures Resolve.
type ResolveOptions struct {
	// ProjectRoot anchors relative sops/file paths.
	ProjectRoot string
	// Keyring is the file keyring backend (required for keyring scheme).
	Keyring *FileBackend
	// AllowFile outside project is denied when false (strict default).
	AllowFileOutsideProject bool
}

// Resolve returns the secret value for ref without logging it.
func Resolve(ref string, opts ResolveOptions) (string, error) {
	r, err := ParseRef(ref)
	if err != nil {
		return "", err
	}
	switch r.Scheme {
	case "env":
		v, ok := os.LookupEnv(r.Path)
		if !ok {
			return "", fmt.Errorf("secret: env %q not set", r.Path)
		}
		return v, nil
	case "file":
		path, err := resolvePath(r.Path, opts)
		if err != nil {
			return "", err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("secret: file: %w", err)
		}
		return strings.TrimRight(string(data), "\r\n"), nil
	case "keyring":
		if opts.Keyring == nil {
			return "", fmt.Errorf("secret: keyring backend not configured")
		}
		v, err := opts.Keyring.Get(r.Path)
		if err != nil {
			return "", err
		}
		return v, nil
	case "sops":
		path, err := resolvePath(r.Path, opts)
		if err != nil {
			return "", err
		}
		plain, err := DecryptFile(path)
		if err != nil {
			return "", err
		}
		if r.Key == "" {
			return strings.TrimRight(string(plain), "\r\n"), nil
		}
		return extractKey(plain, r.Key)
	default:
		return "", fmt.Errorf("secret: unknown scheme %q", r.Scheme)
	}
}

// Check reports whether ref resolves without returning the value.
func Check(ref string, opts ResolveOptions) (ok bool, errMsg string) {
	_, err := Resolve(ref, opts)
	if err != nil {
		return false, err.Error()
	}
	return true, ""
}

// ResolveEnvMap resolves secret:// values in an env map; non-refs pass through.
func ResolveEnvMap(env map[string]string, opts ResolveOptions) (map[string]string, error) {
	if env == nil {
		return map[string]string{}, nil
	}
	out := make(map[string]string, len(env))
	for k, v := range env {
		if LooksLikeRef(v) {
			val, err := Resolve(v, opts)
			if err != nil {
				return nil, fmt.Errorf("secret: resolve %s: %w", k, err)
			}
			out[k] = val
			continue
		}
		out[k] = v
	}
	return out, nil
}

func resolvePath(p string, opts ResolveOptions) (string, error) {
	p = strings.TrimSpace(p)
	if p == "" {
		return "", fmt.Errorf("secret: empty path")
	}
	if !filepath.IsAbs(p) {
		if opts.ProjectRoot == "" {
			return "", fmt.Errorf("secret: relative path %q needs project root", p)
		}
		p = filepath.Join(opts.ProjectRoot, p)
	}
	p = filepath.Clean(p)

	// Fail closed: without a project root, an absolute path is only allowed when
	// the caller explicitly opts out of containment. Otherwise a symlink inside
	// the project could still point outside it, so resolve both the root and the
	// candidate through EvalSymlinks before the containment check.
	if opts.AllowFileOutsideProject {
		return p, nil
	}
	if opts.ProjectRoot == "" {
		return "", fmt.Errorf("secret: path %q needs project root (set AllowFileOutsideProject to override)", p)
	}
	root := filepath.Clean(opts.ProjectRoot)
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	// Resolve the candidate's existing prefix; a not-yet-existing file resolves
	// its parent, so a symlinked parent cannot smuggle the path outside.
	cand := evalExisting(p)
	if cand != root && !strings.HasPrefix(cand, root+string(os.PathSeparator)) {
		return "", fmt.Errorf("secret: path %q outside project root", p)
	}
	return p, nil
}

// evalExisting resolves symlinks for the longest existing prefix of p, then
// re-appends the trailing non-existent segments (cleaned).
func evalExisting(p string) string {
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		return resolved
	}
	dir := filepath.Dir(p)
	if dir == p {
		return p
	}
	return filepath.Join(evalExisting(dir), filepath.Base(p))
}

func extractKey(data []byte, key string) (string, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return string(data), nil
	}
	// Try JSON object first.
	var obj map[string]any
	if err := json.Unmarshal(data, &obj); err == nil {
		if v, ok := obj[key]; ok {
			switch t := v.(type) {
			case string:
				return t, nil
			default:
				b, _ := json.Marshal(t)
				return string(b), nil
			}
		}
		return "", fmt.Errorf("secret: sops key %q not found", key)
	}
	// dotenv-style KEY=VAL
	m, err := parseEnvBytes(data)
	if err == nil {
		if v, ok := m[key]; ok {
			return v, nil
		}
		return "", fmt.Errorf("secret: sops key %q not found", key)
	}
	// YAML simple top-level key: "key: value"
	prefix := key + ":"
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix)), nil
		}
	}
	return "", fmt.Errorf("secret: sops key %q not found in cleartext", key)
}
