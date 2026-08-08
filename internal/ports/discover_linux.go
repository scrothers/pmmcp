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
	"bufio"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// DiscoverListeningPorts returns host:port strings for TCP sockets in LISTEN
// state owned by pid. Empty when the process has no listeners or /proc is
// unreadable. Order is stable by appearance in /proc/net/tcp then tcp6.
func DiscoverListeningPorts(pid int) []string {
	if pid <= 0 {
		return nil
	}
	inodes := socketInodes(pid)
	if len(inodes) == 0 {
		return nil
	}
	return discoverFromTables(inodes, []string{"/proc/net/tcp", "/proc/net/tcp6"})
}

// discoverFromTables merges listening addresses from tables (in order),
// deduplicating repeats across tables. Split out of DiscoverListeningPorts so
// tests can supply synthetic table files instead of the real /proc/net/tcp{,6}.
func discoverFromTables(inodes map[uint64]struct{}, tables []string) []string {
	var out []string
	seen := make(map[string]struct{})
	for _, table := range tables {
		for _, addr := range parseListenTable(table, inodes) {
			if _, ok := seen[addr]; ok {
				continue
			}
			seen[addr] = struct{}{}
			out = append(out, addr)
		}
	}
	return out
}

func socketInodes(pid int) map[uint64]struct{} {
	return socketInodesFromDir(fmt.Sprintf("/proc/%d/fd", pid))
}

// socketInodesFromDir is socketInodes with an injectable fd directory, so
// tests can exercise the ReadDir/Readlink error and malformed-entry branches
// without needing a real process's /proc/<pid>/fd.
func socketInodesFromDir(dir string) map[uint64]struct{} {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	out := make(map[uint64]struct{})
	for _, e := range entries {
		target, err := os.Readlink(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		// socket:[inode]
		if !strings.HasPrefix(target, "socket:[") || !strings.HasSuffix(target, "]") {
			continue
		}
		inoStr := target[len("socket:[") : len(target)-1]
		ino, err := strconv.ParseUint(inoStr, 10, 64)
		if err != nil {
			continue
		}
		out[ino] = struct{}{}
	}
	return out
}

func parseListenTable(path string, inodes map[uint64]struct{}) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()

	var out []string
	sc := bufio.NewScanner(f)
	// Skip header.
	if !sc.Scan() {
		return nil
	}
	isIPv6 := strings.HasSuffix(path, "tcp6")
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		// local_address rem_address st ... uid timeout inode
		if len(fields) < 10 {
			continue
		}
		// st == 0A → TCP_LISTEN
		if fields[3] != "0A" {
			continue
		}
		inode, err := strconv.ParseUint(fields[9], 10, 64)
		if err != nil {
			continue
		}
		if _, ok := inodes[inode]; !ok {
			continue
		}
		host, port, err := parseHexAddr(fields[1], isIPv6)
		if err != nil {
			continue
		}
		out = append(out, net.JoinHostPort(host, strconv.Itoa(port)))
	}
	return out
}

func parseHexAddr(s string, ipv6 bool) (host string, port int, err error) {
	return parseHexAddrOrder(s, ipv6, binary.NativeEndian)
}

// parseHexAddrOrder is parseHexAddr with the source word order injectable, so
// tests can exercise both the little- and big-endian decode arms regardless of
// GOARCH (on amd64 binary.NativeEndian is always little-endian at runtime).
func parseHexAddrOrder(s string, ipv6 bool, order binary.ByteOrder) (host string, port int, err error) {
	ipPort := strings.Split(s, ":")
	if len(ipPort) != 2 {
		return "", 0, fmt.Errorf("bad addr %q", s)
	}
	port64, err := strconv.ParseUint(ipPort[1], 16, 16)
	if err != nil {
		return "", 0, err
	}
	port = int(port64)
	raw, err := hex.DecodeString(ipPort[0])
	if err != nil {
		return "", 0, err
	}
	// /proc/net/tcp{,6} prints each 32-bit word of the address with the host's
	// native byte order. Reading each word as NativeEndian and re-emitting it as
	// BigEndian (network order) yields the correct address on both little- and
	// big-endian hosts.
	if !ipv6 {
		if len(raw) != 4 {
			return "", 0, fmt.Errorf("bad ipv4 len")
		}
		var b [4]byte
		binary.BigEndian.PutUint32(b[:], order.Uint32(raw))
		return net.IP(b[:]).String(), port, nil
	}
	if len(raw) != 16 {
		return "", 0, fmt.Errorf("bad ipv6 len")
	}
	var b [16]byte
	for i := 0; i < 16; i += 4 {
		binary.BigEndian.PutUint32(b[i:i+4], order.Uint32(raw[i:i+4]))
	}
	return net.IP(b[:]).String(), port, nil
}
