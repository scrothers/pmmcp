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

//go:build linux

package local

import (
	"errors"
	"strings"
	"testing"

	"github.com/scrothers/pmmcp/internal/process"
)

// TestWrapSandboxRejectsUnknownProfile covers the policy-construction failure
// arm. Start only ever passes "strict" or "standard", so this arm is reachable
// only by calling wrapSandbox directly; it exists to keep the wrapper fail-closed
// if a new caller ever hands it an unvalidated profile name.
func TestWrapSandboxRejectsUnknownProfile(t *testing.T) {
	t.Parallel()
	argv, err := wrapSandbox([]string{"/bin/true"}, t.TempDir(), "bogus-profile")
	if argv != nil {
		t.Fatalf("argv = %v, want nil", argv)
	}
	if !errors.Is(err, process.ErrSandboxFailed) {
		t.Fatalf("err = %v, want ErrSandboxFailed", err)
	}
	if !strings.Contains(err.Error(), "unknown profile") {
		t.Fatalf("err = %v, want the underlying unknown-profile cause", err)
	}
}
