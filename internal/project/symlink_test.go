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

package project_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/scrothers/pmmcp/internal/project"
)

func TestKeyResolvesSymlink(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	realRoot := filepath.Join(dir, "real")
	if err := os.MkdirAll(realRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(realRoot, link); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}
	k1 := project.Key(realRoot)
	k2 := project.Key(link)
	if k1 != k2 {
		t.Fatalf("Key(real)=%q Key(link)=%q", k1, k2)
	}
}
