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

package config

// SanitizePipeUserForTest exposes sanitizePipeUser so tests can exercise its
// empty-input fallback, which the public API never triggers (callers always
// supply a non-empty default before calling it).
func SanitizePipeUserForTest(s string) string {
	return sanitizePipeUser(s)
}

// ValidateForTest exposes validate so tests can exercise the state_dir/
// ipc.endpoint empty checks directly. Load always fills both via
// normalizeAndDefault before calling validate, so the public API alone
// cannot reach these branches.
func (c *Config) ValidateForTest() error {
	return c.validate()
}
