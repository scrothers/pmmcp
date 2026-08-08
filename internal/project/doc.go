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

// Package project detects project roots and stable project keys from a working directory.
// Detection walks parents for pmmcp.yaml/yml then .git (directory or worktree file). Key
// canonicalizes via EvalSymlinks when possible so the same tree maps to one key. Callers supply
// cwd explicitly; this package does not read process-global env for project selection.
package project
