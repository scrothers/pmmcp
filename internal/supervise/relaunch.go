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

package supervise

import "github.com/scrothers/pmmcp/internal/domain"

// EligibleForRelaunch reports whether a stored process should be started on daemon boot.
// Desired must be running; policy.Enabled gates optional auto-restart semantics.
func EligibleForRelaunch(desired domain.Desired, policy RestartPolicy) bool {
	if desired != domain.DesiredRunning {
		return false
	}
	// Boot relaunch follows durable desired state; RestartPolicy.Enabled is for crash loops.
	// Relaunch eligible whenever desired=running (caller may further filter by persist flags).
	_ = policy
	return true
}
