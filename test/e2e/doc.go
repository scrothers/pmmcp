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

// Package e2e contains end-to-end tests that boot real pmmcp binaries and exercise MCP/CLI
// flows against a live daemon.
//
// Build with -tags=e2e. Tests must not run under the default go test ./... gate. Prefer
// PMMCP_REQUIRE_SANDBOX=1 on platforms that provide isolation.
package e2e
