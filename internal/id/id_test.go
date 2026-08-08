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

package id_test

import (
	"crypto/rand"
	"errors"
	"strings"
	"testing"

	"github.com/scrothers/pmmcp/internal/id"
)

func TestNewAndParseRoundTrip(t *testing.T) {
	t.Parallel()
	prefixes := []id.Prefix{id.Proc, id.Group, id.Profile, id.Event, id.Audit, id.Session, id.Project}
	for _, p := range prefixes {
		p := p
		t.Run(string(p), func(t *testing.T) {
			t.Parallel()
			got, err := id.New(p)
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			if !strings.HasPrefix(got, string(p)+"-") {
				t.Fatalf("id %q missing prefix %s-", got, p)
			}
			body := strings.TrimPrefix(got, string(p)+"-")
			if len(body) != 26 {
				t.Fatalf("body len = %d, want 26", len(body))
			}
			// Crockford base32 is case-insensitive; we emit lower.
			if body != strings.ToLower(body) {
				t.Fatalf("body not lowercase: %q", body)
			}
			gotP, _, err := id.Parse(got)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if gotP != p {
				t.Fatalf("prefix = %q, want %q", gotP, p)
			}
			if !id.Valid(got) {
				t.Fatal("Valid = false")
			}
			if !id.HasPrefix(got, p) {
				t.Fatal("HasPrefix = false")
			}
		})
	}
}

func TestNewUnknownPrefix(t *testing.T) {
	t.Parallel()
	_, err := id.New("nope")
	if !errors.Is(err, id.ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid", err)
	}
}

func TestParseRejects(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   string
	}{
		{"empty", ""},
		{"whitespace", "   "},
		{"no_hyphen", "proc01ARZ3NDEKTSV4RRFFQ69G5FAV"},
		{"wrong_prefix", "foo-01ARZ3NDEKTSV4RRFFQ69G5FAV"},
		{"short_body", "proc-01ARZ3NDEKTSV4RRFFQ69G5FA"},
		{"long_body", "proc-01ARZ3NDEKTSV4RRFFQ69G5FAVX"},
		{"bad_charset", "proc-!!!!!!!!!!!!!!!!!!!!!!!!!!"},
		{"uuid_shape", "proc-550e8400-e29b-41d4-a716-446655440000"},
		{"missing_body", "proc-"},
		{"missing_prefix", "-01ARZ3NDEKTSV4RRFFQ69G5FAV"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, _, err := id.Parse(tc.in)
			if !errors.Is(err, id.ErrInvalid) {
				t.Fatalf("Parse(%q) err = %v, want ErrInvalid", tc.in, err)
			}
			if id.Valid(tc.in) {
				t.Fatalf("Valid(%q) = true", tc.in)
			}
		})
	}
}

func TestParseRejectsNonCanonical(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   string
	}{
		{"upper_prefix", "PROC-01arz3ndektsv4rrffq69g5fav"},
		{"upper_body", "proc-01ARZ3NDEKTSV4RRFFQ69G5FAV"},
		{"mixed_body", "proc-01ArZ3ndektsv4rrffq69g5fav"},
		{"leading_space", " proc-01arz3ndektsv4rrffq69g5fav"},
		{"trailing_space", "proc-01arz3ndektsv4rrffq69g5fav "},
		{"inner_space", "proc-01arz3ndektsv4rrffq6 g5fav"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, _, err := id.Parse(tc.in); !errors.Is(err, id.ErrInvalid) {
				t.Fatalf("Parse(%q) err = %v, want ErrInvalid", tc.in, err)
			}
			if id.Valid(tc.in) {
				t.Fatalf("Valid(%q) = true, want false", tc.in)
			}
		})
	}
}

func TestNewUnique(t *testing.T) {
	t.Parallel()
	const n = 1000
	seen := make(map[string]struct{}, n)
	for range n {
		s, err := id.New(id.Proc)
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		if _, dup := seen[s]; dup {
			t.Fatalf("duplicate id %q", s)
		}
		seen[s] = struct{}{}
	}
}

// failingReader is an io.Reader that always errors, used to force New's
// entropy-read failure path.
type failingReader struct{}

func (failingReader) Read([]byte) (int, error) {
	return 0, errors.New("failingReader: no entropy")
}

// TestNewEntropyFailure swaps the shared crypto/rand.Reader for one that
// always errors, so it must not run in parallel with tests that call New.
func TestNewEntropyFailure(t *testing.T) {
	orig := rand.Reader
	rand.Reader = failingReader{}
	t.Cleanup(func() { rand.Reader = orig })

	_, err := id.New(id.Proc)
	if err == nil {
		t.Fatal("New: err = nil, want entropy failure")
	}
	if !strings.Contains(err.Error(), "generate") {
		t.Fatalf("err = %v, want it to mention generate", err)
	}
}

func TestHasPrefixMismatch(t *testing.T) {
	t.Parallel()
	s, err := id.New(id.Proc)
	if err != nil {
		t.Fatal(err)
	}
	if id.HasPrefix(s, id.Group) {
		t.Fatal("proc id should not match grp prefix")
	}
}
