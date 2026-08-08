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
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/scrothers/pmmcp/internal/ipc"
)

func TestPeerUIDSameUser(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	sock := filepath.Join(dir, "t.sock")
	ln, err := ipc.Listen(sock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })

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
		if uid != uint32(os.Getuid()) {
			errCh <- errUID{got: uid}
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

type errUID struct{ got uint32 }

func (e errUID) Error() string { return "uid mismatch" }

func TestPeerUIDNotUnixConn(t *testing.T) {
	t.Parallel()
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	if _, err := ipc.PeerUID(a); err == nil {
		t.Fatal("PeerUID on a non-unix conn: want error, got nil")
	}
}

func TestPeerUIDZeroValueConn(t *testing.T) {
	t.Parallel()
	var zero net.UnixConn
	if _, err := ipc.PeerUID(&zero); err == nil {
		t.Fatal("PeerUID on a zero-value *net.UnixConn: want error, got nil")
	}
}

func TestPeerUIDClosedConn(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	sock := filepath.Join(dir, "closed.sock")
	ln, err := ipc.Listen(sock)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	acceptedCh := make(chan net.Conn, 1)
	go func() {
		c, err := ln.Accept()
		if err == nil {
			acceptedCh <- c
		} else {
			acceptedCh <- nil
		}
	}()
	conn, err := net.Dial("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	uc, ok := conn.(*net.UnixConn)
	if !ok {
		t.Fatalf("dialed conn is %T, want *net.UnixConn", conn)
	}
	if err := uc.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if _, err := ipc.PeerUID(uc); err == nil {
		t.Fatal("PeerUID on a closed unix conn: want error, got nil")
	}
	if c := <-acceptedCh; c != nil {
		_ = c.Close()
	}
}
