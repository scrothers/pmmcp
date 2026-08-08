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
	"testing"
)

// TestRunMCPSDKLoadCfgError covers runMCPSDK's own loadCfg error branch,
// directly (Run's "mcp" dispatch case is covered separately in
// dispatch_internal_test.go, which cannot use this shortcut because a
// config error returns before ever touching stdio).
func TestRunMCPSDKLoadCfgError(t *testing.T) {
	t.Setenv("PMMCP_CONFIG", "/nonexistent/config.json")
	if err := (&rootState{}).runMCPSDK(context.Background()); err == nil {
		t.Fatal("want loadCfg error")
	}
}
