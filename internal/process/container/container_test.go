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

package container_test

import (
	"context"
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/scrothers/pmmcp/internal/domain"
	"github.com/scrothers/pmmcp/internal/engine"
	"github.com/scrothers/pmmcp/internal/engine/fake"
	"github.com/scrothers/pmmcp/internal/process"
	"github.com/scrothers/pmmcp/internal/process/container"
)

// raceEngine wraps *fake.Engine so tests can create a deterministic race
// window inside Run and record Stop calls, without editing internal/engine/fake.
type raceEngine struct {
	*fake.Engine

	release chan struct{}
	entered chan struct{}
	first   atomic.Bool

	mu         sync.Mutex
	blockedCID string
	stopCalls  []string
}

var _ engine.Engine = (*raceEngine)(nil)

func newRaceEngine() *raceEngine {
	return &raceEngine{
		Engine:  fake.New(),
		release: make(chan struct{}),
		entered: make(chan struct{}),
	}
}

// Run blocks the first caller until release is closed, letting later callers
// proceed immediately; this reproduces the window container.Manager.Start's
// post-Run duplicate check guards against.
func (e *raceEngine) Run(ctx context.Context, spec engine.RunSpec) (string, error) {
	blocked := e.first.CompareAndSwap(false, true)
	if blocked {
		close(e.entered)
		<-e.release
	}
	cid, err := e.Engine.Run(ctx, spec)
	if blocked {
		e.mu.Lock()
		e.blockedCID = cid
		e.mu.Unlock()
	}
	return cid, err
}

func (e *raceEngine) Stop(ctx context.Context, containerID string, timeout time.Duration) error {
	e.mu.Lock()
	e.stopCalls = append(e.stopCalls, containerID)
	e.mu.Unlock()
	return e.Engine.Stop(ctx, containerID, timeout)
}

func TestStartInspectStop(t *testing.T) {
	t.Parallel()
	eng := fake.New()
	m := container.New(eng)
	ctx := context.Background()

	h, err := m.Start(ctx, process.StartSpec{
		ID:      "proc-01ARZ3NDEKTSV4RRFFQ69G5FAV",
		Name:    "db",
		Image:   "postgres:16",
		Env:     []string{"POSTGRES_PASSWORD=secret"},
		Ports:   []string{"5432:5432"},
		Command: []string{"postgres"},
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if h.PID != 0 {
		t.Fatalf("PID = %d, want 0 for container handle", h.PID)
	}
	if h.ContainerID == "" {
		t.Fatal("empty ContainerID")
	}
	if h.Status != domain.StatusRunning {
		t.Fatalf("Status = %q, want %q", h.Status, domain.StatusRunning)
	}
	if len(eng.Runs) != 1 || eng.Runs[0].Image != "postgres:16" {
		t.Fatalf("engine Runs = %+v", eng.Runs)
	}
	if eng.Runs[0].Env["POSTGRES_PASSWORD"] != "secret" {
		t.Fatalf("env not mapped: %+v", eng.Runs[0].Env)
	}

	got, err := m.Inspect(ctx, h.ID)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if got.ContainerID != h.ContainerID {
		t.Fatalf("Inspect ContainerID = %q, want %q", got.ContainerID, h.ContainerID)
	}

	if err := m.Stop(ctx, h.ID, time.Second); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	waited, err := m.Wait(ctx, h.ID)
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if waited.Status != domain.StatusExited {
		t.Fatalf("status = %q, want exited", waited.Status)
	}
}

func TestStartValidation(t *testing.T) {
	t.Parallel()
	m := container.New(fake.New())
	ctx := context.Background()

	if _, err := m.Start(ctx, process.StartSpec{Image: "x"}); !errors.Is(err, process.ErrInvalidSpec) {
		t.Fatalf("empty id err = %v", err)
	}
	if _, err := m.Start(ctx, process.StartSpec{ID: "p1"}); !errors.Is(err, process.ErrInvalidSpec) {
		t.Fatalf("empty image err = %v", err)
	}

	spec := process.StartSpec{ID: "p1", Image: "alpine"}
	if _, err := m.Start(ctx, spec); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if _, err := m.Start(ctx, spec); !errors.Is(err, process.ErrAlreadyExists) {
		t.Fatalf("duplicate err = %v", err)
	}
}

func TestStrictHardeningAndLabels(t *testing.T) {
	t.Parallel()
	eng := fake.New()
	m := container.New(eng)
	if _, err := m.Start(context.Background(), process.StartSpec{
		ID:      "proc-01ARZ3NDEKTSV4RRFFQ69G5FAV",
		Name:    "app",
		Image:   "alpine",
		Ports:   []string{"8080:8080"},
		Sandbox: "strict",
	}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if len(eng.Runs) != 1 {
		t.Fatalf("Runs = %+v", eng.Runs)
	}
	rs := eng.Runs[0]
	if len(rs.CapDrop) != 1 || rs.CapDrop[0] != "ALL" {
		t.Errorf("CapDrop = %v, want [ALL]", rs.CapDrop)
	}
	if !rs.ReadOnlyRootfs {
		t.Error("ReadOnlyRootfs = false, want true in strict")
	}
	if len(rs.SecurityOpt) != 1 || rs.SecurityOpt[0] != "no-new-privileges" {
		t.Errorf("SecurityOpt = %v", rs.SecurityOpt)
	}
	if rs.Privileged {
		t.Error("Privileged = true, want false in strict")
	}
	if rs.PublishAllInterfaces {
		t.Error("PublishAllInterfaces = true, want loopback in strict")
	}
	if rs.Labels["io.pmmcp.proc_id"] != "proc-01ARZ3NDEKTSV4RRFFQ69G5FAV" {
		t.Errorf("proc_id label = %q", rs.Labels["io.pmmcp.proc_id"])
	}
	if rs.Labels["io.pmmcp.name"] != "app" {
		t.Errorf("name label = %q", rs.Labels["io.pmmcp.name"])
	}
}

func TestStrictRejectsSockEnv(t *testing.T) {
	t.Parallel()
	m := container.New(fake.New())
	_, err := m.Start(context.Background(), process.StartSpec{
		ID:      "proc-1",
		Image:   "alpine",
		Env:     []string{"DOCKER_HOST=unix:///var/run/docker.sock"},
		Sandbox: "strict",
	})
	if !errors.Is(err, process.ErrSandboxFailed) {
		t.Fatalf("err = %v, want ErrSandboxFailed", err)
	}
}

func TestPermissivePublishesAllInterfaces(t *testing.T) {
	t.Parallel()
	eng := fake.New()
	m := container.New(eng)
	if _, err := m.Start(context.Background(), process.StartSpec{
		ID:      "proc-2",
		Image:   "alpine",
		Ports:   []string{"8080:8080"},
		Sandbox: "permissive",
	}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	rs := eng.Runs[0]
	if !rs.PublishAllInterfaces {
		t.Error("permissive should publish on all interfaces")
	}
	if len(rs.CapDrop) != 0 {
		t.Errorf("permissive should not drop caps: %v", rs.CapDrop)
	}
}

func TestInspectNotFound(t *testing.T) {
	t.Parallel()
	m := container.New(fake.New())
	_, err := m.Inspect(context.Background(), "missing")
	if !errors.Is(err, process.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

// TestInspectDetectsNaturalExit verifies the Inspector wiring: when the
// container exits on its own (not via Stop), Inspect refreshes state from the
// engine and reports the terminal status + exit code, instead of staying pinned
// to running.
func TestInspectDetectsNaturalExit(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("clean exit", func(t *testing.T) {
		t.Parallel()
		eng := fake.New()
		m := container.New(eng)
		h, err := m.Start(ctx, process.StartSpec{ID: "p-clean", Image: "img"})
		if err != nil {
			t.Fatalf("Start: %v", err)
		}
		eng.Exit(h.ContainerID, 0, false)
		got, err := m.Inspect(ctx, h.ID)
		if err != nil {
			t.Fatalf("Inspect: %v", err)
		}
		if got.Status != domain.StatusExited {
			t.Fatalf("Status = %q, want exited", got.Status)
		}
		if got.ExitCode == nil || *got.ExitCode != 0 {
			t.Fatalf("ExitCode = %v, want 0", got.ExitCode)
		}
		// Wait now returns immediately (done was closed by the Inspect reap).
		if w, err := m.Wait(ctx, h.ID); err != nil || w.Status != domain.StatusExited {
			t.Fatalf("Wait = %+v err=%v", w, err)
		}
	})

	t.Run("crash maps to crashed", func(t *testing.T) {
		t.Parallel()
		eng := fake.New()
		m := container.New(eng)
		h, _ := m.Start(ctx, process.StartSpec{ID: "p-crash", Image: "img"})
		eng.Exit(h.ContainerID, 1, false)
		got, _ := m.Inspect(ctx, h.ID)
		if got.Status != domain.StatusCrashed {
			t.Fatalf("Status = %q, want crashed", got.Status)
		}
		if got.ExitCode == nil || *got.ExitCode != 1 {
			t.Fatalf("ExitCode = %v, want 1", got.ExitCode)
		}
	})

	t.Run("oom maps to crashed", func(t *testing.T) {
		t.Parallel()
		eng := fake.New()
		m := container.New(eng)
		h, _ := m.Start(ctx, process.StartSpec{ID: "p-oom", Image: "img"})
		eng.Exit(h.ContainerID, 0, true) // exit 0 but OOM-killed
		got, _ := m.Inspect(ctx, h.ID)
		if got.Status != domain.StatusCrashed {
			t.Fatalf("Status = %q, want crashed (OOM)", got.Status)
		}
	})

	t.Run("unhealthy reflected while running", func(t *testing.T) {
		t.Parallel()
		eng := fake.New()
		m := container.New(eng)
		h, _ := m.Start(ctx, process.StartSpec{ID: "p-unhealthy", Image: "img"})
		eng.SetHealth(h.ContainerID, "unhealthy")
		got, _ := m.Inspect(ctx, h.ID)
		if got.Status != domain.StatusUnhealthy {
			t.Fatalf("Status = %q, want unhealthy", got.Status)
		}
	})

	t.Run("engine inspect error falls back to last-known", func(t *testing.T) {
		t.Parallel()
		eng := fake.New()
		eng.InspectErr = errors.New("transient")
		m := container.New(eng)
		h, _ := m.Start(ctx, process.StartSpec{ID: "p-err", Image: "img"})
		got, err := m.Inspect(ctx, h.ID)
		if err != nil {
			t.Fatalf("Inspect should not fail on engine error: %v", err)
		}
		if got.Status != domain.StatusRunning {
			t.Fatalf("Status = %q, want running (last-known)", got.Status)
		}
	})
}

func TestNewNilEnginePanics(t *testing.T) {
	t.Parallel()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for nil engine")
		}
		if r != "process/container: nil engine" {
			t.Fatalf("panic = %v, want %q", r, "process/container: nil engine")
		}
	}()
	container.New(nil)
}

func TestStartCtxCanceled(t *testing.T) {
	t.Parallel()
	m := container.New(fake.New())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := m.Start(ctx, process.StartSpec{ID: "x", Image: "alpine"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestStartInvalidCommand(t *testing.T) {
	t.Parallel()
	m := container.New(fake.New())
	_, err := m.Start(context.Background(), process.StartSpec{
		ID: "x", Image: "alpine", Command: []string{""},
	})
	if !errors.Is(err, process.ErrInvalidSpec) {
		t.Fatalf("err = %v, want ErrInvalidSpec", err)
	}
}

func TestStartEngineRunError(t *testing.T) {
	t.Parallel()
	eng := fake.New()
	eng.RunErr = errors.New("engine boom")
	m := container.New(eng)
	_, err := m.Start(context.Background(), process.StartSpec{ID: "x", Image: "alpine"})
	if err == nil || !errors.Is(err, eng.RunErr) {
		t.Fatalf("err = %v, want wrapped %v", err, eng.RunErr)
	}
}

// TestStartDuplicateIDRace exercises the post-Run duplicate-ID guard: two
// concurrent Start calls with the same spec.ID race between eng.Run and the
// map insert. The first caller into eng.Run is held there (via raceEngine)
// until the second caller has fully inserted its entry, so the first caller
// hits the post-Run duplicate check and must clean up its own container.
func TestStartDuplicateIDRace(t *testing.T) {
	t.Parallel()
	eng := newRaceEngine()
	m := container.New(eng)
	ctx := context.Background()
	spec := process.StartSpec{ID: "proc-dup", Image: "alpine"}

	type result struct {
		h   *process.Handle
		err error
	}
	firstResult := make(chan result, 1)
	go func() {
		h, err := m.Start(ctx, spec)
		firstResult <- result{h, err}
	}()

	<-eng.entered // first caller is now blocked inside eng.Run

	secondH, secondErr := m.Start(ctx, spec)
	if secondErr != nil {
		t.Fatalf("second Start: %v", secondErr)
	}
	if secondH == nil || secondH.ContainerID == "" {
		t.Fatalf("second Start handle = %+v", secondH)
	}

	close(eng.release)
	first := <-firstResult
	if !errors.Is(first.err, process.ErrAlreadyExists) {
		t.Fatalf("first Start err = %v, want ErrAlreadyExists", first.err)
	}

	eng.mu.Lock()
	stopCalls := append([]string(nil), eng.stopCalls...)
	blockedCID := eng.blockedCID
	eng.mu.Unlock()
	if len(stopCalls) != 1 || stopCalls[0] != blockedCID {
		t.Fatalf("stopCalls = %v, want [%q] (the loser's container cleaned up)", stopCalls, blockedCID)
	}
}

func TestStopCtxCanceled(t *testing.T) {
	t.Parallel()
	m := container.New(fake.New())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := m.Stop(ctx, "x", time.Second); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestStopZeroTimeoutUsesDefault(t *testing.T) {
	t.Parallel()
	m := container.New(fake.New())
	ctx := context.Background()
	h, err := m.Start(ctx, process.StartSpec{ID: "proc-1", Image: "alpine"})
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Stop(ctx, h.ID, 0); err != nil {
		t.Fatalf("Stop with zero timeout: %v", err)
	}
}

func TestStopNotFound(t *testing.T) {
	t.Parallel()
	m := container.New(fake.New())
	if err := m.Stop(context.Background(), "missing", time.Second); !errors.Is(err, process.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestStopAlreadyTerminalIsNoOp(t *testing.T) {
	t.Parallel()
	m := container.New(fake.New())
	ctx := context.Background()
	h, err := m.Start(ctx, process.StartSpec{ID: "proc-1", Image: "alpine"})
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Stop(ctx, h.ID, time.Second); err != nil {
		t.Fatalf("first Stop: %v", err)
	}
	if err := m.Stop(ctx, h.ID, time.Second); err != nil {
		t.Fatalf("second Stop (no-op): %v", err)
	}
}

func TestStopEngineError(t *testing.T) {
	t.Parallel()
	eng := fake.New()
	m := container.New(eng)
	ctx := context.Background()
	h, err := m.Start(ctx, process.StartSpec{ID: "proc-1", Image: "alpine"})
	if err != nil {
		t.Fatal(err)
	}
	eng.StopErr = errors.New("stop boom")
	if err := m.Stop(ctx, h.ID, time.Second); err == nil || !errors.Is(err, eng.StopErr) {
		t.Fatalf("err = %v, want wrapped %v", err, eng.StopErr)
	}
}

func TestWaitNotFound(t *testing.T) {
	t.Parallel()
	m := container.New(fake.New())
	if _, err := m.Wait(context.Background(), "missing"); !errors.Is(err, process.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestWaitCtxAlreadyCanceled(t *testing.T) {
	t.Parallel()
	m := container.New(fake.New())
	ctx := context.Background()
	h, err := m.Start(ctx, process.StartSpec{ID: "proc-1", Image: "alpine"})
	if err != nil {
		t.Fatal(err)
	}
	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := m.Wait(canceledCtx, h.ID); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestWaitBlocksThenSucceedsOnStop(t *testing.T) {
	t.Parallel()
	m := container.New(fake.New())
	ctx := context.Background()
	h, err := m.Start(ctx, process.StartSpec{ID: "proc-1", Image: "alpine"})
	if err != nil {
		t.Fatal(err)
	}

	type result struct {
		h   *process.Handle
		err error
	}
	resCh := make(chan result, 1)
	go func() {
		wh, werr := m.Wait(ctx, h.ID)
		resCh <- result{wh, werr}
	}()

	time.Sleep(20 * time.Millisecond)
	select {
	case <-resCh:
		t.Fatal("Wait returned before Stop")
	default:
	}

	if err := m.Stop(ctx, h.ID, time.Second); err != nil {
		t.Fatal(err)
	}

	select {
	case res := <-resCh:
		if res.err != nil {
			t.Fatalf("Wait: %v", res.err)
		}
		if res.h.Status != domain.StatusExited {
			t.Fatalf("Wait status = %q, want exited", res.h.Status)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Wait did not complete after Stop")
	}
}

func TestInspectCtxCanceled(t *testing.T) {
	t.Parallel()
	m := container.New(fake.New())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := m.Inspect(ctx, "x"); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestSignalCtxCanceled(t *testing.T) {
	t.Parallel()
	m := container.New(fake.New())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := m.Signal(ctx, "x", os.Interrupt); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestSignalNotFound(t *testing.T) {
	t.Parallel()
	m := container.New(fake.New())
	if err := m.Signal(context.Background(), "missing", os.Interrupt); !errors.Is(err, process.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestSignalOnTerminalReturnsErrNotRunning(t *testing.T) {
	t.Parallel()
	m := container.New(fake.New())
	ctx := context.Background()
	h, err := m.Start(ctx, process.StartSpec{ID: "proc-1", Image: "alpine"})
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Stop(ctx, h.ID, time.Second); err != nil {
		t.Fatal(err)
	}
	if err := m.Signal(ctx, h.ID, os.Interrupt); !errors.Is(err, process.ErrNotRunning) {
		t.Fatalf("err = %v, want ErrNotRunning", err)
	}
}

func TestSignalOnRunningNotSupported(t *testing.T) {
	t.Parallel()
	m := container.New(fake.New())
	ctx := context.Background()
	h, err := m.Start(ctx, process.StartSpec{ID: "proc-1", Image: "alpine"})
	if err != nil {
		t.Fatal(err)
	}
	err = m.Signal(ctx, h.ID, os.Interrupt)
	if err == nil {
		t.Fatal("expected signal-not-supported error")
	}
	if errors.Is(err, process.ErrNotFound) || errors.Is(err, process.ErrNotRunning) {
		t.Fatalf("err = %v, want plain not-supported error", err)
	}
}

func TestLogsHappyPath(t *testing.T) {
	t.Parallel()
	m := container.New(fake.New())
	ctx := context.Background()
	h, err := m.Start(ctx, process.StartSpec{ID: "proc-1", Image: "alpine"})
	if err != nil {
		t.Fatal(err)
	}
	out, err := m.Logs(ctx, h.ID, 0)
	if err != nil {
		t.Fatalf("Logs: %v", err)
	}
	if out == "" {
		t.Fatal("Logs returned empty output")
	}
}

func TestLogsNotFound(t *testing.T) {
	t.Parallel()
	m := container.New(fake.New())
	if _, err := m.Logs(context.Background(), "missing", 0); !errors.Is(err, process.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestLogsCtxCanceled(t *testing.T) {
	t.Parallel()
	m := container.New(fake.New())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := m.Logs(ctx, "x", 0); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestEnvSliceToMapMalformedEntriesSkipped(t *testing.T) {
	t.Parallel()
	eng := fake.New()
	m := container.New(eng)
	_, err := m.Start(context.Background(), process.StartSpec{
		ID:    "proc-1",
		Image: "alpine",
		Env:   []string{"noequals", "=onlyvalue", "K=V"},
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	got := eng.Runs[0].Env
	if len(got) != 1 || got["K"] != "V" {
		t.Fatalf("Env = %+v, want only {K: V}", got)
	}
}
