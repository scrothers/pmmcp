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

package cli

import (
	"context"
	"os"
	"testing"
)

// TestExecuteRunsTree covers Execute, the os.Args-driven entry point (other tests
// drive Run with explicit args).
func TestExecuteRunsTree(t *testing.T) {
	origArgs := os.Args
	t.Cleanup(func() { os.Args = origArgs })
	os.Args = []string{"pmmcp", "version"}
	if err := Execute(context.Background()); err != nil {
		t.Fatalf("Execute version: %v", err)
	}
}

// TestDSLHelpGuard covers the -h/--help short-circuit in dslCmd's RunE, which
// prints help instead of dialing (DisableFlagParsing means cobra cannot handle
// --help itself).
func TestDSLHelpGuard(t *testing.T) {
	for _, help := range []string{"--help", "-h"} {
		if err := Run(context.Background(), []string{"validate", help}); err != nil {
			t.Errorf("Run(validate %s) = %v, want nil (help)", help, err)
		}
	}
}

// TestSecretSetHelpGuard covers the -h/--help short-circuit in secret set's RunE.
func TestSecretSetHelpGuard(t *testing.T) {
	if err := Run(context.Background(), []string{"secret", "set", "--help"}); err != nil {
		t.Errorf("Run(secret set --help) = %v, want nil (help)", err)
	}
}
