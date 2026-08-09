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

package ipc_test

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scrothers/pmmcp/internal/ipc"
	"github.com/scrothers/pmmcp/internal/testsock"
)

func TestListenEmptyEndpoint(t *testing.T) {
	t.Parallel()
	if _, err := ipc.Listen(""); err == nil {
		t.Fatal("Listen(\"\"): want error, got nil")
	}
}

func TestListenNamedPipePrefixUnsupportedOnUnix(t *testing.T) {
	t.Parallel()
	if _, err := ipc.Listen(`\\.\pipe\pmmcp-test`); err == nil {
		t.Fatal(`Listen with a named-pipe endpoint on a non-Windows build: want error, got nil`)
	}
	if _, err := ipc.Listen(`//./pipe/pmmcp-test`); err == nil {
		t.Fatal(`Listen with a slash-form named-pipe endpoint: want error, got nil`)
	}
}

func TestListenCreatesFreshNestedDir(t *testing.T) {
	t.Parallel()
	base := filepath.Dir(testsock.Path(t))
	endpoint := filepath.Join(base, "nested", "does", "not", "exist", "pmmcp.sock")
	ln, err := ipc.Listen(endpoint)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer ln.Close()
	fi, err := os.Stat(filepath.Dir(endpoint))
	if err != nil {
		t.Fatalf("stat dir: %v", err)
	}
	if fi.Mode().Perm()&0o077 != 0 {
		t.Fatalf("fresh socket dir mode = %o, want no group/other bits", fi.Mode().Perm())
	}
}

func TestSecureSocketDirParentNotWritable(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory write permission checks")
	}
	t.Parallel()
	base := filepath.Dir(testsock.Path(t))
	parent := filepath.Join(base, "parent")
	if err := os.Mkdir(parent, 0o500); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(parent, 0o700) })
	endpoint := filepath.Join(parent, "nested", "pmmcp.sock")
	if _, err := ipc.Listen(endpoint); err == nil {
		t.Fatal("Listen with unwritable parent: want error, got nil")
	}
}

func TestSecureSocketDirStatError(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	// A directory-name component this long fails Lstat with ENAMETOOLONG,
	// which is not os.ErrNotExist, exercising the generic stat-error path.
	longDir := strings.Repeat("d", 300)
	endpoint := filepath.Join(base, longDir, "pmmcp.sock")
	if _, err := ipc.Listen(endpoint); err == nil {
		t.Fatal("Listen with an overlong dir component: want error, got nil")
	}
}

func TestSecureSocketDirIsSymlink(t *testing.T) {
	t.Parallel()
	base := filepath.Dir(testsock.Path(t))
	realDir := filepath.Join(base, "real")
	if err := os.Mkdir(realDir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	link := filepath.Join(base, "link")
	if err := os.Symlink(realDir, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	endpoint := filepath.Join(link, "pmmcp.sock")
	if _, err := ipc.Listen(endpoint); err == nil {
		t.Fatal("Listen with a symlinked socket dir: want error, got nil")
	}
}

func TestSecureSocketDirNotADirectory(t *testing.T) {
	t.Parallel()
	base := filepath.Dir(testsock.Path(t))
	notDir := filepath.Join(base, "notadir")
	if err := os.WriteFile(notDir, []byte("x"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	endpoint := filepath.Join(notDir, "pmmcp.sock")
	if _, err := ipc.Listen(endpoint); err == nil {
		t.Fatal("Listen with a non-directory socket dir: want error, got nil")
	}
}

func TestListenUnixListenFailsOnOverlongPath(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	// Each path component is well under the 255-byte name limit, but the
	// total length exceeds the kernel's AF_UNIX sun_path buffer (~108
	// bytes), so secureSocketDir/prepareSocketPath succeed and net.Listen
	// itself fails with EINVAL — exercising listenUnix's net.Listen error
	// wrap rather than any of the pre-flight checks.
	comp := strings.Repeat("b", 40)
	endpoint := filepath.Join(base, comp, comp, "sock.sock")
	if _, err := ipc.Listen(endpoint); err == nil {
		t.Fatal("Listen over an AF_UNIX sun_path-length socket path: want error, got nil")
	}
}

func TestListenRemovesTrulyStaleSocketFile(t *testing.T) {
	t.Parallel()
	endpoint := filepath.Join(filepath.Dir(testsock.Path(t)), "dead.sock")

	// Build a socket file with nothing listening behind it: bind, then
	// close without unlinking, leaving a dead-but-present socket inode —
	// unlike ln.Close() via ipc.Listen (which unlinks on close), this
	// reaches prepareSocketPath's actual os.Remove(stale) branch.
	raw, err := net.Listen("unix", endpoint)
	if err != nil {
		t.Fatalf("raw listen: %v", err)
	}
	ul, ok := raw.(*net.UnixListener)
	if !ok {
		t.Fatalf("raw listener is %T, want *net.UnixListener", raw)
	}
	ul.SetUnlinkOnClose(false)
	if err := ul.Close(); err != nil {
		t.Fatalf("close raw listener: %v", err)
	}

	ln, err := ipc.Listen(endpoint)
	if err != nil {
		t.Fatalf("Listen over a dead socket file: %v", err)
	}
	defer ln.Close()
}

func TestPrepareSocketPathRemoveStaleSocketFails(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory write permission checks")
	}
	t.Parallel()
	dir := filepath.Dir(testsock.Path(t))
	endpoint := filepath.Join(dir, "dead.sock")

	raw, err := net.Listen("unix", endpoint)
	if err != nil {
		t.Fatalf("raw listen: %v", err)
	}
	ul, ok := raw.(*net.UnixListener)
	if !ok {
		t.Fatalf("raw listener is %T, want *net.UnixListener", raw)
	}
	ul.SetUnlinkOnClose(false)
	if err := ul.Close(); err != nil {
		t.Fatalf("close raw listener: %v", err)
	}

	// Owner read+execute only: unlink requires write permission on the
	// containing directory, so os.Remove(endpoint) fails with EACCES.
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatalf("chmod dir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	if _, err := ipc.Listen(endpoint); err == nil {
		t.Fatal("Listen removing a stale socket from an unwritable dir: want error, got nil")
	}
}

func TestPrepareSocketPathStatError(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	// An overlong filename component fails Lstat with ENAMETOOLONG (not
	// os.ErrNotExist), exercising prepareSocketPath's generic stat-error path.
	endpoint := filepath.Join(base, strings.Repeat("f", 300))
	if _, err := ipc.Listen(endpoint); err == nil {
		t.Fatal("Listen with an overlong socket filename: want error, got nil")
	}
}

func TestListenTightensLooseSocketDir(t *testing.T) {
	base := filepath.Dir(testsock.Path(t))
	dir := filepath.Join(base, "run")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	ln, err := ipc.Listen(filepath.Join(dir, "pmmcp.sock"))
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer ln.Close()
	fi, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat dir: %v", err)
	}
	if fi.Mode().Perm()&0o077 != 0 {
		t.Fatalf("socket dir mode = %o, want no group/other bits", fi.Mode().Perm())
	}
}

func TestListenRefusesSymlinkedEndpoint(t *testing.T) {
	base := filepath.Dir(testsock.Path(t))
	target := filepath.Join(base, "real")
	if err := os.WriteFile(target, []byte("x"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	link := filepath.Join(base, "pmmcp.sock")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	if _, err := ipc.Listen(link); err == nil {
		t.Fatal("Listen on symlinked endpoint: want error, got nil")
	}
}

func TestListenRefusesNonSocketFile(t *testing.T) {
	base := filepath.Dir(testsock.Path(t))
	endpoint := filepath.Join(base, "pmmcp.sock")
	if err := os.WriteFile(endpoint, []byte("x"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := ipc.Listen(endpoint); err == nil {
		t.Fatal("Listen on non-socket file: want error, got nil")
	}
}

func TestListenRemovesStaleSocket(t *testing.T) {
	endpoint := testsock.Path(t)
	ln, err := ipc.Listen(endpoint)
	if err != nil {
		t.Fatalf("first Listen: %v", err)
	}
	// Close the listener but leave the socket file on disk (stale).
	if err := ln.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	ln2, err := ipc.Listen(endpoint)
	if err != nil {
		t.Fatalf("second Listen over stale socket: %v", err)
	}
	defer ln2.Close()
}

func TestListenRefusesLiveSocketSteal(t *testing.T) {
	endpoint := testsock.Path(t)
	ln, err := ipc.Listen(endpoint)
	if err != nil {
		t.Fatalf("first Listen: %v", err)
	}
	defer ln.Close()
	if _, err := ipc.Listen(endpoint); err == nil {
		t.Fatal("second Listen over live socket: want error, got nil")
	}
}
