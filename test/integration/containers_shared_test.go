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

//go:build integration

package integration_test

import (
	"os"
	"strconv"
	"testing"
	"time"
)

// Shared fixtures for the container-engine integration suites. The docker and
// podman suites deliberately test DIFFERENT scenarios (see each file); only the
// image, the uniqueness helper, and the require/skip gate are common.

// testContainerImage is the tiny image both engine suites use.
const testContainerImage = "alpine:3.20"

// uniqueSuffix returns a per-run token so repeated or concurrent runs against
// the same engine do not collide on container names or labels.
func uniqueSuffix() string {
	return strconv.FormatInt(time.Now().UnixNano(), 36)
}

// requireOrSkip fails the test when the given PMMCP_REQUIRE_* variable is set
// (as CI does, so a missing engine is a hard failure) and otherwise skips it (a
// local run without that engine installed).
func requireOrSkip(t *testing.T, envVar, msg string) {
	t.Helper()
	if os.Getenv(envVar) != "" {
		t.Fatalf("%s (%s set)", msg, envVar)
	}
	t.Skip(msg)
}
