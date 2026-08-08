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
	"errors"
	"testing"
	"time"

	"github.com/scrothers/pmmcp/internal/domain"
	"github.com/scrothers/pmmcp/internal/engine"
	"github.com/scrothers/pmmcp/internal/engine/docker"
	"github.com/scrothers/pmmcp/internal/process"
	"github.com/scrothers/pmmcp/internal/process/container"
)

// Docker integration suite. These tests target what is DISTINCTIVE about
// Docker's client/daemon architecture and the engine capabilities that only the
// real daemon can validate — NOT the same scenarios as the Podman suite, which
// covers daemonless availability, label reconcile, and the Manager lifecycle.
//
// GitHub-hosted ubuntu runners ship Docker, so CI sets PMMCP_REQUIRE_DOCKER=1 to
// make these fail (rather than skip) when the daemon is unreachable.
//
// Run: PMMCP_REQUIRE_DOCKER=1 go test -tags=integration ./test/integration/ -run Docker

// requireDocker returns a daemon-aware docker Engine, honoring the
// PMMCP_REQUIRE_DOCKER hard-fail gate.
func requireDocker(ctx context.Context, t *testing.T) *docker.Engine {
	t.Helper()
	eng := docker.New()
	if !eng.Available(ctx) {
		requireOrSkip(t, "PMMCP_REQUIRE_DOCKER", "docker not available (binary missing or daemon down)")
	}
	return eng
}

// ensureDockerImage pulls the shared test image, honoring the hard-fail gate.
func ensureDockerImage(ctx context.Context, t *testing.T, eng *docker.Engine) {
	t.Helper()
	if err := eng.PullImage(ctx, testContainerImage); err != nil {
		requireOrSkip(t, "PMMCP_REQUIRE_DOCKER", "cannot pull "+testContainerImage+": "+err.Error())
	}
}

// TestDockerDaemonAvailability is Docker-specific: unlike daemonless Podman, a
// present docker binary does not imply a usable engine. Available must confirm
// the daemon, and Version must report a Server.
func TestDockerDaemonAvailability(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	eng := requireDocker(ctx, t)
	v, err := eng.Version(ctx)
	if err != nil {
		t.Fatalf("Version: %v", err)
	}
	if v.Server == "" {
		t.Fatalf("Version.Server empty; a reachable Docker daemon must report one: %+v", v)
	}
	t.Logf("docker client=%s server=%s", v.Client, v.Server)
}

// TestDockerOneShotExitStatus exercises the exit-status capabilities against a
// real daemon: run a short-lived container (no --rm so its status survives),
// Wait for the exit code, Inspect the terminal state parsed from real docker
// JSON, then Remove it and confirm it is gone. This is the engine-level path the
// Podman suite does not cover.
func TestDockerOneShotExitStatus(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	eng := requireDocker(ctx, t)
	ensureDockerImage(ctx, t, eng)

	cid, err := eng.Run(ctx, engine.RunSpec{
		Image:    testContainerImage,
		Command:  []string{"sh", "-c", "exit 7"},
		NoRemove: true, // keep the exited container so its status is inspectable
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	t.Cleanup(func() { _ = eng.Remove(context.Background(), cid, true) })

	code, err := eng.Wait(ctx, cid)
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if code != 7 {
		t.Fatalf("Wait code = %d, want 7", code)
	}

	st, err := eng.Inspect(ctx, cid)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if st.Running {
		t.Errorf("Running = true, want false")
	}
	if st.State != engine.StateExited {
		t.Errorf("State = %q, want exited", st.State)
	}
	if st.ExitCode != 7 {
		t.Errorf("ExitCode = %d, want 7", st.ExitCode)
	}
	if st.Image != testContainerImage {
		t.Errorf("Image = %q, want %q", st.Image, testContainerImage)
	}

	if err := eng.Remove(ctx, cid, true); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := eng.Inspect(ctx, cid); !errors.Is(err, engine.ErrNotFound) {
		t.Fatalf("Inspect after remove err = %v, want ErrNotFound", err)
	}
}

// TestDockerStrictHardening drives the container Manager with the strict profile
// against real Docker, confirming the daemon accepts the hardening flags
// (cap_drop ALL, read-only rootfs, no-new-privileges, loopback ports) and the
// container runs, then stops cleanly.
func TestDockerStrictHardening(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	eng := requireDocker(ctx, t)
	ensureDockerImage(ctx, t, eng)

	m := container.New(eng)
	suffix := uniqueSuffix()
	h, err := m.Start(ctx, process.StartSpec{
		ID:      "proc-dockerstrict" + suffix,
		Name:    "pmmcp-int-docker-strict-" + suffix,
		Image:   testContainerImage,
		Command: []string{"sleep", "30"}, // no rootfs writes: valid under read-only
		Sandbox: "strict",
	})
	if err != nil {
		t.Fatalf("Start (strict): %v", err)
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
