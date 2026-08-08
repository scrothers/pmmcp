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

// Package audit appends and queries control-plane audit records.
// Records capture who asked for a mutating or sensitive action (session, actor, action, target,
// detail) without secret values. Backends may be in-memory rings or SQLite-backed logs opened by
// the daemon. This package does not authorize; callers check authz then Append on success or deny.
package audit
