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

package ipc

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"time"
)

// Listen opens the private IPC listener for endpoint (UDS path or Windows pipe).
func Listen(endpoint string) (net.Listener, error) {
	if endpoint == "" {
		return nil, fmt.Errorf("ipc: listen: empty endpoint")
	}
	if strings.HasPrefix(endpoint, `\\.\pipe\`) || strings.HasPrefix(endpoint, `//./pipe/`) {
		return listenNamedPipe(endpoint)
	}
	return listenUnix(endpoint)
}

// listenUnix creates a Unix domain socket with mode 0600, dir 0700. Startup
// refuses an insecurely linked or mispermissioned socket path.
func listenUnix(endpoint string) (net.Listener, error) {
	if err := secureSocketDir(filepathDir(endpoint)); err != nil {
		return nil, err
	}
	if err := prepareSocketPath(endpoint); err != nil {
		return nil, err
	}
	ln, err := net.Listen("unix", endpoint)
	if err != nil {
		return nil, fmt.Errorf("ipc: listen: %w", err)
	}
	if err := os.Chmod(endpoint, 0o600); err != nil {
		_ = ln.Close()
		return nil, fmt.Errorf("ipc: chmod socket: %w", err)
	}
	return &peerFilterListener{Listener: ln, allowUID: AllowedUID()}, nil
}

// secureSocketDir ensures dir exists as a private (0700), non-symlink directory,
// tightening the mode of a pre-existing directory rather than trusting it.
func secureSocketDir(dir string) error {
	fi, err := os.Lstat(dir)
	switch {
	case errors.Is(err, os.ErrNotExist):
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("ipc: listen: socket dir: %w", err)
		}
		return nil
	case err != nil:
		return fmt.Errorf("ipc: listen: stat socket dir: %w", err)
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("ipc: listen: socket dir %q is a symlink", dir)
	}
	if !fi.IsDir() {
		return fmt.Errorf("ipc: listen: socket path parent %q is not a directory", dir)
	}
	if fi.Mode().Perm()&0o077 != 0 {
		if err := os.Chmod(dir, 0o700); err != nil {
			return fmt.Errorf("ipc: listen: socket dir %q has insecure mode %o and cannot be tightened: %w", dir, fi.Mode().Perm(), err)
		}
	}
	return nil
}

// prepareSocketPath refuses a symlinked or non-socket endpoint and removes only
// a stale socket, probing first so a live daemon's socket is not silently stolen.
func prepareSocketPath(endpoint string) error {
	fi, err := os.Lstat(endpoint)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("ipc: listen: stat socket: %w", err)
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("ipc: listen: socket path %q is a symlink", endpoint)
	}
	if fi.Mode()&os.ModeSocket == 0 {
		return fmt.Errorf("ipc: listen: refusing to remove non-socket file at %q", endpoint)
	}
	if c, derr := net.DialTimeout("unix", endpoint, 200*time.Millisecond); derr == nil {
		_ = c.Close()
		return fmt.Errorf("ipc: listen: another daemon is already listening on %q", endpoint)
	}
	if err := os.Remove(endpoint); err != nil {
		return fmt.Errorf("ipc: listen: remove stale socket: %w", err)
	}
	return nil
}

func filepathDir(p string) string {
	i := strings.LastIndexByte(p, '/')
	if i < 0 {
		return "."
	}
	if i == 0 {
		return "/"
	}
	return p[:i]
}

// peerFilterListener rejects connections from other OS users (same-UID).
type peerFilterListener struct {
	net.Listener
	allowUID uint32
}

// Accept enforces same-UID via SO_PEERCRED on Linux.
func (l *peerFilterListener) Accept() (net.Conn, error) {
	for {
		c, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}
		uid, err := PeerUID(c)
		if err != nil {
			// If we cannot read creds, fail closed for unix sockets.
			_ = c.Close()
			continue
		}
		// Same-user only. Other UIDs — including root when
		// the daemon is not root — are rejected.
		if uid != l.allowUID {
			_ = c.Close()
			continue
		}
		return c, nil
	}
}
