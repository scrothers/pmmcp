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

// Package version holds build-time version metadata for pmmcp binaries.
package version

// Version is the semantic version string. Overridden at link time with -ldflags.
var Version = "0.0.0-dev"

// Commit is the git commit, if set via -ldflags.
var Commit = "unknown"

// BuildDate is the build timestamp, if set via -ldflags.
var BuildDate = "unknown"

// String returns a human-readable version line.
func String() string {
	return Version + " (commit=" + Commit + " date=" + BuildDate + ")"
}
