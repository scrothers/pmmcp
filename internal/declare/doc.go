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

// Package declare parses, validates, diffs, and imports project declarations (pmmcp.yaml, Procfile).
// Parse unmarshals YAML into Document; ParseStrict rejects unknown fields. Validate runs structural
// checks plus a deny-by-default security policy (sandbox off, shell-risk argv, privileged ports,
// watch path containment, webhook SSRF) with optional relaxations via ValidateOption.
//
// DiffServices compares declared names to running names. ImportProcfile builds a draft Document.
// This package does not talk to the daemon or start processes.
package declare
