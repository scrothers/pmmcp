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
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/scrothers/pmmcp/internal/secret"
)

// RotatingWriter is the write-path capture sink for one process stream. It
// redacts each line via secret.RedactLine before it reaches disk and enforces
// the size cap continuously: when the active file would exceed the
// capturer's MaxFileBytes, it rotates (rename + gzip) and reopens a fresh active
// file, all while holding its lock, so no output written through it is lost
// across a rotation.
//
// Value-based redaction of a process's secrets relies on the daemon registering
// those resolved values with secret.RegisterNamedValue at process start;
// RedactLine consults the secret package's shared default.
type RotatingWriter struct {
	mu      sync.Mutex
	cap     *Capturer
	base    string // stdoutName or stderrName
	f       *os.File
	size    int64
	partial []byte
	closed  bool
}

// OpenStdoutWriter returns a size-aware, redacting writer for the stdout stream.
func (c *Capturer) OpenStdoutWriter() (*RotatingWriter, error) {
	return c.openWriter(stdoutName)
}

// OpenStderrWriter returns a size-aware, redacting writer for the stderr stream.
func (c *Capturer) OpenStderrWriter() (*RotatingWriter, error) {
	return c.openWriter(stderrName)
}

func (c *Capturer) openWriter(base string) (*RotatingWriter, error) {
	if c == nil {
		return nil, fmt.Errorf("logcap: nil capturer")
	}
	path := filepath.Join(c.Dir, base)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("logcap: open %s: %w", base, err)
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("logcap: stat %s: %w", base, err)
	}
	return &RotatingWriter{cap: c, base: base, f: f, size: info.Size()}, nil
}

// Path returns the active log file path.
func (w *RotatingWriter) Path() string {
	return filepath.Join(w.cap.Dir, w.base)
}

// Write implements io.Writer, redacting complete lines and rotating on the size
// cap at line boundaries.
func (w *RotatingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed || w.f == nil {
		return 0, io.ErrClosedPipe
	}
	data := append(w.partial, p...)
	w.partial = nil
	for {
		i := bytes.IndexByte(data, '\n')
		if i < 0 {
			if len(data) > maxPartialBytes {
				if err := w.emit(secret.RedactLine(string(data))); err != nil {
					return len(p), err
				}
				w.partial = nil
			} else {
				w.partial = append([]byte(nil), data...)
			}
			return len(p), nil
		}
		line := data[:i]
		data = data[i+1:]
		if err := w.emit(secret.RedactLine(string(line)) + "\n"); err != nil {
			return len(p), err
		}
	}
}

// emit writes an already-redacted chunk, rotating first if it would push the
// active file past the cap. Caller holds w.mu.
func (w *RotatingWriter) emit(s string) error {
	if w.cap.MaxFileBytes > 0 && w.size > 0 && w.size+int64(len(s)) > w.cap.MaxFileBytes {
		if err := w.rotate(); err != nil {
			return err
		}
	}
	n, err := io.WriteString(w.f, s)
	w.size += int64(n)
	if err != nil {
		return fmt.Errorf("logcap: write %s: %w", w.base, err)
	}
	return nil
}

// rotate closes the active fd, rotates the file, and reopens a fresh active fd.
// Closing before the rename guarantees the writer never keeps writing to a
// renamed/removed inode, so no data is lost. Caller holds w.mu.
func (w *RotatingWriter) rotate() error {
	if err := w.f.Close(); err != nil {
		return fmt.Errorf("logcap: rotate close %s: %w", w.base, err)
	}
	rotErr := w.cap.rotateNow(w.base)
	f, err := os.OpenFile(w.Path(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		w.f = nil
		if rotErr != nil {
			return rotErr
		}
		return fmt.Errorf("logcap: reopen %s: %w", w.base, err)
	}
	w.f = f
	if info, statErr := f.Stat(); statErr == nil {
		w.size = info.Size()
	} else {
		w.size = 0
	}
	return rotErr
}

// Close flushes any buffered partial line and closes the active file.
func (w *RotatingWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return nil
	}
	w.closed = true
	var flushErr error
	if len(w.partial) > 0 && w.f != nil {
		if _, err := io.WriteString(w.f, secret.RedactLine(string(w.partial))); err != nil {
			flushErr = fmt.Errorf("logcap: flush %s: %w", w.base, err)
		}
		w.partial = nil
	}
	if w.f != nil {
		if err := w.f.Close(); err != nil && flushErr == nil {
			flushErr = fmt.Errorf("logcap: close %s: %w", w.base, err)
		}
		w.f = nil
	}
	return flushErr
}
