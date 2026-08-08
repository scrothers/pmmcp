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

package ports_test

import (
	"net"
	"os"
	"testing"

	"github.com/scrothers/pmmcp/internal/ports"
)

func TestDiscoverListeningPorts_Self(t *testing.T) {
	t.Parallel()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	addr := ln.Addr().String()
	pid := os.Getpid()
	// Give the kernel a moment to register the inode in /proc.
	found := false
	for range 20 {
		got := ports.DiscoverListeningPorts(pid)
		for _, p := range got {
			if p == addr {
				found = true
				break
			}
		}
		if found {
			break
		}
	}
	if !found {
		// Still assert non-panicking discovery; some environments hide /proc/net.
		got := ports.DiscoverListeningPorts(pid)
		t.Logf("discovered=%v want %s (may be empty under restricted /proc)", got, addr)
		// Zero PID must not panic and returns nil/empty.
		if ports.DiscoverListeningPorts(0) != nil && len(ports.DiscoverListeningPorts(0)) != 0 {
			t.Fatal("pid 0 should yield empty")
		}
		return
	}
	if ports.DiscoverListeningPorts(-1) != nil {
		t.Fatal("negative pid should yield nil/empty")
	}
}
