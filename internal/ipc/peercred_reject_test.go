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

//go:build linux

package ipc_test

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/scrothers/pmmcp/internal/ipc"
)

// TestPeerFilterRejectsMismatchedUID verifies the filter compares peer UID to
// allowed UID. We cannot easily forge SO_PEERCRED as another user without root,
// so this asserts the same-user accept path and that AllowedUID matches getuid.
func TestPeerFilterSameUIDOnly(t *testing.T) {
	t.Parallel()
	if ipc.AllowedUID() != uint32(os.Getuid()) {
		t.Fatalf("AllowedUID=%d getuid=%d", ipc.AllowedUID(), os.Getuid())
	}
	dir := t.TempDir()
	sock := filepath.Join(dir, "p.sock")
	ln, err := ipc.Listen(sock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	// Socket must be 0600 (no o+rw).
	st, err := os.Stat(sock)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm()&0o077 != 0 {
		t.Fatalf("socket mode %o allows group/other", st.Mode().Perm())
	}

	errCh := make(chan error, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			errCh <- err
			return
		}
		uid, err := ipc.PeerUID(c)
		_ = c.Close()
		if err != nil {
			errCh <- err
			return
		}
		if uid != ipc.AllowedUID() {
			errCh <- fmt.Errorf("uid mismatch got %d", uid)
			return
		}
		errCh <- nil
	}()
	conn, err := net.Dial("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	_ = conn.Close()
	if err := <-errCh; err != nil {
		t.Fatal(err)
	}
}
