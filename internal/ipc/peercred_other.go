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

//go:build !linux

package ipc

import (
	"fmt"
	"net"
	"os"
)

// PeerUID is best-effort off Linux (macOS getpeereid via x/sys when needed).
// Unix socket mode 0600 remains the primary access control.
func PeerUID(conn net.Conn) (uint32, error) {
	if _, ok := conn.(*net.UnixConn); !ok {
		return 0, fmt.Errorf("ipc: peercred: not a unix conn")
	}
	// Without SO_PEERCRED, trust filesystem socket permissions (0600).
	return uint32(os.Getuid()), nil
}

// AllowedUID is the daemon process UID.
func AllowedUID() uint32 {
	return uint32(os.Getuid())
}
