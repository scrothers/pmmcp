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

//go:build !linux

package linux

import (
	"fmt"

	"github.com/scrothers/pmmcp/internal/sandbox"
)

// LandlockAvailable is always false off Linux.
func LandlockAvailable() bool { return false }

// LandlockRestrictPaths is unsupported off Linux.
func LandlockRestrictPaths(pol sandbox.Policy) error {
	_ = pol
	return fmt.Errorf("sandbox/linux: landlock: not supported on this platform")
}
