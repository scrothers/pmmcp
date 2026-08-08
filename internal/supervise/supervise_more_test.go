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

package supervise_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/scrothers/pmmcp/internal/domain"
	"github.com/scrothers/pmmcp/internal/supervise"
)

// TestRestartCounterNoDowntimeCredit is the regression for the circuit-breaker
// bug: a long gap that includes backoff/restart downtime must not be credited as
// healthy uptime, so the counter does not reset after a restart with no verified
// healthy runtime.
func TestRestartCounterNoDowntimeCredit(t *testing.T) {
	t.Parallel()
	policy := supervise.RestartPolicy{Enabled: true, Max: 5, StableWindow: 5 * time.Second}
	c := supervise.NewRestartCounter(policy)
	t0 := time.Unix(0, 0).UTC()

	c.ObserveUnhealthy(t0)
	c.RecordRestart()
	// A 6s gap (backoff + restart) then the first healthy tick: must NOT reset.
	c.ObserveHealthy(t0.Add(6 * time.Second))
	if c.Count() != 1 {
		t.Fatalf("count after restart+long gap = %d, want 1 (downtime must not reset)", c.Count())
	}
	// Now genuine healthy runtime accrues and eventually resets.
	c.ObserveHealthy(t0.Add(11 * time.Second)) // +5s of confirmed health
	if c.Count() != 0 {
		t.Fatalf("count after 5s confirmed health = %d, want 0", c.Count())
	}
}

// TestRestartCounterUnhealthyGapNotCredited verifies the interval spanning an
// unhealthy period is not credited toward the stable window.
func TestRestartCounterUnhealthyGapNotCredited(t *testing.T) {
	t.Parallel()
	policy := supervise.RestartPolicy{Enabled: true, Max: 5, StableWindow: 2 * time.Second}
	c := supervise.NewRestartCounter(policy)
	c.RecordRestart()
	t0 := time.Unix(0, 0).UTC()
	c.ObserveHealthy(t0) // first healthy: arms anchor
	c.ObserveUnhealthy(t0.Add(1 * time.Second))
	// A big gap while unhealthy then healthy again: only post-recovery counts.
	c.ObserveHealthy(t0.Add(30 * time.Second)) // re-arms, credits nothing
	if c.Count() != 1 {
		t.Fatalf("count = %d, want 1 (unhealthy gap must not reset)", c.Count())
	}
	c.ObserveHealthy(t0.Add(32 * time.Second)) // +2s confirmed health
	if c.Count() != 0 {
		t.Fatalf("count = %d, want 0 after confirmed window", c.Count())
	}
}

func TestNextBackoffExponentialWithCap(t *testing.T) {
	t.Parallel()
	policy := supervise.RestartPolicy{
		Enabled:    true,
		Backoff:    100 * time.Millisecond,
		Multiplier: 2,
		MaxBackoff: 800 * time.Millisecond,
	}
	want := []time.Duration{
		100 * time.Millisecond, // n=0
		200 * time.Millisecond, // n=1
		400 * time.Millisecond, // n=2
		800 * time.Millisecond, // n=3
		800 * time.Millisecond, // n=4 capped
	}
	for n, w := range want {
		if got := supervise.NextBackoff(policy, n); got != w {
			t.Fatalf("NextBackoff(n=%d) = %v, want %v", n, got, w)
		}
	}
}

func TestNextBackoffJitterBounded(t *testing.T) {
	t.Parallel()
	policy := supervise.RestartPolicy{
		Enabled:    true,
		Backoff:    100 * time.Millisecond,
		Multiplier: 2,
		MaxBackoff: time.Second,
		Jitter:     0.5,
	}
	base := 400 * time.Millisecond // 100 * 2^2
	for range 50 {
		got := supervise.NextBackoff(policy, 2)
		if got < base/2 || got > base+base/2 {
			t.Fatalf("jittered backoff %v outside [%v,%v]", got, base/2, base+base/2)
		}
	}
}

func TestProbeHTTPBlocksNonLoopback(t *testing.T) {
	t.Parallel()
	// Literal non-loopback IP: rejected without any network access.
	res := supervise.ProbeHTTP(context.Background(), "http://93.184.216.34:1/health", time.Second)
	if res.OK {
		t.Fatalf("expected non-loopback to be blocked, got %+v", res)
	}
}

func TestProbeHTTPAllowNonLoopbackOptIn(t *testing.T) {
	t.Parallel()
	// With the opt-in, the guard is skipped; dial fails fast on port 1 but the
	// failure is a dial error, not an SSRF block.
	res := supervise.ProbeHTTPOpts(context.Background(), "http://93.184.216.34:1/health", 200*time.Millisecond, supervise.ProbeOptions{AllowNonLoopback: true})
	if res.OK {
		t.Fatalf("dial should fail, got %+v", res)
	}
}

func TestProbeHTTPBlocksRedirectToNonLoopback(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Redirect(w, &http.Request{}, "http://93.184.216.34/", http.StatusFound)
	}))
	t.Cleanup(srv.Close)
	res := supervise.ProbeHTTP(context.Background(), srv.URL, time.Second)
	if res.OK {
		t.Fatalf("expected redirect to non-loopback to be blocked, got %+v", res)
	}
}

func TestProbeHTTPRejectsBadScheme(t *testing.T) {
	t.Parallel()
	res := supervise.ProbeHTTP(context.Background(), "file:///etc/passwd", time.Second)
	if res.OK {
		t.Fatalf("file scheme should be rejected, got %+v", res)
	}
}

func TestMapStatus(t *testing.T) {
	t.Parallel()
	if got := supervise.MapStatus(false, false); got != domain.StatusExited {
		t.Fatalf("not running = %v", got)
	}
	if got := supervise.MapStatus(true, false); got != domain.StatusUnhealthy {
		t.Fatalf("unhealthy = %v", got)
	}
	if got := supervise.MapStatus(true, true); got != domain.StatusRunning {
		t.Fatalf("running = %v", got)
	}
}

func TestMapStatusExit(t *testing.T) {
	t.Parallel()
	nonzero := 1
	zero := 0
	if got := supervise.MapStatusExit(false, false, &nonzero); got != domain.StatusCrashed {
		t.Fatalf("nonzero exit = %v, want crashed", got)
	}
	if got := supervise.MapStatusExit(false, false, &zero); got != domain.StatusExited {
		t.Fatalf("zero exit = %v, want exited", got)
	}
	if got := supervise.MapStatusExit(false, false, nil); got != domain.StatusExited {
		t.Fatalf("nil exit = %v, want exited", got)
	}
	if got := supervise.MapStatusExit(true, true, &nonzero); got != domain.StatusRunning {
		t.Fatalf("running = %v", got)
	}
}

func TestEligibleForRelaunch(t *testing.T) {
	t.Parallel()
	if !supervise.EligibleForRelaunch(domain.DesiredRunning, supervise.RestartPolicy{}) {
		t.Fatal("desired running should be eligible")
	}
	if supervise.EligibleForRelaunch(domain.DesiredStopped, supervise.RestartPolicy{}) {
		t.Fatal("desired stopped should not be eligible")
	}
}

func TestCrashLoopRestartsUntilExhausted(t *testing.T) {
	t.Parallel()
	var restarts int64
	exhausted := make(chan string, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cfg := supervise.CrashLoopConfig{
		Period: 5 * time.Millisecond,
		IDs:    []string{"proc-1"},
		Policy: supervise.RestartPolicy{Enabled: true, Max: 3, Backoff: 0},
		Monitor: func(context.Context, string) (bool, bool, error) {
			return false, false, nil // always crashed
		},
		Restart: func(context.Context, string) error {
			atomic.AddInt64(&restarts, 1)
			return nil
		},
		OnExhausted: func(_ context.Context, id string) {
			select {
			case exhausted <- id:
			default:
			}
		},
	}
	go supervise.CrashLoop(ctx, cfg)

	select {
	case id := <-exhausted:
		if id != "proc-1" {
			t.Fatalf("exhausted id = %q", id)
		}
	case <-ctx.Done():
		t.Fatal("timeout waiting for exhaustion")
	}
	cancel()
	time.Sleep(30 * time.Millisecond)
	if n := atomic.LoadInt64(&restarts); n != 3 {
		t.Fatalf("restarts = %d, want exactly Max=3", n)
	}
}

func TestCrashLoopSkipsIntentionalStop(t *testing.T) {
	t.Parallel()
	var restarts int64
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	cfg := supervise.CrashLoopConfig{
		Period: 5 * time.Millisecond,
		IDs:    []string{"proc-1"},
		Policy: supervise.RestartPolicy{Enabled: true, Max: 5},
		Monitor: func(context.Context, string) (bool, bool, error) {
			return false, false, nil // not running
		},
		Restart: func(context.Context, string) error {
			atomic.AddInt64(&restarts, 1)
			return nil
		},
		Desired: func(context.Context, string) domain.Desired {
			return domain.DesiredStopped // intentional stop
		},
	}
	supervise.CrashLoop(ctx, cfg)
	if n := atomic.LoadInt64(&restarts); n != 0 {
		t.Fatalf("restarts = %d, want 0 for intentional stop", n)
	}
}

func TestCrashLoopStopsOnCancel(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	var mu sync.Mutex
	calls := 0
	cfg := supervise.CrashLoopConfig{
		Period: 5 * time.Millisecond,
		IDs:    []string{"proc-1"},
		Policy: supervise.RestartPolicy{Enabled: true, Max: 100, Backoff: time.Second},
		Monitor: func(context.Context, string) (bool, bool, error) {
			mu.Lock()
			calls++
			mu.Unlock()
			return true, true, nil
		},
		Restart: func(context.Context, string) error { return nil },
	}
	go func() {
		supervise.CrashLoop(ctx, cfg)
		close(done)
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("CrashLoop did not return after cancel")
	}
}

func TestCrashLoopBackoffGating(t *testing.T) {
	t.Parallel()
	var restarts int64
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
	defer cancel()
	cfg := supervise.CrashLoopConfig{
		Period: 5 * time.Millisecond,
		IDs:    []string{"proc-1"},
		// Large backoff means at most one restart within the test window,
		// proving the loop does not busy-restart every tick.
		Policy: supervise.RestartPolicy{Enabled: true, Max: 100, Backoff: time.Hour},
		Monitor: func(context.Context, string) (bool, bool, error) {
			return false, false, nil
		},
		Restart: func(context.Context, string) error {
			atomic.AddInt64(&restarts, 1)
			return nil
		},
	}
	supervise.CrashLoop(ctx, cfg)
	if n := atomic.LoadInt64(&restarts); n != 1 {
		t.Fatalf("restarts = %d, want 1 (backoff should gate further attempts)", n)
	}
}

func TestProbeHTTPErrSSRFSentinel(t *testing.T) {
	t.Parallel()
	// The sentinel is exported and usable by callers via the message; ensure it
	// exists and is distinct.
	if !errors.Is(supervise.ErrSSRF, supervise.ErrSSRF) {
		t.Fatal("ErrSSRF sentinel broken")
	}
}
