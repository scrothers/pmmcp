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

package ipc

import (
	"fmt"
	"net"
	"os"

	"golang.org/x/sys/unix"
)

// PeerUID returns the connecting process's UID via SO_PEERCRED.
func PeerUID(conn net.Conn) (uint32, error) {
	uc, ok := conn.(*net.UnixConn)
	if !ok {
		return 0, fmt.Errorf("ipc: peercred: not a unix conn")
	}
	raw, err := uc.SyscallConn()
	if err != nil {
		return 0, fmt.Errorf("ipc: peercred: syscall conn: %w", err)
	}
	var (
		cred *unix.Ucred
		cerr error
	)
	if err := raw.Control(func(fd uintptr) {
		cred, cerr = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
	}); err != nil {
		return 0, fmt.Errorf("ipc: peercred: control: %w", err)
	}
	if cerr != nil {
		return 0, fmt.Errorf("ipc: peercred: getsockopt: %w", cerr)
	}
	if cred == nil {
		return 0, fmt.Errorf("ipc: peercred: nil credentials")
	}
	return cred.Uid, nil
}

// AllowedUID is the daemon process UID used for same-user enforcement.
func AllowedUID() uint32 {
	return uint32(os.Getuid())
}
