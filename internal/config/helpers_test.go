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

package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/pflag"

	"github.com/scrothers/pmmcp/internal/config"
)

// noEnv is a LoadOptions.LookupEnv that reports every path-discovery variable as
// unset, so tests exercise the injected opts fields without leaking real env.
func noEnv(string) (string, bool) { return "", false }

// overlayEnvKeys is every PMMCP_* config-value overlay viper reads via
// AutomaticEnv. Tests clear these so a stray variable in the real environment
// cannot perturb an assertion.
var overlayEnvKeys = []string{
	"PMMCP_VERSION",
	"PMMCP_STATE_DIR",
	"PMMCP_IPC_ENDPOINT",
	"PMMCP_IPC_TOKEN_FILE",
	"PMMCP_TOKEN_FILE",
	"PMMCP_LOG_LEVEL",
	"PMMCP_LOG_FORMAT",
	"PMMCP_SANDBOX_DEFAULT",
	"PMMCP_LOGS_MAX_FILE_MB",
	"PMMCP_LOGS_MAX_FILES",
	"PMMCP_LOGS_COMPRESS",
	"PMMCP_RELAUNCH_ENABLED",
	"PMMCP_WEBHOOK_ALLOWLIST",
	"PMMCP_CONFIG",
}

// clearOverlayEnv unsets every config overlay for the duration of the test so
// viper's AutomaticEnv sees only what the test itself sets. viper treats an empty
// environment value as unset (AllowEmptyEnv defaults false), so setting each to ""
// is equivalent to clearing it. t.Setenv makes the test non-parallel, which is
// correct: it mutates process-global state.
func clearOverlayEnv(t *testing.T) {
	t.Helper()
	for _, k := range overlayEnvKeys {
		t.Setenv(k, "")
	}
}

// newDaemonFlags builds a daemon flag set (via config.RegisterDaemonFlags) and
// parses args into it, so precedence tests can supply changed CLI flags.
func newDaemonFlags(t *testing.T, args ...string) *pflag.FlagSet {
	t.Helper()
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	config.RegisterDaemonFlags(fs)
	if err := fs.Parse(args); err != nil {
		t.Fatalf("parse flags %v: %v", args, err)
	}
	return fs
}

// writeConfig writes a config file body, creating parent directories.
func writeConfig(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}
