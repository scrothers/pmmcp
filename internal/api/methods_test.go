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

package api_test

import (
	"testing"

	"github.com/scrothers/pmmcp/internal/api"
)

func TestAllMethodsNonEmptyUnique(t *testing.T) {
	t.Parallel()
	if len(api.AllMethods) < 50 {
		t.Fatalf("AllMethods too small: %d", len(api.AllMethods))
	}
	seen := map[string]bool{}
	for _, m := range api.AllMethods {
		if m == "" {
			t.Fatal("empty method")
		}
		if seen[m] {
			t.Fatalf("duplicate %s", m)
		}
		seen[m] = true
	}
}
