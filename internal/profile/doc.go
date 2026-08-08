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

// Package profile is an in-memory registry of named profiles under a project.
// Profiles carry env overlays and defaults (sandbox posture, restart policy) but are not a security
// boundary. The name default is used when none is selected. Use/Active track per-session selection.
// IDs use the prof- prefix. Durable persistence may wrap Store later.
package profile
