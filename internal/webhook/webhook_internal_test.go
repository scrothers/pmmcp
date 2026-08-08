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

package webhook

import (
	"context"
	"errors"
	"net"
	"testing"
)

func TestAdmitParseError(t *testing.T) {
	t.Parallel()
	if err := admit("", []string{"example.com"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("admit(\"\") = %v, want ErrInvalid", err)
	}
}

func TestAdmitAllowlistRejectsNonEmpty(t *testing.T) {
	t.Parallel()
	err := admit("https://evil.com/x", []string{"other.example.com"})
	if !errors.Is(err, ErrSSRF) {
		t.Fatalf("admit(evil.com) = %v, want ErrSSRF", err)
	}
}

func TestAllowlistedSkipsEmptyPattern(t *testing.T) {
	t.Parallel()
	u, err := parseHTTPURL("https://example.com/x")
	if err != nil {
		t.Fatalf("parseHTTPURL: %v", err)
	}
	if !allowlisted("https://example.com/x", u, []string{"", "example.com"}) {
		t.Fatal("allowlisted should match after skipping the empty pattern")
	}
}

func TestParseHTTPURLEmptyString(t *testing.T) {
	t.Parallel()
	if _, err := parseHTTPURL(""); !errors.Is(err, ErrInvalid) {
		t.Fatalf("parseHTTPURL(\"\") = %v, want ErrInvalid", err)
	}
}

func TestParseHTTPURLParseError(t *testing.T) {
	t.Parallel()
	// Unterminated IPv6 bracket makes url.Parse itself fail.
	if _, err := parseHTTPURL("http://[::1"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("parseHTTPURL(bad bracket) = %v, want ErrInvalid", err)
	}
}

func TestParseHTTPURLEmptyHost(t *testing.T) {
	t.Parallel()
	if _, err := parseHTTPURL("http:///path"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("parseHTTPURL(empty host) = %v, want ErrInvalid", err)
	}
}

func TestIsBlockedHostnameLocalhostSuffix(t *testing.T) {
	t.Parallel()
	if !isBlockedHostname("foo.localhost") {
		t.Fatal("foo.localhost should be blocked")
	}
}

func TestIsBlockedIPTable(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		ip   net.IP
		want bool
	}{
		{"nil", nil, true},
		{"loopback v4", net.ParseIP("127.0.0.1"), true},
		{"loopback v6", net.ParseIP("::1"), true},
		{"unspecified v4", net.ParseIP("0.0.0.0"), true},
		{"unspecified v6", net.ParseIP("::"), true},
		{"link-local unicast", net.ParseIP("169.254.1.1"), true},
		{"link-local multicast", net.ParseIP("224.0.0.1"), true},
		{"metadata", net.ParseIP("169.254.169.254"), true},
		{"public", net.ParseIP("93.184.216.34"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := isBlockedIP(tt.ip); got != tt.want {
				t.Fatalf("isBlockedIP(%v) = %v, want %v", tt.ip, got, tt.want)
			}
		})
	}
}

func TestValidateURLContextLiteralIPEarlyReturn(t *testing.T) {
	t.Parallel()
	// A public IP literal in the raw URL: validateHost already checked it, so
	// ValidateURLContext must return before ever attempting DNS resolution.
	if err := ValidateURLContext(context.Background(), "http://93.184.216.34/x"); err != nil {
		t.Fatalf("ValidateURLContext(literal public IP) = %v, want nil", err)
	}
}

func TestValidateURLContextResolverFailureTolerated(t *testing.T) {
	t.Parallel()
	lookupErr := errors.New("injected SERVFAIL")
	fakeLookup := func(_ context.Context, _ string) ([]net.IPAddr, error) {
		return nil, lookupErr
	}
	if err := validateURLContext(context.Background(), "https://example.com/hook", fakeLookup); err != nil {
		t.Fatalf("validateURLContext(resolver failure) = %v, want nil (tolerated)", err)
	}
}

func TestValidateURLContextResolvedBlockedIP(t *testing.T) {
	t.Parallel()
	fakeLookup := func(_ context.Context, _ string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
	}
	err := validateURLContext(context.Background(), "https://example.com/hook", fakeLookup)
	if !errors.Is(err, ErrSSRF) {
		t.Fatalf("validateURLContext(resolved blocked IP) = %v, want ErrSSRF", err)
	}
}

func TestValidateURLContextResolvedAllowedIP(t *testing.T) {
	t.Parallel()
	fakeLookup := func(_ context.Context, _ string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
	}
	if err := validateURLContext(context.Background(), "https://example.com/hook", fakeLookup); err != nil {
		t.Fatalf("validateURLContext(resolved allowed IP) = %v, want nil", err)
	}
}

func TestRegistryListEmpty(t *testing.T) {
	t.Parallel()
	r := NewRegistry(WithAllowlist("*.example.com"))
	if got := r.List(); len(got) != 0 {
		t.Fatalf("List() on fresh registry = %v, want empty", got)
	}
}

func TestRegistryListSortsByID(t *testing.T) {
	t.Parallel()
	r := NewRegistry(WithAllowlist("*.example.com"))
	for _, id := range []string{"hook-b", "hook-a", "hook-c"} {
		if err := r.Create(Hook{ID: id, URL: "https://example.com/hook"}); err != nil {
			t.Fatalf("Create(%s): %v", id, err)
		}
	}
	list := r.List()
	if len(list) != 3 {
		t.Fatalf("List() = %d hooks, want 3", len(list))
	}
	for i := 1; i < len(list); i++ {
		if list[i-1].ID >= list[i].ID {
			t.Fatalf("List() not sorted: %v", list)
		}
	}
}

func TestRegistryGetNotFound(t *testing.T) {
	t.Parallel()
	r := NewRegistry(WithAllowlist("*.example.com"))
	if _, err := r.Get("missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get(missing) = %v, want ErrNotFound", err)
	}
}

func TestRegistryCreateEmptyID(t *testing.T) {
	t.Parallel()
	r := NewRegistry(WithAllowlist("*.example.com"))
	err := r.Create(Hook{URL: "https://example.com/hook"})
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("Create(empty id) = %v, want ErrInvalid", err)
	}
}
