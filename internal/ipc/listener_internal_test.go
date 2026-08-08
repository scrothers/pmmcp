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

//go:build unix

package ipc

import (
	"errors"
	"net"
	"path/filepath"
	"testing"
)

func TestFilepathDirTable(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want string
	}{
		{"relative-no-slash", "."},
		{"/root-only", "/"},
		{"/a/b/c.sock", "/a/b"},
	}
	for _, tc := range cases {
		if got := filepathDir(tc.in); got != tc.want {
			t.Errorf("filepathDir(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// fakeConnListener is a minimal net.Listener that hands back a queued
// net.Conn per Accept call, used to force peerFilterListener through its
// credential-error path without needing a second OS user.
type fakeConnListener struct {
	conns []net.Conn
	i     int
}

func (f *fakeConnListener) Accept() (net.Conn, error) {
	if f.i >= len(f.conns) {
		return nil, errors.New("fakeConnListener: exhausted")
	}
	c := f.conns[f.i]
	f.i++
	return c, nil
}

func (f *fakeConnListener) Close() error   { return nil }
func (f *fakeConnListener) Addr() net.Addr { return fakeAddr{} }

type fakeAddr struct{}

func (fakeAddr) Network() string { return "fake" }
func (fakeAddr) String() string  { return "fake" }

func TestAcceptRejectsPeerCredError(t *testing.T) {
	t.Parallel()
	// net.Pipe conns are not *net.UnixConn, so PeerUID fails on them: this
	// exercises Accept's fail-closed "cannot read creds" branch.
	a, b := net.Pipe()
	t.Cleanup(func() { _ = a.Close(); _ = b.Close() })
	pfl := &peerFilterListener{
		Listener: &fakeConnListener{conns: []net.Conn{a}},
		allowUID: AllowedUID(),
	}
	if _, err := pfl.Accept(); err == nil {
		t.Fatal("Accept over a fakeConnListener exhausted after the reject: want error, got nil")
	}
}

func TestAcceptRejectsMismatchedUID(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	sock := filepath.Join(dir, "reject.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	// Obtain a real accepted connection (so PeerUID succeeds and reports
	// our own real UID) synchronously, so the mismatch check below is
	// deterministic rather than racing a listener Close against Accept.
	acceptedCh := make(chan net.Conn, 1)
	go func() {
		c, _ := ln.Accept()
		acceptedCh <- c
	}()
	conn, err := net.Dial("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	_ = conn.Close()
	accepted := <-acceptedCh
	if accepted == nil {
		t.Fatal("Accept on the real listener returned a nil conn")
	}
	t.Cleanup(func() { _ = accepted.Close() })

	// A UID that can never equal our own real UID (SO_PEERCRED reports the
	// dialing process's real UID, i.e. ours).
	pfl := &peerFilterListener{
		Listener: &fakeConnListener{conns: []net.Conn{accepted}},
		allowUID: AllowedUID() + 1,
	}
	if _, err := pfl.Accept(); err == nil {
		t.Fatal("Accept with a UID-mismatched peer: want error, got nil")
	}
}
