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
	"strings"
	"sync"
)

// FileBackend stores secret *values* under a directory with mode 0600 files.
// Refs remain path-based; this is the local keyring stand-in (no OS keyring dep).
type FileBackend struct {
	mu  sync.Mutex
	dir string
}

// NewFileBackend creates dir if needed and tightens it to 0700 (even when it
// pre-exists with looser permissions).
func NewFileBackend(dir string) (*FileBackend, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("secret: keyring dir: %w", err)
	}
	// MkdirAll does not tighten a pre-existing looser directory.
	if err := os.Chmod(dir, 0o700); err != nil {
		return nil, fmt.Errorf("secret: keyring dir chmod: %w", err)
	}
	return &FileBackend{dir: dir}, nil
}

// validateKeyringName rejects names that could escape the keyring directory.
func validateKeyringName(name string) error {
	if name == "" || strings.Contains(name, "/") || strings.Contains(name, `\`) ||
		strings.Contains(name, "..") {
		return fmt.Errorf("secret: invalid keyring name %q", name)
	}
	return nil
}

// Set writes value to dir/name with 0600.
func (b *FileBackend) Set(name, value string) (path string, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := validateKeyringName(name); err != nil {
		return "", err
	}
	path = filepath.Join(b.dir, name)
	if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
		return "", fmt.Errorf("secret: write: %w", err)
	}
	// WriteFile only applies the mode on create; tighten an existing looser file.
	if err := os.Chmod(path, 0o600); err != nil {
		return "", fmt.Errorf("secret: chmod: %w", err)
	}
	return path, nil
}

// Get reads a secret value by name. The name is validated exactly as Set does,
// so a traversal like "../../etc/hostname" is rejected rather than followed.
func (b *FileBackend) Get(name string) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := validateKeyringName(name); err != nil {
		return "", err
	}
	path := filepath.Join(b.dir, name)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("secret: keyring read: %w", err)
	}
	return string(data), nil
}

// ListNames returns secret file names (not values).
func (b *FileBackend) ListNames() ([]string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	entries, err := os.ReadDir(b.dir)
	if err != nil {
		return nil, fmt.Errorf("secret: keyring list: %w", err)
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			out = append(out, e.Name())
		}
	}
	return out, nil
}
