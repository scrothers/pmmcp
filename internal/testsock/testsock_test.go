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

package testsock_test

import (
	"net"
	"runtime"
	"strings"
	"testing"

	"github.com/scrothers/pmmcp/internal/testsock"
)

// TestPathBindsWithinSockaddrLimit is the reason this package exists: even
// under a test whose (deliberately long) name would blow the 104-byte macOS
// sockaddr_un limit via t.TempDir, the returned path must stay bindable.
func TestPathBindsWithinSockaddrLimit_EvenUnderAVeryLongTestNameLikeThisOne(t *testing.T) {
	t.Parallel()
	p := testsock.Path(t)
	if runtime.GOOS == "windows" {
		// Windows gets a named pipe (internal/ipc handles the transport);
		// sockaddr_un limits do not apply.
		if !strings.HasPrefix(p, `\\.\pipe\`) {
			t.Fatalf("Path() = %q, want a \\\\.\\pipe\\ name on windows", p)
		}
		return
	}
	if n := len(p); n > 103 {
		t.Fatalf("Path() = %q (%d bytes), exceeds the 104-byte sockaddr_un floor", p, n)
	}
	ln, err := net.Listen("unix", p)
	if err != nil {
		t.Fatalf("bind %q: %v", p, err)
	}
	_ = ln.Close()
}

func TestPathIsPerCallUnique(t *testing.T) {
	t.Parallel()
	if a, b := testsock.Path(t), testsock.Path(t); a == b {
		t.Fatalf("two Path() calls returned the same path %q", a)
	}
}

// TestPipePathShapeAndUniqueness covers the Windows branch directly — pipe
// names are pure strings, so this runs on every OS.
func TestPipePathShapeAndUniqueness(t *testing.T) {
	t.Parallel()
	a, b := testsock.PipePath(t), testsock.PipePath(t)
	if !strings.HasPrefix(a, `\\.\pipe\pmmcp-test-`) {
		t.Fatalf("PipePath() = %q, want \\\\.\\pipe\\pmmcp-test- prefix", a)
	}
	if a == b {
		t.Fatalf("two PipePath() calls returned the same name %q", a)
	}
}
