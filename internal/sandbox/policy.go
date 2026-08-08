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

package sandbox

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Profile is a named sandbox posture.
type Profile string

// Well-known profiles. Strict is the OSS default.
const (
	Strict     Profile = "strict"
	Standard   Profile = "standard"
	Permissive Profile = "permissive"
	Off        Profile = "off"
)

// Mode strings returned by platform Apply helpers.
const (
	ModePolicy              = "policy"
	ModeOff                 = "off"
	ModeLandlock            = "landlock"
	ModeBwrap               = "bwrap"
	ModeContainerSubstitute = "container-substitute"
)

// ErrUnknownProfile is returned for profiles outside the known set.
var ErrUnknownProfile = errors.New("sandbox: unknown profile")

// ErrProjectRootRequired is returned when strict/standard need a project root
// but none is configured on the policy.
var ErrProjectRootRequired = errors.New("sandbox: project root required")

// ErrStrictUnsupported is returned when strict is requested on a platform that
// has no filesystem-isolation mechanism in this MVP (Windows), so it fails
// closed rather than running unconfined. Use a container runtime, or
// relax the profile, to run such workloads on that platform.
var ErrStrictUnsupported = errors.New("sandbox: strict unsupported on this platform without a container runtime")

// knownProfiles is the fail-closed allowlist of Profile values.
var knownProfiles = map[Profile]struct{}{
	Strict:     {},
	Standard:   {},
	Permissive: {},
	Off:        {},
}

// Valid reports whether p is a known profile.
func Valid(p Profile) bool {
	_, ok := knownProfiles[p]
	return ok
}

// Policy is the platform-agnostic sandbox intent for a process start.
type Policy struct {
	// Profile is the named posture.
	Profile Profile
	// WritableRoots are absolute paths allowed for open-write.
	WritableRoots []string
	// ReadDeny are path prefixes / markers always denied for open-read under
	// restrictive profiles (e.g. home.ssh trees).
	ReadDeny []string
}

// DefaultPolicy builds a Policy for profile rooted at projectRoot.
// Unknown profiles return ErrUnknownProfile.
// projectRoot should be absolute; empty is allowed only for Off (and yields
// temp-only writable roots for other profiles until Apply rejects strict).
func DefaultPolicy(profile Profile, projectRoot string) (Policy, error) {
	if !Valid(profile) {
		return Policy{}, fmt.Errorf("%w: %q", ErrUnknownProfile, profile)
	}

	projectRoot = strings.TrimSpace(projectRoot)
	if projectRoot != "" {
		projectRoot = filepath.Clean(projectRoot)
	}
	temp := filepath.Clean(os.TempDir())

	switch profile {
	case Off:
		return Policy{Profile: Off}, nil

	case Strict:
		return Policy{
			Profile:       Strict,
			WritableRoots: writableRoots(projectRoot, temp),
			ReadDeny:      defaultReadDeny(),
		}, nil

	case Standard:
		return Policy{
			Profile:       Standard,
			WritableRoots: writableRoots(projectRoot, temp),
			ReadDeny:      defaultReadDeny(),
		}, nil

	case Permissive:
		roots := writableRoots(projectRoot, temp)
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			home = filepath.Clean(home)
			if !containsPath(roots, home) {
				roots = append(roots, home)
			}
		}
		return Policy{
			Profile:       Permissive,
			WritableRoots: roots,
			ReadDeny:      nil,
		}, nil

	default:
		// Unreachable when Valid is maintained; keep fail-closed.
		return Policy{}, fmt.Errorf("%w: %q", ErrUnknownProfile, profile)
	}
}

// AllowsRead reports whether path may be opened for reading under this policy.
// Fail-closed: unknown profiles and empty profile deny.
func (p Policy) AllowsRead(path string) bool {
	if !Valid(p.Profile) {
		return false
	}
	if p.Profile == Off {
		return true
	}
	path = normalizePath(path)
	if path == "" {
		return false
	}
	// Restrictive profiles always block secret path classes (e.g. ~/.ssh).
	if p.Profile == Strict || p.Profile == Standard {
		if isSSHStylePath(path) {
			return false
		}
	}
	for _, deny := range p.ReadDeny {
		if pathMatchesDeny(path, deny) {
			return false
		}
	}
	return true
}

// AllowsWrite reports whether path may be opened for writing under this policy.
// Fail-closed: unknown profiles deny; Off allows; others require a writable root.
func (p Policy) AllowsWrite(path string) bool {
	if !Valid(p.Profile) {
		return false
	}
	if p.Profile == Off {
		return true
	}
	path = normalizePath(path)
	if path == "" {
		return false
	}
	// Writes inherit read denials (never write under.ssh in strict/standard).
	if !p.AllowsRead(path) {
		return false
	}
	for _, root := range p.WritableRoots {
		if root == "" {
			continue
		}
		if isUnder(path, normalizePath(root)) {
			return true
		}
	}
	return false
}

// HasProjectRoot reports whether the policy has a non-temp writable root
// suitable as a project root for strict/standard Apply.
func (p Policy) HasProjectRoot() bool {
	temp := filepath.Clean(os.TempDir())
	for _, r := range p.WritableRoots {
		r = strings.TrimSpace(r)
		if r == "" {
			continue
		}
		if filepath.Clean(r) != temp {
			return true
		}
	}
	return false
}

func writableRoots(projectRoot, temp string) []string {
	roots := make([]string, 0, 2)
	if projectRoot != "" {
		roots = append(roots, projectRoot)
	}
	if temp != "" && !containsPath(roots, temp) {
		roots = append(roots, temp)
	}
	return roots
}

func defaultReadDeny() []string {
	// Portable markers checked via pathMatchesDeny / isSSHStylePath.
	denies := []string{
		"/.ssh/",
		"/.ssh",
		"/.gnupg/",
		"/.aws/",
		"/docker.sock",
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		denies = append(denies, filepath.Join(home, ".ssh"))
	}
	return denies
}

func containsPath(roots []string, p string) bool {
	p = filepath.Clean(p)
	for _, r := range roots {
		if filepath.Clean(r) == p {
			return true
		}
	}
	return false
}

func normalizePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	return filepath.Clean(path)
}

// isSSHStylePath detects home SSH key paths across OS separators.
// Matches paths containing "/.ssh/" or ending with "/.ssh" or "/.ssh/id_*".
// Both slash styles are normalized so policy is portable under cross-OS tests.
func isSSHStylePath(path string) bool {
	n := slashy(path)
	if strings.Contains(n, "/.ssh/") {
		return true
	}
	if strings.HasSuffix(n, "/.ssh") {
		return true
	}
	// Bare relative ".ssh/id_rsa".
	if strings.HasPrefix(n, ".ssh/") || n == ".ssh" {
		return true
	}
	return false
}

// slashy maps any path separator to '/' for portable policy checks.
func slashy(path string) string {
	n := filepath.ToSlash(path)
	// filepath.ToSlash only rewrites the host separator; also fold '\' so
	// Windows-style paths match when evaluated on Unix (and vice versa).
	return strings.ReplaceAll(n, "\\", "/")
}

func pathMatchesDeny(path, deny string) bool {
	deny = strings.TrimSpace(deny)
	if deny == "" {
		return false
	}
	nPath := slashy(path)
	nDeny := slashy(deny)
	// Marker-style denials (e.g. "/.ssh/", "/docker.sock") use contains/suffix.
	if strings.Contains(nDeny, "/.") || strings.HasSuffix(nDeny, ".sock") || !filepath.IsAbs(deny) {
		if strings.Contains(nPath, nDeny) || strings.HasSuffix(nPath, nDeny) {
			return true
		}
	}
	// Absolute prefix deny (e.g. /home/u/.ssh).
	d := filepath.Clean(deny)
	if isUnder(path, d) || path == d {
		return true
	}
	return false
}

func isUnder(path, root string) bool {
	if root == "" || path == "" {
		return false
	}
	path = filepath.Clean(path)
	root = filepath.Clean(root)
	if path == root {
		return true
	}
	sep := string(filepath.Separator)
	prefix := root
	if !strings.HasSuffix(prefix, sep) {
		prefix += sep
	}
	return strings.HasPrefix(path, prefix)
}
