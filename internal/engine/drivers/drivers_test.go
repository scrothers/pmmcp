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
	"context"
	"errors"
	"testing"

	"github.com/scrothers/pmmcp/internal/engine/drivers"
	"github.com/scrothers/pmmcp/internal/engine/fake"
)

func TestOpenNamed(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"podman", "docker"} {
		e, err := drivers.Open(name)
		if err != nil {
			t.Fatalf("Open(%q): %v", name, err)
		}
		if e.Name() != name {
			t.Fatalf("Name = %q, want %q", e.Name(), name)
		}
	}
}

func TestOpenUnknown(t *testing.T) {
	t.Parallel()
	_, err := drivers.Open("nerdctl")
	if !errors.Is(err, drivers.ErrUnknown) {
		t.Fatalf("err = %v, want ErrUnknown", err)
	}
}

func TestOpenContextNamedAndUnknown(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	e, err := drivers.OpenContext(ctx, "podman")
	if err != nil {
		t.Fatalf("OpenContext(podman): %v", err)
	}
	if e.Name() != "podman" {
		t.Fatalf("Name = %q, want podman", e.Name())
	}
	if _, err := drivers.OpenContext(ctx, "nerdctl"); !errors.Is(err, drivers.ErrUnknown) {
		t.Fatalf("err = %v, want ErrUnknown", err)
	}
	dockerEngine, err := drivers.OpenContext(ctx, "docker")
	if err != nil {
		t.Fatalf("OpenContext(docker): %v", err)
	}
	if dockerEngine.Name() != "docker" {
		t.Fatalf("Name = %q, want docker", dockerEngine.Name())
	}
	// Empty name routes to auto (either an engine or ErrNoneAvailable).
	if _, err := drivers.OpenContext(ctx, ""); err != nil && !errors.Is(err, drivers.ErrNoneAvailable) {
		t.Fatalf("OpenContext(\"\") err = %v", err)
	}
}

func TestOpenContextUnknown(t *testing.T) {
	t.Parallel()
	if _, err := drivers.OpenContext(context.Background(), "nerdctl"); !errors.Is(err, drivers.ErrUnknown) {
		t.Fatalf("err = %v, want ErrUnknown", err)
	}
}

func TestOpenContextEmptyRoutesToAuto(t *testing.T) {
	t.Parallel()
	if _, err := drivers.OpenContext(context.Background(), ""); err != nil && !errors.Is(err, drivers.ErrNoneAvailable) {
		t.Fatalf("OpenContext(\"\") err = %v", err)
	}
}

func TestChooseEngineFirstAvailable(t *testing.T) {
	t.Parallel()
	first := fake.New()
	first.AvailableFunc = func(context.Context) bool { return true }
	second := fake.New()
	second.AvailableFunc = func(context.Context) bool {
		t.Fatal("second candidate should not be probed when the first is available")
		return false
	}
	e, err := drivers.ChooseEngine(context.Background(), first, second)
	if err != nil {
		t.Fatalf("ChooseEngine: %v", err)
	}
	if e != first {
		t.Fatal("ChooseEngine returned the wrong candidate")
	}
}

func TestChooseEngineFirstUnavailableSecondAvailable(t *testing.T) {
	t.Parallel()
	first := fake.New()
	first.AvailableFunc = func(context.Context) bool { return false }
	second := fake.New()
	second.AvailableFunc = func(context.Context) bool { return true }
	e, err := drivers.ChooseEngine(context.Background(), first, second)
	if err != nil {
		t.Fatalf("ChooseEngine: %v", err)
	}
	if e != second {
		t.Fatal("ChooseEngine returned the wrong candidate")
	}
}

func TestChooseEngineNoneAvailable(t *testing.T) {
	t.Parallel()
	first := fake.New()
	first.AvailableFunc = func(context.Context) bool { return false }
	second := fake.New()
	second.AvailableFunc = func(context.Context) bool { return false }
	_, err := drivers.ChooseEngine(context.Background(), first, second)
	if !errors.Is(err, drivers.ErrNoneAvailable) {
		t.Fatalf("err = %v, want ErrNoneAvailable", err)
	}
}

func TestChooseEngineCtxCanceled(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	// AvailableFunc left nil: fake.Engine.Available falls back to ctx.Err().
	_, err := drivers.ChooseEngine(ctx, fake.New(), fake.New())
	if !errors.Is(err, drivers.ErrNoneAvailable) {
		t.Fatalf("err = %v, want ErrNoneAvailable", err)
	}
}

func TestChooseEngineNoCandidates(t *testing.T) {
	t.Parallel()
	_, err := drivers.ChooseEngine(context.Background())
	if !errors.Is(err, drivers.ErrNoneAvailable) {
		t.Fatalf("err = %v, want ErrNoneAvailable", err)
	}
}

func TestOpenAuto(t *testing.T) {
	t.Parallel()
	e, err := drivers.Open("auto")
	if err != nil {
		// Neither binary present is acceptable on a bare host.
		if !errors.Is(err, drivers.ErrNoneAvailable) {
			t.Fatalf("auto err = %v, want engine or ErrNoneAvailable", err)
		}
		return
	}
	switch e.Name() {
	case "podman", "docker":
		// ok
	default:
		t.Fatalf("auto Name = %q, want podman or docker", e.Name())
	}
}
