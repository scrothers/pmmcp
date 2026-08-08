// Copyright 2026 Steven Crothers
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package drivers selects and assembles engine.Engine implementations.
package drivers

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/scrothers/pmmcp/internal/engine"
	"github.com/scrothers/pmmcp/internal/engine/docker"
	"github.com/scrothers/pmmcp/internal/engine/podman"
)

// ErrUnknown is returned when Open is asked for an unregistered engine name.
var ErrUnknown = errors.New("engine/drivers: unknown engine")

// ErrNoneAvailable is returned when auto cannot find podman or docker.
var ErrNoneAvailable = errors.New("engine/drivers: no container engine available")

// Open returns an engine.Engine for the named backend.
//
// Supported names: "podman", "docker", "auto" (or empty, treated as auto).
// Auto prefers podman when Available, else docker; never guesses silently
// when both exist — the returned engine's Name records the choice.
//
// Wiring is explicit: no func init.
func Open(name string) (engine.Engine, error) {
	n := strings.ToLower(strings.TrimSpace(name))
	switch n {
	case "podman":
		return podman.New(), nil
	case "docker":
		return docker.New(), nil
	case "auto", "":
		return auto(context.Background())
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnknown, name)
	}
}

// OpenContext is like Open but uses ctx for availability probes on auto.
func OpenContext(ctx context.Context, name string) (engine.Engine, error) {
	n := strings.ToLower(strings.TrimSpace(name))
	switch n {
	case "podman":
		return podman.New(), nil
	case "docker":
		return docker.New(), nil
	case "auto", "":
		return auto(ctx)
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnknown, name)
	}
}

func auto(ctx context.Context) (engine.Engine, error) {
	return chooseEngine(ctx, podman.New(), docker.New())
}

// chooseEngine returns the first candidate that reports Available, in the
// order given (podman preferred over docker for auto), or ErrNoneAvailable
// if none are. Extracted from auto so tests can inject fake engines and
// exercise every branch without depending on which binaries happen to be
// installed on the host running the tests.
func chooseEngine(ctx context.Context, candidates ...engine.Engine) (engine.Engine, error) {
	for _, c := range candidates {
		if c.Available(ctx) {
			return c, nil
		}
	}
	return nil, ErrNoneAvailable
}
