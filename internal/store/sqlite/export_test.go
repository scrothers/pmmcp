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

package sqlite

import "database/sql"

// NewForTest constructs a Store around an arbitrary *sql.DB, letting tests
// exercise Store methods against a fault-injecting driver that a real,
// on-disk sqlite.Open cannot be made to fail deterministically (e.g. a
// mid-migration BeginTx or Commit error).
func NewForTest(db *sql.DB) *Store {
	return &Store{db: db}
}

// IsUnique exposes isUnique for direct unit testing of its fallback,
// message-matching branch, which real constraint violations never take
// (modernc always returns a typed *sqlite.Error for those).
var IsUnique = isUnique
