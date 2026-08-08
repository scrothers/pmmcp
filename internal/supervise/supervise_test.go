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
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/scrothers/pmmcp/internal/supervise"
)

func TestShouldRestart(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		policy   supervise.RestartPolicy
		restarts int
		want     bool
	}{
		{
			name:     "disabled",
			policy:   supervise.RestartPolicy{Enabled: false, Max: 5},
			restarts: 0,
			want:     false,
		},
		{
			name:     "enabled under max",
			policy:   supervise.RestartPolicy{Enabled: true, Max: 3},
			restarts: 0,
			want:     true,
		},
		{
			name:     "enabled at max-1",
			policy:   supervise.RestartPolicy{Enabled: true, Max: 3},
			restarts: 2,
			want:     true,
		},
		{
			name:     "enabled at max",
			policy:   supervise.RestartPolicy{Enabled: true, Max: 3},
			restarts: 3,
			want:     false,
		},
		{
			name:     "enabled past max",
			policy:   supervise.RestartPolicy{Enabled: true, Max: 3},
			restarts: 10,
			want:     false,
		},
		{
			name:     "unlimited max zero",
			policy:   supervise.RestartPolicy{Enabled: true, Max: 0},
			restarts: 100,
			want:     true,
		},
		{
			name:     "negative restarts",
			policy:   supervise.RestartPolicy{Enabled: true, Max: 3},
			restarts: -1,
			want:     false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := supervise.ShouldRestart(tt.policy, tt.restarts)
			if got != tt.want {
				t.Fatalf("ShouldRestart(%+v, %d) = %v, want %v", tt.policy, tt.restarts, got, tt.want)
			}
		})
	}
}

func TestRestartCounterStableWindowResets(t *testing.T) {
	t.Parallel()
	policy := supervise.RestartPolicy{Enabled: true, Max: 5, StableWindow: 2 * time.Second}
	c := supervise.NewRestartCounter(policy)
	c.RecordRestart()
	c.RecordRestart()
	if c.Count() != 2 {
		t.Fatalf("count = %d", c.Count())
	}
	t0 := time.Unix(0, 0).UTC()
	c.ObserveUnhealthy(t0)
	// healthy ticks totaling >= StableWindow
	c.ObserveHealthy(t0.Add(time.Second))
	if c.Count() != 2 {
		t.Fatalf("should not reset yet, count=%d", c.Count())
	}
	c.ObserveHealthy(t0.Add(2 * time.Second))
	// accumulated 2s of health
	c.ObserveHealthy(t0.Add(3 * time.Second))
	if c.Count() != 0 {
		t.Fatalf("stable window should reset count, got %d", c.Count())
	}
	if !c.ShouldRestart() {
		t.Fatal("after reset should allow restart")
	}
}

func TestNextBackoff(t *testing.T) {
	t.Parallel()
	base := time.Second
	policy := supervise.RestartPolicy{Enabled: true, Max: 5, Backoff: base}

	if got := supervise.NextBackoff(policy, 0); got != base {
		t.Fatalf("restarts=0: got %v, want %v", got, base)
	}
	if got := supervise.NextBackoff(policy, 1); got != 2*base {
		t.Fatalf("restarts=1: got %v, want %v", got, 2*base)
	}
	if got := supervise.NextBackoff(policy, 4); got != 5*base {
		t.Fatalf("restarts=4: got %v, want %v", got, 5*base)
	}

	zero := supervise.RestartPolicy{Enabled: true, Backoff: 0}
	if got := supervise.NextBackoff(zero, 3); got != 0 {
		t.Fatalf("zero backoff: got %v", got)
	}
}

func TestProbeHTTP_OK(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	t.Cleanup(srv.Close)

	ctx := context.Background()
	res := supervise.ProbeHTTP(ctx, srv.URL+"/healthz", time.Second)
	if !res.OK {
		t.Fatalf("ProbeHTTP healthy: %+v", res)
	}
}

func TestProbeHTTP_UnhealthyStatus(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(srv.Close)

	ctx := context.Background()
	res := supervise.ProbeHTTP(ctx, srv.URL, time.Second)
	if res.OK {
		t.Fatalf("expected unhealthy, got %+v", res)
	}
}

func TestProbeHTTP_Timeout(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	ctx := context.Background()
	res := supervise.ProbeHTTP(ctx, srv.URL, 20*time.Millisecond)
	if res.OK {
		t.Fatalf("expected timeout failure, got %+v", res)
	}
	if res.Message == "" {
		t.Fatal("expected non-empty message on timeout")
	}
}

func TestProbeHTTP_EmptyURL(t *testing.T) {
	t.Parallel()
	res := supervise.ProbeHTTP(context.Background(), "", time.Second)
	if res.OK {
		t.Fatal("empty url should fail")
	}
}
