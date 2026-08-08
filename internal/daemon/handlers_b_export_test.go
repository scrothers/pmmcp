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

package daemon

import "github.com/scrothers/pmmcp/internal/store"

// SetStoreForTest swaps the process store after construction, for
// fault-injection tests that need store.List/Get to fail (declare.diff and
// declare.apply's store-read error paths, which cannot be reached through the
// public IPC surface with a healthy store).
func SetStoreForTest(s *Server, st store.ProcessStore) {
	s.store = st
}

// KeyringForTest exposes the daemon's file-backed keyring so tests can write
// a value directly (bypassing secret.set, which also registers an s.secrets
// path entry) to exercise doSecretRefCheck's keyring-only lookup branch.
func KeyringForTest(s *Server) interface {
	Set(name, value string) (string, error)
	Get(name string) (string, error)
} {
	return s.keyring
}
