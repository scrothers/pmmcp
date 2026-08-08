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

package docker

import "github.com/scrothers/pmmcp/internal/engine"

// NewWithCLI returns a docker Engine backed by cli, for hermetic unit tests
// that inject a fake LookPath/Command instead of requiring a real docker
// binary. Test-only: this file is compiled solely when running `go test`,
// never into the built package (export_test.go idiom).
func NewWithCLI(cli engine.CLIRunner) *Engine {
	return &Engine{cli: cli}
}

// CLIEnv exposes the engine's extra command environment so tests can assert that
// options like WithHost populated it.
func CLIEnv(e *Engine) []string {
	return e.cli.Env
}
