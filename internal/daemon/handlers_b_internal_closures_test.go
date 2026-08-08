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

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/scrothers/pmmcp/internal/api"
)

// TestDoWatchSetNilRunCtxFallsBackToBackground exercises doWatchSet's fallback
// when s.runDoneCh hasn't been set (New() was called directly, without
// ListenAndServe ever running) — startWatchForProcess must still get a usable
// context rather than nil.
func TestDoWatchSetNilRunCtxFallsBackToBackground(t *testing.T) {
	t.Parallel()
	s, ctx := newSuperviseTestServer(t, nil)
	res := startSuperviseProcess(ctx, t, s, api.StartPayload{
		Name: "watch-nilctx", Command: []string{"sleep", "5"}, Sandbox: "off",
	})
	dir := t.TempDir()
	watchFile := filepath.Join(dir, "trigger")
	if err := os.WriteFile(watchFile, []byte("v1"), 0o600); err != nil {
		t.Fatalf("write watch file: %v", err)
	}
	t.Cleanup(s.stopAllWatchers)

	p := s.principal("full", "sess-watch-nilctx")
	resp := s.doWatchSet(ctx, p, mustJSON(api.WatchPayload{ID: res.ID, Path: watchFile}))
	if !resp.OK {
		t.Fatalf("doWatchSet with a nil s.runDoneCh: %s: %s", resp.ErrorCode, resp.Error)
	}
}
