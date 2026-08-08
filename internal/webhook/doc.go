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

// Package webhook registers outbound hooks and delivers JSON POSTs under SSRF-safe policy.
// ValidateURL and delivery refuse non-http(s), loopback, link-local, and cloud metadata targets
// unless explicitly allowed. An empty allowlist can mean webhooks disabled depending on caller
// policy. Registry is in-memory; the daemon wires dispatch from domain events.
package webhook
