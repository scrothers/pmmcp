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

// Package local implements process.Manager for host OS processes.
// Children start via exec with argv only (no shell). Unix uses a new process group for tree-kill
// (SIGTERM then SIGKILL after grace). Windows uses Job Objects when assigned (kill-on-close).
// Strict/standard sandbox wraps apply platform isolation (bwrap, seatbelt, job) and fail closed
// when the mechanism is missing. Optional logcap and memory rlimits attach when configured.
package local
