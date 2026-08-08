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

package project_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/scrothers/pmmcp/internal/project"
)

func TestDetectGitDirFromNested(t *testing.T) {
	t.Parallel()
	top := t.TempDir()
	nested := filepath.Join(top, "a", "b", "c")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(top, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	root, key, err := project.Detect(context.Background(), nested)
	if err != nil {
		t.Fatal(err)
	}
	wantRoot := filepath.Clean(mustAbs(t, top))
	if root != wantRoot {
		t.Errorf("root = %q, want %q", root, wantRoot)
	}
	// key is symlink-resolved (see Key); wantRoot itself may not be (e.g.
	// macOS's /var -> /private/var), so the expectation must be resolved too.
	wantKey := mustKey(t, wantRoot)
	if key != wantKey {
		t.Errorf("key = %q, want %q", key, wantKey)
	}
	if key != project.Key(root) {
		t.Errorf("key = %q, Key(root) = %q", key, project.Key(root))
	}
}

func TestDetectGitFileFromNested(t *testing.T) {
	t.Parallel()
	// Git worktrees use a .git file rather than a directory.
	top := t.TempDir()
	nested := filepath.Join(top, "pkg", "sub")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	gitFile := filepath.Join(top, ".git")
	if err := os.WriteFile(gitFile, []byte("gitdir: /tmp/fake\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	root, key, err := project.Detect(context.Background(), nested)
	if err != nil {
		t.Fatal(err)
	}
	wantRoot := filepath.Clean(mustAbs(t, top))
	if root != wantRoot {
		t.Errorf("root = %q, want %q", root, wantRoot)
	}
	// key is symlink-resolved (see Key); wantRoot itself may not be (e.g.
	// macOS's /var -> /private/var), so the expectation must be resolved too.
	wantKey := mustKey(t, wantRoot)
	if key != wantKey {
		t.Errorf("key = %q, want %q", key, wantKey)
	}
}

func TestDetectPmmcpYAMLFromNested(t *testing.T) {
	t.Parallel()
	top := t.TempDir()
	nested := filepath.Join(top, "src")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(top, "pmmcp.yaml"), []byte("apiVersion: pmmcp.dev/v1alpha1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	root, _, err := project.Detect(context.Background(), nested)
	if err != nil {
		t.Fatal(err)
	}
	wantRoot := filepath.Clean(mustAbs(t, top))
	if root != wantRoot {
		t.Errorf("root = %q, want %q", root, wantRoot)
	}
}

func TestDetectPmmcpYMLFromNested(t *testing.T) {
	t.Parallel()
	top := t.TempDir()
	nested := filepath.Join(top, "src")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(top, "pmmcp.yml"), []byte("apiVersion: pmmcp.dev/v1alpha1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	root, _, err := project.Detect(context.Background(), nested)
	if err != nil {
		t.Fatal(err)
	}
	wantRoot := filepath.Clean(mustAbs(t, top))
	if root != wantRoot {
		t.Errorf("root = %q, want %q", root, wantRoot)
	}
}

func TestDetectNoMarkersUsesCwd(t *testing.T) {
	t.Parallel()
	// Walk from a marker-free tree. TempDir lives under /tmp (or OS temp),
	// which has no .git / pmmcp.yaml on a normal host, so Detect falls back
	// to abs(cwd).
	cwd := t.TempDir()
	nested := filepath.Join(cwd, "leaf")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}

	root, key, err := project.Detect(context.Background(), nested)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Clean(mustAbs(t, nested))
	if root != want {
		t.Errorf("root = %q, want cwd %q (no markers in tree)", root, want)
	}
	// key is symlink-resolved (see Key); want itself may not be (e.g. macOS's
	// /var -> /private/var), so the expectation must be resolved too.
	wantKey := mustKey(t, want)
	if key != wantKey {
		t.Errorf("key = %q, want %q", key, wantKey)
	}
}

func TestDetectCanceledContext(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err := project.Detect(ctx, t.TempDir())
	if err == nil {
		t.Fatal("expected error for canceled context")
	}
}

func TestKey(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	got := project.Key(dir)
	want := mustKey(t, filepath.Clean(mustAbs(t, dir)))
	if got != want {
		t.Errorf("Key = %q, want %q", got, want)
	}
	if err := os.MkdirAll(filepath.Join(dir, "y"), 0o755); err != nil {
		t.Fatal(err)
	}
	rel := filepath.Join(dir, "x", "..", "y")
	got = project.Key(rel)
	want = mustKey(t, filepath.Clean(mustAbs(t, filepath.Join(dir, "y"))))
	if got != want {
		t.Errorf("Key(rel) = %q, want %q", got, want)
	}
}

func TestDetectPrefersNearestMarker(t *testing.T) {
	t.Parallel()
	// Outer git root and inner pmmcp.yaml — nearest ancestor wins.
	outer := t.TempDir()
	inner := filepath.Join(outer, "apps", "web")
	if err := os.MkdirAll(inner, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(outer, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(inner, "pmmcp.yaml"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(inner, "src")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}

	root, _, err := project.Detect(context.Background(), nested)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Clean(mustAbs(t, inner))
	if root != want {
		t.Errorf("root = %q, want nearest marker dir %q", root, want)
	}
}

func TestDetectConfigBeatsNearerGit(t *testing.T) {
	t.Parallel()
	// Ancestor pmmcp.yaml with a nearer (nested) .git — the ordered two-pass
	// detection must pick the ancestor pmmcp.yaml, not the closer .git.
	outer := t.TempDir()
	sub := filepath.Join(outer, "vendored")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outer, "pmmcp.yaml"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(sub, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(sub, "src")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}

	root, _, err := project.Detect(context.Background(), nested)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Clean(mustAbs(t, outer))
	if root != want {
		t.Errorf("root = %q, want ancestor pmmcp.yaml dir %q (config walk precedes .git walk)", root, want)
	}
}

func mustAbs(t *testing.T, path string) string {
	t.Helper()
	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	return abs
}

// mustKey mirrors project.Key's own symlink resolution (EvalSymlinks, falling
// back to the cleaned input when resolution fails) so test expectations match
// production's normalization on platforms where the OS temp dir is itself a
// symlink (e.g. macOS's /var -> /private/var).
func mustKey(t *testing.T, path string) string {
	t.Helper()
	if resolved, err := filepath.EvalSymlinks(path); err == nil && resolved != "" {
		return filepath.Clean(resolved)
	}
	return path
}
