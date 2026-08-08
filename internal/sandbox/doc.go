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

// Package sandbox defines sandbox profiles and platform-agnostic path policy.
// Profiles express intent: strict, standard, permissive, and off. Platform packages under
// sandbox/linux, sandbox/darwin, and sandbox/windows apply that intent. Unknown profiles are
// rejected. Strict refuses enforcement claims without a project root (never a silent open host).
//
// Policy helpers (AllowsRead/AllowsWrite) back fail-closed checks; kernel mechanisms (Landlock,
// seatbelt, Job Objects) and bubblewrap strengthen children without changing the Policy API.
package sandbox
