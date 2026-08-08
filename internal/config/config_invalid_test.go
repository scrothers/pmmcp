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
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scrothers/pmmcp/internal/config"
)

func TestInvalidTOMLReportsError(t *testing.T) {
	clearOverlayEnv(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.toml")
	// Unclosed string — should fail decode.
	writeConfig(t, path, "state_dir = \"oops\n")
	_, err := config.Load(config.LoadOptions{
		Path: path, GOOS: "linux", Home: dir, LookupEnv: noEnv,
	})
	if !errors.Is(err, config.ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid", err)
	}
}

func TestInvalidSandboxValue(t *testing.T) {
	clearOverlayEnv(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "c.toml")
	writeConfig(t, path, "[sandbox]\ndefault = \"nope\"\n")
	_, err := config.Load(config.LoadOptions{
		Path: path, GOOS: "linux", Home: dir, LookupEnv: noEnv,
	})
	if err == nil || !strings.Contains(err.Error(), "sandbox") {
		t.Fatalf("err = %v, want sandbox validation error", err)
	}
}
