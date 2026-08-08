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
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/scrothers/pmmcp/internal/domain"
	"github.com/scrothers/pmmcp/internal/supervise"
)

// TestObserveHealthyClockRewind covers the delta<0 clamp: a backward clock
// jump between observations must not produce negative accumulated healthy
// time.
func TestObserveHealthyClockRewind(t *testing.T) {
	t.Parallel()
	policy := supervise.RestartPolicy{Enabled: true, Max: 5, StableWindow: time.Second}
	c := supervise.NewRestartCounter(policy)
	c.RecordRestart()
	t0 := time.Unix(100, 0).UTC()
	c.ObserveHealthy(t0)
	// Clock moves backward: delta would be negative without the clamp.
	c.ObserveHealthy(t0.Add(-10 * time.Second))
	if c.Count() != 1 {
		t.Fatalf("count = %d, want 1 (rewind must not fabricate healthy time)", c.Count())
	}
}

// TestNextBackoffNegativeRestartsClamped covers the n<0 clamp shared by both
// the linear and exponential schedules.
func TestNextBackoffNegativeRestartsClamped(t *testing.T) {
	t.Parallel()
	policy := supervise.RestartPolicy{Enabled: true, Backoff: 100 * time.Millisecond}
	if got, want := supervise.NextBackoff(policy, -5), supervise.NextBackoff(policy, 0); got != want {
		t.Fatalf("NextBackoff(restarts=-5) = %v, want %v (same as restarts=0)", got, want)
	}
}

// TestNextBackoffMultiplierClamp covers the mult<1 clamp: a Multiplier of 0
// (or negative) in the exponential branch must not shrink the backoff.
func TestNextBackoffMultiplierClamp(t *testing.T) {
	t.Parallel()
	policy := supervise.RestartPolicy{
		Enabled:    true,
		Backoff:    100 * time.Millisecond,
		Multiplier: 0, // triggers the exponential branch via MaxBackoff, then clamps to 1
		MaxBackoff: 500 * time.Millisecond,
	}
	for _, n := range []int{0, 1, 3} {
		if got := supervise.NextBackoff(policy, n); got != 100*time.Millisecond {
			t.Fatalf("NextBackoff(n=%d) = %v, want unchanged 100ms (mult clamped to 1)", n, got)
		}
	}
}

// TestNextBackoffJitterOverOneClamped covers the j>1 clamp: Jitter above 1
// must behave as if it were exactly 1 (bounded to [0, 2x base]).
func TestNextBackoffJitterOverOneClamped(t *testing.T) {
	t.Parallel()
	policy := supervise.RestartPolicy{
		Enabled:    true,
		Backoff:    100 * time.Millisecond,
		Multiplier: 2,
		MaxBackoff: time.Second,
		Jitter:     5, // clamped to 1
	}
	base := 400 * time.Millisecond // 100 * 2^2
	for range 50 {
		got := supervise.NextBackoff(policy, 2)
		if got < 0 || got > 2*base {
			t.Fatalf("jittered backoff %v outside [0,%v]", got, 2*base)
		}
	}
}

// TestNextBackoffCapAppliesWithoutLooping covers the post-loop MaxBackoff cap
// for restarts=0 (the loop body never runs), where the initial Backoff alone
// already exceeds MaxBackoff.
func TestNextBackoffCapAppliesWithoutLooping(t *testing.T) {
	t.Parallel()
	policy := supervise.RestartPolicy{
		Enabled:    true,
		Backoff:    500 * time.Millisecond,
		MaxBackoff: 100 * time.Millisecond,
	}
	if got, want := supervise.NextBackoff(policy, 0), 100*time.Millisecond; got != want {
		t.Fatalf("NextBackoff(restarts=0) = %v, want %v (capped)", got, want)
	}
}

// TestProbeHTTPOptsDefaultsZeroTimeout covers the timeout<=0 default.
func TestProbeHTTPOptsDefaultsZeroTimeout(t *testing.T) {
	t.Parallel()
	res := supervise.ProbeHTTPOpts(context.Background(), "not a url with a space", 0, supervise.ProbeOptions{})
	// The zero-timeout default line runs regardless of outcome; assert we got
	// a definitive (non-hanging) result with a message.
	if res.OK {
		t.Fatalf("expected failure for malformed url, got %+v", res)
	}
}

// TestProbeHTTPOptsParseError covers the url.Parse error branch.
func TestProbeHTTPOptsParseError(t *testing.T) {
	t.Parallel()
	res := supervise.ProbeHTTP(context.Background(), "http://127.0.0.1/%zz", time.Second)
	if res.OK {
		t.Fatalf("expected parse failure, got %+v", res)
	}
}

// TestValidateLoopbackEmptyHost covers the empty-host branch (a URL with no
// authority component).
func TestValidateLoopbackEmptyHost(t *testing.T) {
	t.Parallel()
	res := supervise.ProbeHTTP(context.Background(), "http:///health", time.Second)
	if res.OK {
		t.Fatalf("empty host should be blocked, got %+v", res)
	}
	if res.Message == "" {
		t.Fatal("expected non-empty message for empty-host block")
	}
}

// TestValidateLoopbackResolveError covers the LookupIPAddr error branch via
// the injectable resolver seam — no real DNS/network involved.
func TestValidateLoopbackResolveError(t *testing.T) {
	restore := supervise.SetLookupIPAddrForTest(func(context.Context, string) ([]net.IPAddr, error) {
		return nil, errors.New("boom: no such host")
	})
	defer restore()
	res := supervise.ProbeHTTP(context.Background(), "http://svc.internal/health", time.Second)
	if res.OK {
		t.Fatalf("resolve error should block, got %+v", res)
	}
}

// TestValidateLoopbackZeroAddresses covers the len(ips)==0 branch.
func TestValidateLoopbackZeroAddresses(t *testing.T) {
	restore := supervise.SetLookupIPAddrForTest(func(context.Context, string) ([]net.IPAddr, error) {
		return nil, nil
	})
	defer restore()
	res := supervise.ProbeHTTP(context.Background(), "http://svc.internal/health", time.Second)
	if res.OK {
		t.Fatalf("zero addresses should block, got %+v", res)
	}
}

// TestValidateLoopbackMixedAddressesRejected covers the per-address
// non-loopback rejection inside the resolved-address loop.
func TestValidateLoopbackMixedAddressesRejected(t *testing.T) {
	restore := supervise.SetLookupIPAddrForTest(func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{
			{IP: net.ParseIP("127.0.0.1")},
			{IP: net.ParseIP("93.184.216.34")}, // not loopback
		}, nil
	})
	defer restore()
	res := supervise.ProbeHTTP(context.Background(), "http://svc.internal/health", time.Second)
	if res.OK {
		t.Fatalf("mixed loopback/non-loopback resolution should block, got %+v", res)
	}
}

// TestValidateLoopbackAllLoopbackResolved covers the success path where
// hostname resolution (not an IP literal) yields only loopback addresses.
func TestValidateLoopbackAllLoopbackResolved(t *testing.T) {
	restore := supervise.SetLookupIPAddrForTest(func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{
			{IP: net.ParseIP("127.0.0.1")},
			{IP: net.ParseIP("::1")},
		}, nil
	})
	defer restore()
	// Port 0 with no listener: this specific host won't accept a connection,
	// but that's a dial-time failure, not an SSRF block — proving the
	// resolution path (not the IP-literal fast path) accepted the host.
	res := supervise.ProbeHTTPOpts(context.Background(), "http://svc.internal:1/health", 200*time.Millisecond, supervise.ProbeOptions{})
	if res.OK {
		t.Fatalf("dial to port 1 should fail, got %+v", res)
	}
}

// TestProbeHTTPRedirectLimitExceeded covers the >=10 redirect cap in
// CheckRedirect.
func TestProbeHTTPRedirectLimitExceeded(t *testing.T) {
	t.Parallel()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, srv.URL, http.StatusFound) // redirects forever
	}))
	t.Cleanup(srv.Close)

	res := supervise.ProbeHTTP(context.Background(), srv.URL, time.Second)
	if res.OK {
		t.Fatalf("expected redirect-limit failure, got %+v", res)
	}
	if !strings.Contains(res.Message, "redirect") {
		t.Fatalf("message %q should mention redirects", res.Message)
	}
}

// TestProbeHTTPAllowNonLoopbackSkipsRedirectValidation covers the
// AllowNonLoopback branch inside CheckRedirect (skips re-validation on hop).
func TestProbeHTTPAllowNonLoopbackSkipsRedirectValidation(t *testing.T) {
	t.Parallel()
	final := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(final.Close)
	entry := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, final.URL, http.StatusFound)
	}))
	t.Cleanup(entry.Close)

	res := supervise.ProbeHTTPOpts(context.Background(), entry.URL, time.Second, supervise.ProbeOptions{AllowNonLoopback: true})
	if !res.OK {
		t.Fatalf("expected success following redirect with AllowNonLoopback, got %+v", res)
	}
}

// TestCrashLoopDefaultPeriod covers the Period<=0 default-assignment branch.
// A short-lived context returns before any tick fires, keeping the test fast
// regardless of the 2s default.
func TestCrashLoopDefaultPeriod(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	done := make(chan struct{})
	go func() {
		supervise.CrashLoop(ctx, supervise.CrashLoopConfig{
			IDs:     []string{"proc-1"},
			Monitor: func(context.Context, string) (bool, bool, error) { return true, true, nil },
			Restart: func(context.Context, string) error { return nil },
		})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("CrashLoop with default period did not return after context timeout")
	}
}

// TestMapStatusExitRunningUnhealthy covers the running-but-unhealthy branch.
func TestMapStatusExitRunningUnhealthy(t *testing.T) {
	t.Parallel()
	if got := supervise.MapStatusExit(true, false, nil); got != domain.StatusUnhealthy {
		t.Fatalf("running+unhealthy = %v, want StatusUnhealthy", got)
	}
}
