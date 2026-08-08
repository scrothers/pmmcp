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

package watch

import (
	"testing"
	"time"
)

// TestTickNoOpWhenClosed covers tick's closed-guard. Reaching this branch
// through the public API would require a timing race between Close setting
// closed=true and a concurrently firing poll tick, so it is exercised
// directly here instead.
func TestTickNoOpWhenClosed(t *testing.T) {
	t.Parallel()
	w := New()
	w.closed = true
	// Must not panic or touch w.paths/w.pending/w.events; a nil map write
	// here would still panic even on the closed path bug, so this doubles
	// as a straightforward regression guard.
	w.tick(time.Now())
}
