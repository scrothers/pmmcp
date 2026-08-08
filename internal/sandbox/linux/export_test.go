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

package linux

// Test seams for unexported helpers. Only compiled into the package's own test
// binary, so the production surface stays unchanged.
var (
	// IsUnderPath exposes isUnderPath for path-containment table tests.
	IsUnderPath = isUnderPath
	// DenyMountPaths exposes denyMountPaths for tmpfs-overlay table tests.
	DenyMountPaths = denyMountPaths
)
