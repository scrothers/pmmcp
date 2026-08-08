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

package drivers

import (
	"errors"
	"fmt"

	engdrivers "github.com/scrothers/pmmcp/internal/engine/drivers"
	"github.com/scrothers/pmmcp/internal/process"
	"github.com/scrothers/pmmcp/internal/process/container"
	"github.com/scrothers/pmmcp/internal/process/local"
)

// ErrUnknown is returned when Open is asked for an unregistered driver name.
var ErrUnknown = errors.New("process/drivers: unknown driver")

// Open returns a process.Manager for the named driver.
// Supported: "local", "container" (uses engine auto), "container:podman", "container:docker".
func Open(name string) (process.Manager, error) {
	switch name {
	case "", "local":
		return local.New(), nil
	case "container", "container:auto":
		eng, err := engdrivers.Open("auto")
		if err != nil {
			return nil, err
		}
		return container.New(eng), nil
	case "container:podman":
		eng, err := engdrivers.Open("podman")
		if err != nil {
			return nil, err
		}
		return container.New(eng), nil
	case "container:docker":
		eng, err := engdrivers.Open("docker")
		if err != nil {
			return nil, err
		}
		return container.New(eng), nil
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnknown, name)
	}
}
