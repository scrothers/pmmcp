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

package logcap_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/scrothers/pmmcp/internal/logcap"
)

// failingSink is an io.Writer that always fails, used to exercise
// RedactWriter's error-propagation paths without any OS-level trickery: W is
// a plain interface field, so a fault can be injected directly.
type failingSink struct{}

func (failingSink) Write([]byte) (int, error) { return 0, errors.New("injected sink failure") }

func TestRedactWriterWriteOverCapFlushFails(t *testing.T) {
	t.Parallel()
	w := &logcap.RedactWriter{W: failingSink{}}
	// No newline and over maxPartialBytes: Write must flush immediately via
	// the sink, which fails.
	if _, err := w.Write([]byte(strings.Repeat("A", 300*1024))); err == nil {
		t.Fatal("expected error from failing sink on over-cap flush")
	}
}

func TestRedactWriterWriteLineFails(t *testing.T) {
	t.Parallel()
	w := &logcap.RedactWriter{W: failingSink{}}
	if _, err := w.Write([]byte("a complete line\n")); err == nil {
		t.Fatal("expected error from failing sink on line write")
	}
}

func TestRedactWriterCloseNoPartialSucceeds(t *testing.T) {
	t.Parallel()
	// A non-nil sink with nothing buffered: Close must fall through to its
	// final success return without attempting any flush.
	w := &logcap.RedactWriter{W: failingSink{}}
	if err := w.Close(); err != nil {
		t.Fatalf("Close with no buffered partial = %v, want nil", err)
	}
}

func TestRedactWriterCloseFlushFails(t *testing.T) {
	t.Parallel()
	w := &logcap.RedactWriter{W: failingSink{}}
	// Buffer a partial (no-newline) line directly via a write short enough to
	// stay under maxPartialBytes, so Write itself succeeds and buffers it,
	// then Close attempts to flush it through the failing sink.
	if _, err := w.Write([]byte("partial no newline")); err != nil {
		t.Fatalf("buffering write should not fail: %v", err)
	}
	if err := w.Close(); err == nil {
		t.Fatal("expected error from failing sink on Close flush")
	}
}
