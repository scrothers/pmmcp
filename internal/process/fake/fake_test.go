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

package fake_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/scrothers/pmmcp/internal/domain"
	"github.com/scrothers/pmmcp/internal/process"
	"github.com/scrothers/pmmcp/internal/process/fake"
)

func TestStartInspectStopWait(t *testing.T) {
	t.Parallel()
	m := fake.New()
	ctx := context.Background()

	h, err := m.Start(ctx, process.StartSpec{
		ID:      "proc-01ARZ3NDEKTSV4RRFFQ69G5FAV",
		Name:    "sleep",
		Command: []string{"sleep", "30"},
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if h.PID == 0 {
		t.Fatal("PID = 0, want non-zero")
	}
	if h.Status != domain.StatusRunning {
		t.Fatalf("Status = %q, want %q", h.Status, domain.StatusRunning)
	}
	if len(m.Starts) != 1 {
		t.Fatalf("Starts len = %d, want 1", len(m.Starts))
	}

	got, err := m.Inspect(ctx, h.ID)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if got.PID != h.PID || got.Status != domain.StatusRunning {
		t.Fatalf("Inspect = %+v, want pid %d running", got, h.PID)
	}

	if err := m.Stop(ctx, h.ID, time.Second); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	waited, err := m.Wait(ctx, h.ID)
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if waited.Status != domain.StatusExited {
		t.Fatalf("Wait status = %q, want %q", waited.Status, domain.StatusExited)
	}
	if waited.ExitCode == nil || *waited.ExitCode != 0 {
		t.Fatalf("ExitCode = %v, want 0", waited.ExitCode)
	}
}

func TestStartDuplicateAndInvalid(t *testing.T) {
	t.Parallel()
	m := fake.New()
	ctx := context.Background()
	spec := process.StartSpec{
		ID:      "proc-01ARZ3NDEKTSV4RRFFQ69G5FAV",
		Command: []string{"true"},
	}
	if _, err := m.Start(ctx, spec); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if _, err := m.Start(ctx, spec); !errors.Is(err, process.ErrAlreadyExists) {
		t.Fatalf("duplicate Start err = %v, want ErrAlreadyExists", err)
	}
	if _, err := m.Start(ctx, process.StartSpec{ID: "other", Command: nil}); !errors.Is(err, process.ErrInvalidSpec) {
		t.Fatalf("empty command err = %v, want ErrInvalidSpec", err)
	}
	if _, err := m.Start(ctx, process.StartSpec{Command: []string{"x"}}); !errors.Is(err, process.ErrInvalidSpec) {
		t.Fatalf("empty id err = %v, want ErrInvalidSpec", err)
	}
}

func TestInspectNotFound(t *testing.T) {
	t.Parallel()
	m := fake.New()
	_, err := m.Inspect(context.Background(), "missing")
	if !errors.Is(err, process.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestSignal(t *testing.T) {
	t.Parallel()
	m := fake.New()
	ctx := context.Background()
	h, err := m.Start(ctx, process.StartSpec{
		ID:      "proc-01ARZ3NDEKTSV4RRFFQ69G5FAV",
		Command: []string{"sleep", "1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Signal(ctx, h.ID, os.Interrupt); err != nil {
		t.Fatalf("Signal running: %v", err)
	}
	if err := m.Stop(ctx, h.ID, 0); err != nil {
		t.Fatal(err)
	}
	if err := m.Signal(ctx, h.ID, os.Interrupt); !errors.Is(err, process.ErrNotRunning) {
		t.Fatalf("Signal exited err = %v, want ErrNotRunning", err)
	}
}

func TestStartCtxCanceled(t *testing.T) {
	t.Parallel()
	m := fake.New()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := m.Start(ctx, process.StartSpec{ID: "x", Command: []string{"true"}}); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestStopCtxCanceled(t *testing.T) {
	t.Parallel()
	m := fake.New()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := m.Stop(ctx, "x", time.Second); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestStopNotFound(t *testing.T) {
	t.Parallel()
	m := fake.New()
	if err := m.Stop(context.Background(), "missing", time.Second); !errors.Is(err, process.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestStopAlreadyTerminalIsNoOp(t *testing.T) {
	t.Parallel()
	m := fake.New()
	ctx := context.Background()
	h, err := m.Start(ctx, process.StartSpec{ID: "proc-1", Command: []string{"true"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Stop(ctx, h.ID, time.Second); err != nil {
		t.Fatalf("first Stop: %v", err)
	}
	// Second Stop on an already-terminal process is a no-op that returns nil.
	if err := m.Stop(ctx, h.ID, time.Second); err != nil {
		t.Fatalf("second Stop: %v", err)
	}
}

func TestWaitNotFound(t *testing.T) {
	t.Parallel()
	m := fake.New()
	if _, err := m.Wait(context.Background(), "missing"); !errors.Is(err, process.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestWaitCtxCanceledWhileBlocking(t *testing.T) {
	t.Parallel()
	m := fake.New()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h, err := m.Start(ctx, process.StartSpec{ID: "proc-1", Command: []string{"sleep", "99"}})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		_, werr := m.Wait(ctx, h.ID)
		errCh <- werr
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()
	select {
	case werr := <-errCh:
		if !errors.Is(werr, context.Canceled) {
			t.Fatalf("Wait err = %v, want context.Canceled", werr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Wait did not return after ctx cancel")
	}
}

func TestInspectCtxCanceled(t *testing.T) {
	t.Parallel()
	m := fake.New()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := m.Inspect(ctx, "x"); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestSignalCtxCanceled(t *testing.T) {
	t.Parallel()
	m := fake.New()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := m.Signal(ctx, "x", os.Interrupt); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestSignalNotFound(t *testing.T) {
	t.Parallel()
	m := fake.New()
	if err := m.Signal(context.Background(), "missing", os.Interrupt); !errors.Is(err, process.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestWaitBlocksUntilStop(t *testing.T) {
	t.Parallel()
	m := fake.New()
	ctx := context.Background()
	h, err := m.Start(ctx, process.StartSpec{
		ID:      "proc-01ARZ3NDEKTSV4RRFFQ69G5FAV",
		Command: []string{"sleep", "99"},
	})
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan *process.Handle, 1)
	errCh := make(chan error, 1)
	go func() {
		wh, werr := m.Wait(ctx, h.ID)
		errCh <- werr
		done <- wh
	}()

	// Give Wait a moment to block.
	time.Sleep(20 * time.Millisecond)
	select {
	case <-done:
		t.Fatal("Wait returned before Stop")
	default:
	}

	if err := m.Stop(ctx, h.ID, time.Second); err != nil {
		t.Fatal(err)
	}
	select {
	case werr := <-errCh:
		if werr != nil {
			t.Fatalf("Wait: %v", werr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Wait did not complete after Stop")
	}
}
