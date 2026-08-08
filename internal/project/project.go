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

package project

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// configMarkers name a pmmcp declare file; a VCS marker.
//
// Detection is ordered: a pmmcp.yaml/pmmcp.yml anywhere up the tree outranks
// any.git, per scope-project.md's two-pass detection.
var (
	configMarkers = []string{"pmmcp.yaml", "pmmcp.yml"}
	vcsMarkers    = []string{".git"}
)

// Detect finds the project root starting from cwd. The full precedence, per
// scope-project.md, is: explicit --project flag, then PMMCP_PROJECT, then a
// pmmcp.yaml/pmmcp.yml parent walk, then a.git parent walk, then global scope.
// The flag and env steps are the caller's responsibility (this package takes an
// explicit cwd and reads no environment); Detect implements the two config/VCS
// walks. It returns the absolute root path and a stable, symlink-resolved key.
// When no marker is found, cwd itself is used as the root (the caller applies
// any global-scope or project.required policy).
//
// Symlink escape policy: identity is canonicalized through EvalSymlinks (see
// Key), so a root reached via a symlink resolves to the same project as its
// real path; a symlinked marker file is honored at its containing directory.
func Detect(ctx context.Context, cwd string) (root string, key string, err error) {
	if err := ctx.Err(); err != nil {
		return "", "", fmt.Errorf("project: detect: %w", err)
	}
	abs, err := filepath.Abs(cwd)
	if err != nil {
		return "", "", fmt.Errorf("project: detect: abs: %w", err)
	}
	abs = filepath.Clean(abs)

	// Pass 1: nearest pmmcp.yaml/pmmcp.yml wins over any VCS root.
	if dir, ok, err := walkUp(ctx, abs, configMarkers); err != nil {
		return "", "", err
	} else if ok {
		return dir, Key(dir), nil
	}
	// Pass 2: nearest VCS root.
	if dir, ok, err := walkUp(ctx, abs, vcsMarkers); err != nil {
		return "", "", err
	} else if ok {
		return dir, Key(dir), nil
	}
	// No marker: cwd is the root; caller decides global-scope policy.
	return abs, Key(abs), nil
}

// walkUp climbs from dir toward the filesystem root looking for any of markers.
func walkUp(ctx context.Context, dir string, markers []string) (string, bool, error) {
	for {
		if err := ctx.Err(); err != nil {
			return "", false, fmt.Errorf("project: detect: %w", err)
		}
		if hasMarker(dir, markers) {
			return dir, true, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false, nil
		}
		dir = parent
	}
}

// Key returns the canonical project key for a root path (cleaned absolute path,
// symlinks resolved when possible — / scope-project).
func Key(root string) string {
	abs, err := filepath.Abs(root)
	if err != nil {
		return filepath.Clean(root)
	}
	abs = filepath.Clean(abs)
	if resolved, err := filepath.EvalSymlinks(abs); err == nil && resolved != "" {
		return filepath.Clean(resolved)
	}
	return abs
}

// hasMarker reports whether dir contains any of the given marker names.
func hasMarker(dir string, markers []string) bool {
	for _, name := range markers {
		path := filepath.Join(dir, name)
		if _, err := os.Lstat(path); err == nil {
			return true
		}
	}
	return false
}
