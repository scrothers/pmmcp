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

package logcap

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const (
	// stdoutName is the active stdout capture filename.
	stdoutName = "stdout.log"
	// stderrName is the active stderr capture filename.
	stderrName = "stderr.log"

	//: 10 MiB current, keep 5 archives, gzip on rotate.
	defaultMaxFileMB = 10
	defaultMaxFiles  = 5
)

// Capturer captures process stdout/stderr into rotating files under Dir.
type Capturer struct {
	// Dir is the per-process log directory.
	Dir string
	// MaxFileBytes is the size threshold that triggers rotation.
	MaxFileBytes int64
	// MaxFiles is the number of rotated backups retained per stream (name.log.1 … name.log.N).
	MaxFiles int
	// Compress gzip-compresses rotated archives.
	Compress bool
}

// New creates a Capturer for dir, creating the directory if needed (mode 0700).
// maxFileMB and maxFiles fall back to defaults (10 MiB, 5 backups) when ≤ 0.
// Compression is enabled by default.
func New(dir string, maxFileMB, maxFiles int) (*Capturer, error) {
	if dir == "" {
		return nil, fmt.Errorf("logcap: empty dir")
	}
	if maxFileMB <= 0 {
		maxFileMB = defaultMaxFileMB
	}
	if maxFiles <= 0 {
		maxFiles = defaultMaxFiles
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("logcap: mkdir: %w", err)
	}
	// MkdirAll is umask-subject and won't tighten a pre-existing looser dir;
	// enforce 0700 explicitly.
	if err := os.Chmod(dir, 0o700); err != nil {
		return nil, fmt.Errorf("logcap: chmod: %w", err)
	}
	return &Capturer{
		Dir:          dir,
		MaxFileBytes: int64(maxFileMB) * 1024 * 1024,
		MaxFiles:     maxFiles,
		Compress:     true,
	}, nil
}

// StdoutPath returns the path of the active stdout log file.
func (c *Capturer) StdoutPath() string {
	return filepath.Join(c.Dir, stdoutName)
}

// StderrPath returns the path of the active stderr log file.
func (c *Capturer) StderrPath() string {
	return filepath.Join(c.Dir, stderrName)
}

// OpenStdout creates or appends the stdout log (mode 0600) for process redirect.
func (c *Capturer) OpenStdout() (*os.File, error) {
	return c.open(c.StdoutPath())
}

// OpenStderr creates or appends the stderr log (mode 0600) for process redirect.
func (c *Capturer) OpenStderr() (*os.File, error) {
	return c.open(c.StderrPath())
}

func (c *Capturer) open(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("logcap: open %s: %w", filepath.Base(path), err)
	}
	return f, nil
}

// RotateIfNeeded checks active log sizes and rotates oversized files.
// Rotation renames name.log → name.log.1 → … → name.log.N and drops older backups.
// Callers that hold open writers should reopen after a successful rotate.
func (c *Capturer) RotateIfNeeded() error {
	if c == nil {
		return fmt.Errorf("logcap: nil capturer")
	}
	for _, name := range []string{stdoutName, stderrName} {
		if err := c.rotateOne(name); err != nil {
			return err
		}
	}
	return nil
}

func (c *Capturer) rotateOne(base string) error {
	path := filepath.Join(c.Dir, base)
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("logcap: stat %s: %w", base, err)
	}
	if info.Size() < c.MaxFileBytes {
		return nil
	}
	return c.rotateNow(base)
}

// rotateNow shifts backups and renames the active base file to.1 (gzipping when
// Compress is set) unconditionally, without a size check. Callers that hold the
// active file open must reopen after this returns (RotatingWriter does so under
// its lock); a missing active file is treated as nothing to rotate.
func (c *Capturer) rotateNow(base string) error {
	path := filepath.Join(c.Dir, base)

	// Drop oldest backups (plain and gzip forms).
	for _, suffix := range []string{"", ".gz"} {
		oldest := filepath.Join(c.Dir, fmt.Sprintf("%s.%d%s", base, c.MaxFiles, suffix))
		if err := os.Remove(oldest); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("logcap: remove %s: %w", filepath.Base(oldest), err)
		}
	}

	// Shift backups upward:.(N-1) →.N, …,.1 →.2 (both plain and.gz)
	for i := c.MaxFiles - 1; i >= 1; i-- {
		for _, suffix := range []string{"", ".gz"} {
			src := filepath.Join(c.Dir, fmt.Sprintf("%s.%d%s", base, i, suffix))
			dst := filepath.Join(c.Dir, fmt.Sprintf("%s.%d%s", base, i+1, suffix))
			if err := os.Rename(src, dst); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("logcap: rename %s: %w", filepath.Base(src), err)
			}
		}
	}

	// Active file becomes.1
	dst := filepath.Join(c.Dir, base+".1")
	if err := os.Rename(path, dst); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("logcap: rename %s: %w", base, err)
	}
	if c.Compress {
		if err := gzipFile(dst); err != nil {
			return err
		}
	}
	return nil
}

// gzipFile compresses path to path.gz and removes path.
func gzipFile(path string) error {
	in, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("logcap: gzip open: %w", err)
	}
	defer func() { _ = in.Close() }()
	outPath := path + ".gz"
	out, err := os.OpenFile(outPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("logcap: gzip create: %w", err)
	}
	zw := gzip.NewWriter(out)
	zw.Name = filepath.Base(path)
	if _, err := io.Copy(zw, in); err != nil {
		_ = zw.Close()
		_ = out.Close()
		_ = os.Remove(outPath)
		return fmt.Errorf("logcap: gzip write: %w", err)
	}
	if err := zw.Close(); err != nil {
		_ = out.Close()
		return fmt.Errorf("logcap: gzip close: %w", err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("logcap: gzip out close: %w", err)
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("logcap: gzip remove plain: %w", err)
	}
	return nil
}
