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
	"testing"
)

// procHex renders ip the way the kernel writes it in /proc/net/tcp{,6}: each
// 32-bit word of the network-order address in the host's native byte order.
// This inverts parseHexAddr, so a round-trip is endian-agnostic.
func procHex(ip net.IP, v6 bool) string {
	var raw []byte
	if !v6 {
		raw = make([]byte, 4)
		binary.NativeEndian.PutUint32(raw, binary.BigEndian.Uint32(ip.To4()))
	} else {
		raw = make([]byte, 16)
		ip16 := ip.To16()
		for i := 0; i < 16; i += 4 {
			binary.NativeEndian.PutUint32(raw[i:i+4], binary.BigEndian.Uint32(ip16[i:i+4]))
		}
	}
	return hex.EncodeToString(raw)
}

func TestParseHexAddrRoundTrip(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		ip   string
		v6   bool
		port int
	}{
		{"ipv4 loopback", "127.0.0.1", false, 8080},
		{"ipv4 wildcard", "0.0.0.0", false, 80},
		{"ipv4 public", "93.184.216.34", false, 443},
		{"ipv6 loopback", "::1", true, 8080},
		{"ipv6 linklocal", "fe80::1", true, 9000},
		{"ipv6 global", "2606:2800:220:1:248:1893:25c8:1946", true, 443},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ip := net.ParseIP(tt.ip)
			if ip == nil {
				t.Fatalf("bad test ip %q", tt.ip)
			}
			s := fmt.Sprintf("%s:%04X", procHex(ip, tt.v6), tt.port)
			host, port, err := parseHexAddr(s, tt.v6)
			if err != nil {
				t.Fatalf("parseHexAddr(%q): %v", s, err)
			}
			if got := net.ParseIP(host); got == nil || !got.Equal(ip) {
				t.Fatalf("host = %q, want %s", host, ip)
			}
			if port != tt.port {
				t.Fatalf("port = %d, want %d", port, tt.port)
			}
		})
	}
}

func TestParseHexAddrMalformed(t *testing.T) {
	t.Parallel()
	bad := []struct {
		s  string
		v6 bool
	}{
		{"0100007F", false},      // no colon
		{"zzzzzzzz:1F90", false}, // bad hex
		{"0100007F:zzzz", false}, // bad port
		{"0100:1F90", false},     // wrong ipv4 length
		{"0100007F:1F90", true},  // too short for ipv6
	}
	for _, b := range bad {
		if _, _, err := parseHexAddr(b.s, b.v6); err == nil {
			t.Fatalf("parseHexAddr(%q, v6=%v) = nil err, want error", b.s, b.v6)
		}
	}
}
