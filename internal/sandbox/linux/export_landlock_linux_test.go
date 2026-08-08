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

package linux

import "github.com/scrothers/pmmcp/internal/sandbox"

// Landlock test seams. Compiled only into the package's own test binary.
var (
	// LandlockABIFlags exposes landlockABIFlags so the query-rejected branch
	// can be driven with a flag combination the kernel must refuse.
	LandlockABIFlags = landlockABIFlags
	// LandlockCreateRuleset exposes landlockCreateRuleset so ruleset creation
	// can be exercised for any ABI without ever restricting a thread.
	LandlockCreateRuleset = landlockCreateRuleset
	// LandlockSysRoots is the production read-only system root list.
	LandlockSysRoots = landlockSysRoots
)

// AccessFSRead is the read-oriented right set granted under allowed trees.
const AccessFSRead = accessFSRead

// AccessFSWrite is the read+write right set granted under writable roots.
const AccessFSWrite = accessFSWrite

// LandlockAddPath exposes landlockAddPath. Adding a rule never restricts the
// calling thread, so it is safe to exercise in the main test process.
func LandlockAddPath(rulesetFD int, path string, access uint64) error {
	return landlockAddPath(rulesetFD, path, access)
}

// LandlockRestrictPathsABI exposes landlockRestrictPaths with the kernel ABI
// and system roots injected. Any call that reaches landlock_restrict_self
// permanently restricts the calling OS thread, so callers must run it in a
// throwaway helper process.
func LandlockRestrictPathsABI(abi int, sysRoots []string, pol sandbox.Policy) error {
	return landlockRestrictPaths(abi, sysRoots, pol)
}
