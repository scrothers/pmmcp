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

package api

import "testing"

func TestParseVersion(t *testing.T) {
	cases := []struct {
		in      string
		major   int
		minor   int
		wantErr bool
		desc    string
	}{
		{in: "1.0", major: 1, minor: 0, desc: "canonical"},
		{in: "10.0", major: 10, minor: 0, desc: "multi-digit major"},
		{in: "1.2", major: 1, minor: 2, desc: "minor set"},
		{in: " 2.5 ", major: 2, minor: 5, desc: "surrounding space"},
		{in: "", wantErr: true, desc: "empty"},
		{in: "1", wantErr: true, desc: "no minor"},
		{in: "1.2.3", wantErr: true, desc: "three parts"},
		{in: "x.0", wantErr: true, desc: "bad major"},
		{in: "1.y", wantErr: true, desc: "bad minor"},
		{in: "-1.0", wantErr: true, desc: "negative major"},
	}
	for _, c := range cases {
		t.Run(c.desc, func(t *testing.T) {
			maj, minor, err := ParseVersion(c.in)
			if c.wantErr {
				if err == nil {
					t.Fatalf("ParseVersion(%q): want error, got %d.%d", c.in, maj, minor)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseVersion(%q): unexpected error: %v", c.in, err)
			}
			if maj != c.major || minor != c.minor {
				t.Fatalf("ParseVersion(%q) = %d.%d, want %d.%d", c.in, maj, minor, c.major, c.minor)
			}
		})
	}
}

func TestCompatible(t *testing.T) {
	cases := []struct {
		client string
		daemon string
		ok     bool
		desc   string
	}{
		{client: "1.0", daemon: "1.0", ok: true, desc: "equal"},
		{client: "1.0", daemon: "1.3", ok: true, desc: "daemon minor newer ok"},
		{client: "1.2", daemon: "1.0", ok: false, desc: "client minor newer fails closed"},
		{client: "10.0", daemon: "1.0", ok: false, desc: "major mismatch (not first-byte equal)"},
		{client: "1.0", daemon: "10.0", ok: false, desc: "major mismatch reverse"},
		{client: "2.0", daemon: "1.0", ok: false, desc: "major mismatch"},
		{client: "", daemon: "1.0", ok: false, desc: "empty client fails closed"},
		{client: "1.0", daemon: "", ok: false, desc: "empty daemon fails closed"},
		{client: "bad", daemon: "1.0", ok: false, desc: "unparseable client"},
	}
	for _, c := range cases {
		t.Run(c.desc, func(t *testing.T) {
			err := Compatible(c.client, c.daemon)
			if c.ok && err != nil {
				t.Fatalf("Compatible(%q,%q): want ok, got %v", c.client, c.daemon, err)
			}
			if !c.ok && err == nil {
				t.Fatalf("Compatible(%q,%q): want error, got nil", c.client, c.daemon)
			}
		})
	}
}
