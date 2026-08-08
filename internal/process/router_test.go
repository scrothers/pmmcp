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

package process_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/scrothers/pmmcp/internal/engine/fake"
	"github.com/scrothers/pmmcp/internal/process"
	"github.com/scrothers/pmmcp/internal/process/container"
	"github.com/scrothers/pmmcp/internal/process/local"
)

func TestRouterLocalVsContainer(t *testing.T) {
	t.Parallel()
	loc := local.New()
	eng := fake.New()
	r := process.NewRouter(loc, func(_ string) (process.Manager, error) {
		return container.New(eng), nil
	})
	ctx := context.Background()
	h1, err := r.Start(ctx, process.StartSpec{ID: "proc-local1", Command: []string{"true"}, Runtime: "local"})
	if err != nil {
		t.Fatal(err)
	}
	if h1.PID <= 0 {
		t.Fatalf("local pid %d", h1.PID)
	}
	if r.RuntimeOf("proc-local1") != "local" {
		t.Fatalf("runtime %q", r.RuntimeOf("proc-local1"))
	}
	h2, err := r.Start(ctx, process.StartSpec{
		ID: "proc-ctr1", Runtime: "container", Image: "alpine:3", Command: []string{"sleep", "1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if h2.ContainerID == "" {
		t.Fatal("expected container id")
	}
	if r.RuntimeOf("proc-ctr1") != "container" {
		t.Fatalf("runtime %q", r.RuntimeOf("proc-ctr1"))
	}

	// Lifecycle dispatch routes to the manager recorded at Start.
	if _, err := r.Inspect(ctx, "proc-ctr1"); err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if err := r.Stop(ctx, "proc-ctr1", time.Second); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if _, err := r.Wait(ctx, "proc-ctr1"); err != nil {
		t.Fatalf("Wait: %v", err)
	}
	r.Forget("proc-ctr1")
	if r.RuntimeOf("proc-ctr1") != "" {
		t.Fatalf("runtime after Forget = %q, want empty", r.RuntimeOf("proc-ctr1"))
	}

	// Cleanup the local process.
	_ = r.Stop(ctx, "proc-local1", time.Second)
}

func TestRouterBareEngineAliases(t *testing.T) {
	t.Parallel()
	// Bare "podman"/"docker" runtimes must normalize to the container drivers.
	var opened []string
	eng := fake.New()
	r := process.NewRouter(local.New(), func(name string) (process.Manager, error) {
		opened = append(opened, name)
		return container.New(eng), nil
	})
	ctx := context.Background()
	if _, err := r.Start(ctx, process.StartSpec{ID: "p-pod", Runtime: "podman", Image: "alpine"}); err != nil {
		t.Fatalf("Start podman: %v", err)
	}
	if _, err := r.Start(ctx, process.StartSpec{ID: "p-dock", Runtime: "docker", Image: "alpine"}); err != nil {
		t.Fatalf("Start docker: %v", err)
	}
	want := []string{"container:podman", "container:docker"}
	if len(opened) != 2 || opened[0] != want[0] || opened[1] != want[1] {
		t.Fatalf("opened = %v, want %v", opened, want)
	}
	// Image with no runtime defaults to container.
	if _, err := r.Start(ctx, process.StartSpec{ID: "p-img", Image: "alpine"}); err != nil {
		t.Fatalf("Start image-default: %v", err)
	}
	if r.RuntimeOf("p-img") != "container" {
		t.Fatalf("runtime %q, want container", r.RuntimeOf("p-img"))
	}
}

func TestRouterOpenNilError(t *testing.T) {
	t.Parallel()
	r := process.NewRouter(local.New(), nil)
	_, err := r.Start(context.Background(), process.StartSpec{ID: "x", Runtime: "container", Image: "alpine"})
	if !errors.Is(err, process.ErrInvalidSpec) {
		t.Fatalf("err = %v, want ErrInvalidSpec", err)
	}
}

func TestRouterEmptyRuntimeEmptyImageDefaultsLocal(t *testing.T) {
	t.Parallel()
	loc := local.New()
	r := process.NewRouter(loc, func(name string) (process.Manager, error) {
		t.Fatalf("Open(%q) should not be called for local dispatch", name)
		return nil, errors.New("unreachable")
	})
	ctx := context.Background()
	h, err := r.Start(ctx, process.StartSpec{ID: "proc-empty", Command: []string{"true"}})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if r.RuntimeOf("proc-empty") != "local" {
		t.Fatalf("runtime = %q, want local", r.RuntimeOf("proc-empty"))
	}
	_ = r.Stop(ctx, h.ID, time.Second)
}

func TestRouterOpenPropagatesError(t *testing.T) {
	t.Parallel()
	openErr := errors.New("boom: open failed")
	r := process.NewRouter(local.New(), func(_ string) (process.Manager, error) {
		return nil, openErr
	})
	_, err := r.Start(context.Background(), process.StartSpec{ID: "x", Runtime: "container", Image: "alpine"})
	if !errors.Is(err, openErr) {
		t.Fatalf("err = %v, want %v", err, openErr)
	}
}

func TestRouterStartUnderlyingManagerError(t *testing.T) {
	t.Parallel()
	eng := fake.New()
	eng.RunErr = errors.New("engine boom")
	r := process.NewRouter(local.New(), func(_ string) (process.Manager, error) {
		return container.New(eng), nil
	})
	_, err := r.Start(context.Background(), process.StartSpec{ID: "x", Runtime: "container", Image: "alpine"})
	if err == nil {
		t.Fatal("expected error from underlying manager Start")
	}
}

func TestRouterMgrFallsBackToLocalForUnknownID(t *testing.T) {
	t.Parallel()
	r := process.NewRouter(local.New(), func(_ string) (process.Manager, error) {
		return container.New(fake.New()), nil
	})
	ctx := context.Background()
	if err := r.Signal(ctx, "never-started", os.Interrupt); !errors.Is(err, process.ErrNotFound) {
		t.Fatalf("Signal on unknown id err = %v, want ErrNotFound (routed to Local)", err)
	}
}

func TestRouterSignal(t *testing.T) {
	t.Parallel()
	loc := local.New()
	r := process.NewRouter(loc, nil)
	ctx := context.Background()
	h, err := r.Start(ctx, process.StartSpec{ID: "proc-signal", Command: []string{"sleep", "1"}, Runtime: "local"})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Signal(ctx, h.ID, os.Interrupt); err != nil {
		t.Fatalf("Signal: %v", err)
	}
	_ = r.Stop(ctx, h.ID, time.Second)
}
