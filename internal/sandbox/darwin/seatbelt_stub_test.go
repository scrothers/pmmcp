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

package darwin_test

import (
	"testing"

	"github.com/scrothers/pmmcp/internal/sandbox"
	"github.com/scrothers/pmmcp/internal/sandbox/darwin"
)

// TestTrySandboxExecUnavailableOffDarwin covers the !darwin stub, which never
// has a real sandbox-exec binary to rewrite argv for.
func TestTrySandboxExecUnavailableOffDarwin(t *testing.T) {
	t.Parallel()
	argv, ok := darwin.TrySandboxExec([]string{"echo", "hi"}, "/tmp", sandbox.Policy{})
	if ok || argv != nil {
		t.Fatalf("TrySandboxExec() = (%v, %v), want (nil, false)", argv, ok)
	}
}

// TestSandboxExecAvailableOffDarwin covers the !darwin stub's constant false.
func TestSandboxExecAvailableOffDarwin(t *testing.T) {
	t.Parallel()
	if darwin.SandboxExecAvailable() {
		t.Fatal("SandboxExecAvailable() = true, want false off darwin")
	}
}
