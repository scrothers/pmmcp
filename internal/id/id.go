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

// Package id generates and parses prefixed Crockford ULID identifiers.
package id

import (
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"
)

// Prefix is a resource type prefix for durable IDs (without trailing hyphen).
type Prefix string

// Well-known prefixes.
const (
	Proc    Prefix = "proc"
	Group   Prefix = "grp"
	Profile Prefix = "prof"
	Event   Prefix = "evt"
	Audit   Prefix = "aud"
	Session Prefix = "sess"
	Project Prefix = "proj"
)

// ErrInvalid is returned when an ID string cannot be parsed or validated.
var ErrInvalid = errors.New("id: invalid")

// known lists all prefixes accepted by Parse/Valid.
var known = map[Prefix]struct{}{
	Proc:    {},
	Group:   {},
	Profile: {},
	Event:   {},
	Audit:   {},
	Session: {},
	Project: {},
}

// New generates a new prefixed ULID for the given resource type.
func New(prefix Prefix) (string, error) {
	if _, ok := known[prefix]; !ok {
		return "", fmt.Errorf("%w: unknown prefix %q", ErrInvalid, prefix)
	}
	u, err := ulid.New(ulid.Timestamp(time.Now().UTC()), rand.Reader)
	if err != nil {
		return "", fmt.Errorf("id: generate: %w", err)
	}
	return string(prefix) + "-" + strings.ToLower(u.String()), nil
}

// Parse splits and validates a prefixed ID. It returns the prefix and ULID body.
//
// Input must already be in canonical form: a lowercase prefix and lowercase body
// with no surrounding whitespace. Non-canonical input (uppercase,
// padded) is rejected rather than silently accepted, so any string that passes
// Parse/Valid also matches exact-string lookups (map keys, SQLite) downstream.
func Parse(s string) (Prefix, ulid.ULID, error) {
	var zero ulid.ULID
	if s == "" {
		return "", zero, fmt.Errorf("%w: empty", ErrInvalid)
	}
	prefix, body, ok := strings.Cut(s, "-")
	if !ok || prefix == "" || body == "" {
		return "", zero, fmt.Errorf("%w: missing prefix separator", ErrInvalid)
	}
	// Body may contain hyphens only if we wrongly split; Crockford ULID is 26 chars with no hyphens.
	// If more hyphens appear (e.g. proc-foo-bar), Cut took first only — reject non-26 body.
	if strings.Contains(body, "-") {
		return "", zero, fmt.Errorf("%w: malformed body", ErrInvalid)
	}
	if prefix != strings.ToLower(prefix) {
		return "", zero, fmt.Errorf("%w: non-canonical prefix %q (must be lowercase)", ErrInvalid, prefix)
	}
	if body != strings.ToLower(body) {
		return "", zero, fmt.Errorf("%w: non-canonical body (must be lowercase)", ErrInvalid)
	}
	p := Prefix(prefix)
	if _, ok := known[p]; !ok {
		return "", zero, fmt.Errorf("%w: unknown prefix %q", ErrInvalid, prefix)
	}
	if len(body) != 26 {
		return "", zero, fmt.Errorf("%w: ulid body length", ErrInvalid)
	}
	u, err := ulid.ParseStrict(strings.ToUpper(body))
	if err != nil {
		return "", zero, fmt.Errorf("%w: ulid: %w", ErrInvalid, err)
	}
	return p, u, nil
}

// Valid reports whether s is a well-formed prefixed ULID with a known prefix.
func Valid(s string) bool {
	_, _, err := Parse(s)
	return err == nil
}

// HasPrefix reports whether s is a valid ID with the given prefix.
func HasPrefix(s string, want Prefix) bool {
	got, _, err := Parse(s)
	return err == nil && got == want
}
