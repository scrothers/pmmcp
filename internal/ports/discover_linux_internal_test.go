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

package ports

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
)

func TestSocketInodesFromDirMissing(t *testing.T) {
	t.Parallel()
	if got := socketInodesFromDir(filepath.Join(t.TempDir(), "does-not-exist")); got != nil {
		t.Fatalf("socketInodesFromDir(missing) = %v, want nil", got)
	}
}

func TestSocketInodesFromDirEntries(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Valid socket fd.
	if err := os.Symlink("socket:[42]", filepath.Join(dir, "0")); err != nil {
		t.Fatal(err)
	}
	// Non-socket fd (e.g. a regular file descriptor): skipped by the prefix check.
	if err := os.Symlink("/dev/null", filepath.Join(dir, "1")); err != nil {
		t.Fatal(err)
	}
	// Not a symlink at all: os.Readlink fails, entry skipped.
	if err := os.WriteFile(filepath.Join(dir, "2"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Malformed inode digits: ParseUint fails, entry skipped.
	if err := os.Symlink("socket:[notanumber]", filepath.Join(dir, "3")); err != nil {
		t.Fatal(err)
	}

	got := socketInodesFromDir(dir)
	if _, ok := got[42]; !ok || len(got) != 1 {
		t.Fatalf("socketInodesFromDir = %v, want {42}", got)
	}
}

func TestParseListenTableOpenError(t *testing.T) {
	t.Parallel()
	if got := parseListenTable(filepath.Join(t.TempDir(), "missing"), nil); got != nil {
		t.Fatalf("parseListenTable(missing) = %v, want nil", got)
	}
}

func TestParseListenTableEmptyFile(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "tcp")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if got := parseListenTable(path, map[uint64]struct{}{1: {}}); got != nil {
		t.Fatalf("parseListenTable(empty) = %v, want nil", got)
	}
}

func writeTable(t *testing.T, name string, lines ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	content := "header line ignored\n"
	for _, ln := range lines {
		content += ln + "\n"
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestParseListenTableLineBranches(t *testing.T) {
	t.Parallel()
	addr := procHex(net.ParseIP("127.0.0.1"), false)

	tests := []struct {
		name   string
		line   string
		inodes map[uint64]struct{}
		want   int
	}{
		{"too few fields", "0: 0100007F:1F90 00000000:0000 0A", map[uint64]struct{}{99: {}}, 0},
		{"bad inode field", fmt.Sprintf("0: %s:1F90 00000000:0000 0A 00000000:00000000 00:00000000 00000000 0 0 notanumber", addr), map[uint64]struct{}{99: {}}, 0},
		{"bad hex address", "0: ZZZZZZZZ:1F90 00000000:0000 0A 00000000:00000000 00:00000000 00000000 0 0 99", map[uint64]struct{}{99: {}}, 0},
		{"inode not tracked", fmt.Sprintf("0: %s:1F90 00000000:0000 0A 00000000:00000000 00:00000000 00000000 0 0 5", addr), map[uint64]struct{}{99: {}}, 0},
		{"matches", fmt.Sprintf("0: %s:1F90 00000000:0000 0A 00000000:00000000 00:00000000 00000000 0 0 99", addr), map[uint64]struct{}{99: {}}, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := writeTable(t, "tcp", tt.line)
			got := parseListenTable(path, tt.inodes)
			if len(got) != tt.want {
				t.Fatalf("parseListenTable(%q) = %v, want %d matches", tt.line, got, tt.want)
			}
		})
	}
}

func TestDiscoverFromTablesDedups(t *testing.T) {
	t.Parallel()
	addr := procHex(net.ParseIP("127.0.0.1"), false)
	line := fmt.Sprintf("0: %s:1F90 00000000:0000 0A 00000000:00000000 00:00000000 00000000 0 0 99", addr)
	// Two independent table files list the very same listening address under the
	// same tracked inode; the second occurrence must be deduplicated.
	t1 := writeTable(t, "tcp", line)
	t2 := writeTable(t, "tcp", line)

	got := discoverFromTables(map[uint64]struct{}{99: {}}, []string{t1, t2})
	if len(got) != 1 {
		t.Fatalf("discoverFromTables = %v, want 1 deduplicated entry", got)
	}
}

func TestDiscoverListeningPortsNoSockets(t *testing.T) {
	t.Parallel()
	// A pid whose /proc/<pid>/fd cannot be read yields no inodes, short-circuiting
	// before any table is consulted. 1<<30 is far above the default pid_max, so
	// this deterministically does not exist.
	const nonexistentPID = 1 << 30
	if got := DiscoverListeningPorts(nonexistentPID); got != nil {
		t.Fatalf("DiscoverListeningPorts(nonexistent) = %v, want nil", got)
	}
}

func TestParseHexAddrOrderBigEndianArm(t *testing.T) {
	t.Parallel()
	// binary.NativeEndian is little-endian on every GOARCH this codebase ships to,
	// so the BigEndian arm of the word-order swap is unreachable via parseHexAddr
	// at runtime. Drive it directly through the injectable order so both arms are
	// exercised regardless of host architecture.
	tests := []struct {
		name string
		ip   string
		v6   bool
	}{
		{"ipv4", "93.184.216.34", false},
		{"ipv6", "2606:2800:220:1:248:1893:25c8:1946", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ip := net.ParseIP(tt.ip)
			if ip == nil {
				t.Fatalf("bad test ip %q", tt.ip)
			}
			// Under order=BigEndian, the natural network-order byte layout of the
			// address (i.e. its plain IP bytes) round-trips through the swap as an
			// identity transform.
			var raw []byte
			if tt.v6 {
				raw = ip.To16()
			} else {
				raw = ip.To4()
			}
			s := fmt.Sprintf("%s:%04X", hex.EncodeToString(raw), 443)
			host, port, err := parseHexAddrOrder(s, tt.v6, binary.BigEndian)
			if err != nil {
				t.Fatalf("parseHexAddrOrder: %v", err)
			}
			if got := net.ParseIP(host); got == nil || !got.Equal(ip) {
				t.Fatalf("host = %q, want %s", host, ip)
			}
			if port != 443 {
				t.Fatalf("port = %d, want 443", port)
			}
		})
	}
}
