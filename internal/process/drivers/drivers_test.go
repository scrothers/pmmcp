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

package drivers_test

import (
	"errors"
	"testing"

	engdrivers "github.com/scrothers/pmmcp/internal/engine/drivers"
	"github.com/scrothers/pmmcp/internal/process/drivers"
)

func TestOpenLocal(t *testing.T) {
	t.Parallel()
	m, err := drivers.Open("local")
	if err != nil {
		t.Fatal(err)
	}
	if m == nil {
		t.Fatal("nil manager")
	}
	m2, err := drivers.Open("")
	if err != nil || m2 == nil {
		t.Fatalf("default local: %v", err)
	}
}

func TestOpenUnknown(t *testing.T) {
	t.Parallel()
	_, err := drivers.Open("nope")
	if !errors.Is(err, drivers.ErrUnknown) {
		t.Fatalf("Open(nope) err = %v, want ErrUnknown", err)
	}
}

func TestOpenContainerExplicitEngines(t *testing.T) {
	t.Parallel()
	// container:podman / container:docker construct without probing availability,
	// so they succeed regardless of what is installed.
	for _, name := range []string{"container:podman", "container:docker"} {
		m, err := drivers.Open(name)
		if err != nil {
			t.Fatalf("Open(%q): %v", name, err)
		}
		if m == nil {
			t.Fatalf("Open(%q): nil manager", name)
		}
	}
}

func TestOpenContainerAuto(t *testing.T) {
	t.Parallel()
	// container / container:auto probe availability; either an engine is present
	// or the engine selector reports none available.
	for _, name := range []string{"container", "container:auto"} {
		m, err := drivers.Open(name)
		if err != nil {
			// Acceptable on a host with no container engine.
			continue
		}
		if m == nil {
			t.Fatalf("Open(%q): nil manager with nil error", name)
		}
	}
}

func TestOpenContainerAutoPropagatesNoneAvailable(t *testing.T) {
	// Not t.Parallel(): t.Setenv mutates the process-wide PATH, which would
	// race with other tests probing podman/docker availability.
	//
	// Hide podman/docker from PATH so engine/drivers' "auto" selection returns
	// engdrivers.ErrNoneAvailable, which Open must propagate unwrapped.
	t.Setenv("PATH", t.TempDir())
	for _, name := range []string{"container", "container:auto"} {
		_, err := drivers.Open(name)
		if !errors.Is(err, engdrivers.ErrNoneAvailable) {
			t.Fatalf("Open(%q) err = %v, want ErrNoneAvailable", name, err)
		}
	}
}
