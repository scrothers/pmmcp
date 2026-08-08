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

// Package watch implements debounced filesystem watches for hot reload.
// The default backend is mtime polling so unit tests stay hermetic. Debounce coalesces burst
// writes into one Event. Missing paths are skipped rather than storming errors. Close stops the
// loop without leaking goroutines. Path policy (project containment) is enforced by declare/daemon,
// not this package.
package watch
