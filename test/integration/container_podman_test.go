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
	"context"
	"testing"
	"time"

	"github.com/scrothers/pmmcp/internal/domain"
	"github.com/scrothers/pmmcp/internal/engine"
	"github.com/scrothers/pmmcp/internal/engine/podman"
	"github.com/scrothers/pmmcp/internal/process"
	"github.com/scrothers/pmmcp/internal/process/container"
)

// Podman integration suite. These tests target what is DISTINCTIVE about
// Podman — daemonless/rootless operation — and the capabilities the Docker suite
// does NOT cover (label-based reconcile via List, and the container Manager
// lifecycle). It intentionally does not repeat the Docker suite's daemon-probe,
// one-shot-exit, or strict-hardening scenarios.
//
// GitHub-hosted ubuntu runners ship Podman, so CI sets PMMCP_REQUIRE_PODMAN=1 to
// make these fail (rather than skip) when Podman is unavailable.
//
// Run: PMMCP_REQUIRE_PODMAN=1 go test -tags=integration ./test/integration/ -run Podman

// requirePodman returns a podman Engine, honoring the PMMCP_REQUIRE_PODMAN
// hard-fail gate.
func requirePodman(ctx context.Context, t *testing.T) *podman.Engine {
	t.Helper()
	eng := podman.New()
	if !eng.Available(ctx) {
		requireOrSkip(t, "PMMCP_REQUIRE_PODMAN", "podman not on PATH")
	}
	return eng
}

// ensurePodmanImage pulls the shared test image, honoring the hard-fail gate.
func ensurePodmanImage(ctx context.Context, t *testing.T, eng *podman.Engine) {
	t.Helper()
	if err := eng.PullImage(ctx, testContainerImage); err != nil {
		requireOrSkip(t, "PMMCP_REQUIRE_PODMAN", "cannot pull "+testContainerImage+": "+err.Error())
	}
}

// TestPodmanRootlessAvailability is Podman-specific: Podman is daemonless, so
// availability is binary presence — no separate daemon must be running — and
// Version reports a client even when there is no server (unlike Docker, whose
// availability requires a reachable daemon reporting a server version).
func TestPodmanRootlessAvailability(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	eng := requirePodman(ctx, t)
	v, err := eng.Version(ctx)
	if err != nil {
		t.Fatalf("Version: %v", err)
	}
	if v.Client == "" {
		t.Fatalf("Version.Client empty; podman should report a client version: %+v", v)
	}
	// Server is deliberately not asserted: rootless podman has no long-running
	// daemon, so an empty Server is expected and valid.
	t.Logf("podman client=%s server=%q", v.Client, v.Server)
}

// TestPodmanLabelReconcile exercises List-by-label — the reconcile path pmmcp
// uses to re-attach managed containers after a daemon restart. Two containers
// are started with distinct io.pmmcp.proc_id labels; List must return only the
// one matching the queried label. This engine capability is not covered by the
// Docker suite.
func TestPodmanLabelReconcile(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	eng := requirePodman(ctx, t)
	ensurePodmanImage(ctx, t, eng)

	suffix := uniqueSuffix()
	wantProc := "proc-podman-a-" + suffix
	otherProc := "proc-podman-b-" + suffix

	wantCID, err := eng.Run(ctx, engine.RunSpec{
		Image:    testContainerImage,
		Command:  []string{"sleep", "60"},
		Labels:   map[string]string{"io.pmmcp.proc_id": wantProc},
		NoRemove: true,
	})
	if err != nil {
		t.Fatalf("Run (a): %v", err)
	}
	t.Cleanup(func() { _ = eng.Remove(context.Background(), wantCID, true) })

	otherCID, err := eng.Run(ctx, engine.RunSpec{
		Image:    testContainerImage,
		Command:  []string{"sleep", "60"},
		Labels:   map[string]string{"io.pmmcp.proc_id": otherProc},
		NoRemove: true,
	})
	if err != nil {
		t.Fatalf("Run (b): %v", err)
	}
	t.Cleanup(func() { _ = eng.Remove(context.Background(), otherCID, true) })

	got, err := eng.List(ctx, map[string]string{"io.pmmcp.proc_id": wantProc})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("List returned %d containers, want exactly 1: %+v", len(got), got)
	}
	if got[0].ID != wantCID {
		t.Fatalf("List returned %q, want %q", got[0].ID, wantCID)
	}
	if got[0].Labels["io.pmmcp.proc_id"] != wantProc {
		t.Fatalf("List label = %v, want io.pmmcp.proc_id=%s", got[0].Labels, wantProc)
	}
}

// TestPodmanContainerManagerLifecycle drives the container process Manager
// against real Podman: start a long-running container, confirm the Inspector
// wiring reports it running, then stop it. This is the Manager-level lifecycle
// the Docker suite exercises only under the strict profile.
func TestPodmanContainerManagerLifecycle(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	eng := requirePodman(ctx, t)
	ensurePodmanImage(ctx, t, eng)

	m := container.New(eng)
	suffix := uniqueSuffix()
	h, err := m.Start(ctx, process.StartSpec{
		ID:      "proc-podmanlife" + suffix,
		Name:    "pmmcp-int-podman-" + suffix,
		Image:   testContainerImage,
		Command: []string{"sleep", "30"},
		Sandbox: "off",
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = m.Stop(context.Background(), h.ID, 5*time.Second) })

	if h.ContainerID == "" {
		t.Fatal("empty container id")
	}
	if h.Status != domain.StatusRunning {
		t.Fatalf("status = %q, want running", h.Status)
	}
	insp, err := m.Inspect(ctx, h.ID)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if insp.Status != domain.StatusRunning {
		t.Fatalf("inspected status = %q, want running", insp.Status)
	}
	if err := m.Stop(ctx, h.ID, 10*time.Second); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}
