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

// Package testsock provides short Unix-socket paths for tests.
//
// t.TempDir embeds the full test (and subtest) name in the directory it
// returns. Unix sockets cap the bindable path at sockaddr_un's sun_path —
// 104 bytes on macOS and other BSDs, 108 on Linux — so a socket placed under
// t.TempDir fails bind/connect with EINVAL as soon as the test name is long,
// which is exactly what happened on the macOS CI runners (whose TMPDIR is
// already ~50 bytes of /var/folders/… before the test name is appended).
//
// Path sidesteps the limit by allocating the socket in its own short
// os.MkdirTemp directory, cleaned up with the test.
package testsock

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// Path returns a fresh IPC endpoint usable in tests on every supported OS:
// a short Unix-socket path on Unix, and a named-pipe path on Windows (where
// AF_UNIX is not the production transport — internal/ipc listens and dials
// pipes for any endpoint with the \\.\pipe\ prefix).
func Path(t testing.TB) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		var b [8]byte
		if _, err := rand.Read(b[:]); err != nil {
			t.Fatalf("testsock: rand: %v", err)
		}
		return `\\.\pipe\pmmcp-test-` + hex.EncodeToString(b[:])
	}
	dir, err := os.MkdirTemp("", "pms")
	if err != nil {
		t.Fatalf("testsock: mkdirtemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return filepath.Join(dir, "d.sock")
}
