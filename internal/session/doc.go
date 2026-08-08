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

// Package session tracks client connection sessions and harness identities.
// Open assigns sess- ULIDs and may reuse a session when a harness ID is presented. End removes
// session state for disconnect cleanup. The package does not authorize capabilities; authz and the
// daemon interpret session IDs for audit attribution.
package session
