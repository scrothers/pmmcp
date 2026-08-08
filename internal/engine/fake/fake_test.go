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
	"testing"
	"time"

	"github.com/scrothers/pmmcp/internal/engine"
	"github.com/scrothers/pmmcp/internal/engine/fake"
)

func TestRunStopLogs(t *testing.T) {
	t.Parallel()
	e := fake.New()
	ctx := context.Background()

	if e.Name() != "fake" {
		t.Fatalf("Name = %q, want fake", e.Name())
	}
	if !e.Available(ctx) {
		t.Fatal("Available = false, want true")
	}

	id, err := e.Run(ctx, engine.RunSpec{
		Name:  "db",
		Image: "postgres:16",
		Env:   map[string]string{"POSTGRES_PASSWORD": "x"},
		Ports: []string{"5432:5432"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if id == "" {
		t.Fatal("empty container id")
	}
	if len(e.Runs) != 1 {
		t.Fatalf("Runs len = %d, want 1", len(e.Runs))
	}

	logs, err := e.Logs(ctx, id, 10)
	if err != nil {
		t.Fatalf("Logs: %v", err)
	}
	if logs == "" {
		t.Fatal("empty logs")
	}

	if err := e.Stop(ctx, id, time.Second); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}

func TestRunInvalidAndNotFound(t *testing.T) {
	t.Parallel()
	e := fake.New()
	ctx := context.Background()

	if _, err := e.Run(ctx, engine.RunSpec{}); !errors.Is(err, engine.ErrInvalidSpec) {
		t.Fatalf("empty image err = %v, want ErrInvalidSpec", err)
	}
	if err := e.Stop(ctx, "missing", 0); !errors.Is(err, engine.ErrNotFound) {
		t.Fatalf("Stop missing err = %v, want ErrNotFound", err)
	}
	if _, err := e.Logs(ctx, "missing", 0); !errors.Is(err, engine.ErrNotFound) {
		t.Fatalf("Logs missing err = %v, want ErrNotFound", err)
	}
}

func TestAvailable(t *testing.T) {
	t.Parallel()
	t.Run("nil func, live ctx", func(t *testing.T) {
		t.Parallel()
		e := fake.New()
		if !e.Available(context.Background()) {
			t.Fatal("Available = false, want true")
		}
	})
	t.Run("nil func, canceled ctx", func(t *testing.T) {
		t.Parallel()
		e := fake.New()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if e.Available(ctx) {
			t.Fatal("Available = true, want false for canceled ctx")
		}
	})
	t.Run("func set, returns true", func(t *testing.T) {
		t.Parallel()
		e := fake.New()
		e.AvailableFunc = func(context.Context) bool { return true }
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		// AvailableFunc overrides the ctx check entirely.
		if !e.Available(ctx) {
			t.Fatal("Available = false, want true (AvailableFunc overrides ctx state)")
		}
	})
	t.Run("func set, returns false", func(t *testing.T) {
		t.Parallel()
		e := fake.New()
		e.AvailableFunc = func(context.Context) bool { return false }
		if e.Available(context.Background()) {
			t.Fatal("Available = true, want false")
		}
	})
}

func TestRunCtxCanceled(t *testing.T) {
	t.Parallel()
	e := fake.New()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := e.Run(ctx, engine.RunSpec{Image: "img"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("Run err = %v, want context.Canceled", err)
	}
}

func TestRunErr(t *testing.T) {
	t.Parallel()
	e := fake.New()
	wantErr := errors.New("boom")
	e.RunErr = wantErr
	if _, err := e.Run(context.Background(), engine.RunSpec{Image: "img"}); !errors.Is(err, wantErr) {
		t.Fatalf("Run err = %v, want %v", err, wantErr)
	}
}

func TestStopCtxCanceled(t *testing.T) {
	t.Parallel()
	e := fake.New()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := e.Stop(ctx, "anything", 0); !errors.Is(err, context.Canceled) {
		t.Fatalf("Stop err = %v, want context.Canceled", err)
	}
}

func TestStopErr(t *testing.T) {
	t.Parallel()
	e := fake.New()
	wantErr := errors.New("boom")
	e.StopErr = wantErr
	id, err := e.Run(context.Background(), engine.RunSpec{Image: "img"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if err := e.Stop(context.Background(), id, 0); !errors.Is(err, wantErr) {
		t.Fatalf("Stop err = %v, want %v", err, wantErr)
	}
}

func TestLogsCtxCanceled(t *testing.T) {
	t.Parallel()
	e := fake.New()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := e.Logs(ctx, "anything", 0); !errors.Is(err, context.Canceled) {
		t.Fatalf("Logs err = %v, want context.Canceled", err)
	}
}

// TestInspectAndExit covers the fake's Inspector: a fresh container is running;
// after Exit it reports exited with the given code, OOM flag, and health.
func TestInspectAndExit(t *testing.T) {
	t.Parallel()
	e := fake.New()
	ctx := context.Background()
	id, err := e.Run(ctx, engine.RunSpec{Name: "svc", Image: "img", Labels: map[string]string{"io.pmmcp.proc_id": "p1"}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	e.SetHealth(id, "healthy")
	st, err := e.Inspect(ctx, id)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if !st.Running || st.State != engine.StateRunning || st.Name != "svc" || st.Health != "healthy" {
		t.Fatalf("running inspect = %+v", st)
	}
	if st.Labels["io.pmmcp.proc_id"] != "p1" {
		t.Fatalf("labels = %v", st.Labels)
	}

	e.Exit(id, 137, true)
	st, err = e.Inspect(ctx, id)
	if err != nil {
		t.Fatalf("Inspect after exit: %v", err)
	}
	if st.Running || st.State != engine.StateExited || st.ExitCode != 137 || !st.OOMKilled {
		t.Fatalf("exited inspect = %+v", st)
	}

	if _, err := e.Inspect(ctx, "missing"); !errors.Is(err, engine.ErrNotFound) {
		t.Fatalf("Inspect missing err = %v, want ErrNotFound", err)
	}
}

// TestWait covers the fake's Waiter: it returns immediately once exited, and
// blocks (respecting ctx) until a concurrent Exit unblocks it.
func TestWait(t *testing.T) {
	t.Parallel()
	e := fake.New()
	ctx := context.Background()
	id, err := e.Run(ctx, engine.RunSpec{Image: "img"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Blocks until Exit, then returns the code.
	go func() {
		time.Sleep(5 * time.Millisecond)
		e.Exit(id, 3, false)
	}()
	code, err := e.Wait(ctx, id)
	if err != nil || code != 3 {
		t.Fatalf("Wait = %d err=%v, want 3", code, err)
	}

	// Already exited: returns immediately.
	if code, err := e.Wait(ctx, id); err != nil || code != 3 {
		t.Fatalf("Wait (already exited) = %d err=%v", code, err)
	}

	if _, err := e.Wait(ctx, "missing"); !errors.Is(err, engine.ErrNotFound) {
		t.Fatalf("Wait missing err = %v, want ErrNotFound", err)
	}

	t.Run("ctx canceled while blocked", func(t *testing.T) {
		t.Parallel()
		e := fake.New()
		id, _ := e.Run(context.Background(), engine.RunSpec{Image: "img"})
		cctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := e.Wait(cctx, id); !errors.Is(err, context.Canceled) {
			t.Fatalf("Wait err = %v, want context.Canceled", err)
		}
	})
}

// TestRemovePullListVersion covers the remaining fake capabilities.
func TestRemovePullListVersion(t *testing.T) {
	t.Parallel()
	e := fake.New()
	e.VersionInfo = engine.VersionInfo{Client: "1.0", Server: "1.0"}
	ctx := context.Background()

	id, _ := e.Run(ctx, engine.RunSpec{Image: "img", Labels: map[string]string{"role": "db"}})

	// Remove refuses a running container without force.
	if err := e.Remove(ctx, id, false); err == nil {
		t.Fatal("Remove running without force should fail")
	}
	if err := e.Remove(ctx, id, true); err != nil {
		t.Fatalf("Remove force: %v", err)
	}
	if _, err := e.Inspect(ctx, id); !errors.Is(err, engine.ErrNotFound) {
		t.Fatalf("container should be gone after remove: %v", err)
	}

	if err := e.PullImage(ctx, "alpine"); err != nil {
		t.Fatalf("PullImage: %v", err)
	}
	if len(e.Pulled) != 1 || e.Pulled[0] != "alpine" {
		t.Fatalf("Pulled = %v", e.Pulled)
	}
	if err := e.PullImage(ctx, ""); !errors.Is(err, engine.ErrInvalidSpec) {
		t.Fatalf("PullImage empty err = %v", err)
	}

	// List filters by label.
	_, _ = e.Run(ctx, engine.RunSpec{Image: "img", Labels: map[string]string{"role": "db"}})
	_, _ = e.Run(ctx, engine.RunSpec{Image: "img", Labels: map[string]string{"role": "web"}})
	got, err := e.List(ctx, map[string]string{"role": "db"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 || got[0].Labels["role"] != "db" {
		t.Fatalf("List = %+v, want 1 db container", got)
	}

	if v, err := e.Version(ctx); err != nil || v.Server != "1.0" {
		t.Fatalf("Version = %+v err=%v", v, err)
	}
}

// TestCapabilityErrOverrides covers the injectable error fields.
func TestCapabilityErrOverrides(t *testing.T) {
	t.Parallel()
	sentinel := errors.New("boom")
	e := fake.New()
	e.InspectErr, e.WaitErr, e.RemoveErr, e.PullErr, e.ListErr, e.VersionErr = sentinel, sentinel, sentinel, sentinel, sentinel, sentinel
	ctx := context.Background()

	if _, err := e.Inspect(ctx, "x"); !errors.Is(err, sentinel) {
		t.Errorf("Inspect err = %v", err)
	}
	if _, err := e.Wait(ctx, "x"); !errors.Is(err, sentinel) {
		t.Errorf("Wait err = %v", err)
	}
	if err := e.Remove(ctx, "x", false); !errors.Is(err, sentinel) {
		t.Errorf("Remove err = %v", err)
	}
	if err := e.PullImage(ctx, "img"); !errors.Is(err, sentinel) {
		t.Errorf("PullImage err = %v", err)
	}
	if _, err := e.List(ctx, nil); !errors.Is(err, sentinel) {
		t.Errorf("List err = %v", err)
	}
	if _, err := e.Version(ctx); !errors.Is(err, sentinel) {
		t.Errorf("Version err = %v", err)
	}
}
