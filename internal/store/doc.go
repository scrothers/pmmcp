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

// Package store defines durable repository interfaces for process state.
// Only the daemon opens concrete drivers (store/sqlite). CLI and MCP clients never open the
// database file; they call the daemon over IPC. ProcessStore covers create, get, list, update,
// delete, and compare-and-swap style updates.
package store
