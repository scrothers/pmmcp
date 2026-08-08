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

package api

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/scrothers/pmmcp/internal/domain"
)

// ParseVersion parses a "major.minor" API version string into its numeric
// components. Both parts are required and must be non-negative integers;
// anything else is a parse error ( requires a real numeric compare, not
// a byte-slice).
func ParseVersion(v string) (major, minor int, err error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0, 0, fmt.Errorf("api: parse version: empty")
	}
	parts := strings.Split(v, ".")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("api: parse version %q: want major.minor", v)
	}
	major, err = strconv.Atoi(parts[0])
	if err != nil || major < 0 {
		return 0, 0, fmt.Errorf("api: parse version %q: bad major", v)
	}
	minor, err = strconv.Atoi(parts[1])
	if err != nil || minor < 0 {
		return 0, 0, fmt.Errorf("api: parse version %q: bad minor", v)
	}
	return major, minor, nil
}

// Compatible reports whether a client speaking version client can talk to a
// daemon speaking version daemon, per: fail closed if the major
// versions differ, or if the client requires a minor newer than the daemon's.
// A nil return means compatible; a non-nil return is a domain error with code
// ipc_version_mismatch and both versions in the message.
func Compatible(client, daemon string) error {
	cMajor, cMinor, err := ParseVersion(client)
	if err != nil {
		return domain.NewError(domain.CodeIPCVersionMismatch,
			fmt.Sprintf("unparseable client version %q (daemon %s)", client, daemon), false)
	}
	dMajor, dMinor, err := ParseVersion(daemon)
	if err != nil {
		return domain.NewError(domain.CodeIPCVersionMismatch,
			fmt.Sprintf("unparseable daemon version %q (client %s)", daemon, client), false)
	}
	if cMajor != dMajor {
		return domain.NewError(domain.CodeIPCVersionMismatch,
			fmt.Sprintf("major version mismatch: client %s daemon %s", client, daemon), false)
	}
	if cMinor > dMinor {
		return domain.NewError(domain.CodeIPCVersionMismatch,
			fmt.Sprintf("client minor newer than daemon: client %s daemon %s", client, daemon), false)
	}
	return nil
}
