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

package logcap

import (
	"bytes"
	"io"

	"github.com/scrothers/pmmcp/internal/secret"
)

// maxPartialBytes caps the buffered bytes of an unterminated (newline-free) line.
// Once exceeded, the buffer is redacted and flushed so a hostile or buggy child
// emitting a long run without a newline cannot grow daemon memory without bound.
// It mirrors the read-path scanner limit.
const maxPartialBytes = 256 * 1024

// RedactWriter wraps an io.Writer and redacts sensitive material line-by-line
// via secret.RedactLine before it hits disk. It does not
// rotate; use Capturer.OpenStdoutWriter / OpenStderrWriter for size-aware
// capture.
//
// secret.RedactLine applies key-name, registered-value, and global-pattern
// redaction from the secret package's shared default. Value-based redaction of a
// specific process's secrets requires the daemon to register those resolved
// values with secret.RegisterNamedValue at process start.
type RedactWriter struct {
	// W is the underlying sink.
	W       io.Writer
	partial []byte
}

// Write implements io.Writer.
func (r *RedactWriter) Write(p []byte) (int, error) {
	if r == nil || r.W == nil {
		return 0, io.ErrClosedPipe
	}
	data := append(r.partial, p...)
	r.partial = nil
	for {
		i := bytes.IndexByte(data, '\n')
		if i < 0 {
			if len(data) > maxPartialBytes {
				if _, err := io.WriteString(r.W, secret.RedactLine(string(data))); err != nil {
					return len(p), err
				}
				r.partial = nil
			} else {
				r.partial = append([]byte(nil), data...)
			}
			return len(p), nil
		}
		line := data[:i]
		data = data[i+1:]
		if _, err := io.WriteString(r.W, secret.RedactLine(string(line))+"\n"); err != nil {
			return len(p), err
		}
	}
}

// Close flushes any buffered partial line.
func (r *RedactWriter) Close() error {
	if r == nil || r.W == nil {
		return nil
	}
	if len(r.partial) > 0 {
		out := secret.RedactLine(string(r.partial))
		_, err := io.WriteString(r.W, out)
		r.partial = nil
		return err
	}
	return nil
}
