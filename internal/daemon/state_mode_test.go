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

package daemon_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/scrothers/pmmcp/internal/config"
	"github.com/scrothers/pmmcp/internal/daemon"
	"github.com/scrothers/pmmcp/internal/testsock"
)

func TestStateDirMode0700(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cfg, err := config.Load(config.LoadOptions{
		GOOS: "linux", Home: dir,
		LookupEnv: func(string) (string, bool) { return "", false },
	})
	if err != nil {
		t.Fatal(err)
	}
	cfg.StateDir = filepath.Join(dir, "state")
	cfg.IPC.Endpoint = testsock.Path(t)
	cfg.Sandbox.Default = "off"
	cfg.Relaunch.Enabled = false
	ctx := context.Background()
	srv, err := daemon.New(ctx, daemon.Options{Config: cfg, DBPath: filepath.Join(dir, "db.sqlite")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Close() })
	st, err := os.Stat(cfg.StateDir)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm()&0o077 != 0 {
		t.Fatalf("state dir mode %o allows group/other", st.Mode().Perm())
	}
}
