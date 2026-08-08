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

package supervise

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"time"
)

// ErrSSRF is returned when a health probe target is blocked by the loopback
// SSRF policy (non-loopback host without an explicit allow).
var ErrSSRF = errors.New("supervise: ssrf blocked")

// RestartPolicy controls automatic restart after crash or unhealthy.
type RestartPolicy struct {
	Enabled bool
	Max     int // 0 with Enabled means unlimited (caller may still cap)
	Backoff time.Duration
	// StableWindow is how long a process must stay healthy before the restart
	// counter resets to zero. Zero defaults to 30s in RestartCounter.
	StableWindow time.Duration
	// Multiplier, when > 1, makes NextBackoff exponential (initial * mult^n).
	// Zero or <= 1 keeps the backward-compatible linear schedule.
	Multiplier float64
	// MaxBackoff caps the computed backoff interval. Zero means uncapped.
	MaxBackoff time.Duration
	// Jitter is the fractional +/- jitter applied to the backoff (0..1). Zero
	// keeps the schedule deterministic.
	Jitter float64
}

// RestartCounter tracks restarts and applies the stable-window reset rule.
type RestartCounter struct {
	policy      RestartPolicy
	count       int
	healthyFor  time.Duration // accumulated continuous healthy time
	lastTick    time.Time
	initialized bool
}

// NewRestartCounter builds a counter for policy.
func NewRestartCounter(policy RestartPolicy) *RestartCounter {
	if policy.StableWindow <= 0 {
		policy.StableWindow = 30 * time.Second
	}
	return &RestartCounter{policy: policy}
}

// Count returns the current restart count in the window.
func (c *RestartCounter) Count() int { return c.count }

// ObserveHealthy records that the process was healthy for the duration since last observe.
// After StableWindow of continuous health, count resets to 0.
func (c *RestartCounter) ObserveHealthy(now time.Time) {
	if !c.initialized {
		c.lastTick = now
		c.initialized = true
		return
	}
	delta := now.Sub(c.lastTick)
	if delta < 0 {
		delta = 0
	}
	c.lastTick = now
	c.healthyFor += delta
	if c.healthyFor >= c.policy.StableWindow {
		c.count = 0
		c.healthyFor = 0
	}
}

// ObserveUnhealthy clears the healthy accumulation and resets the stable-window
// anchor, so only runtime observed after the next confirmed-healthy tick counts
// toward the reset (downtime is never credited).
func (c *RestartCounter) ObserveUnhealthy(now time.Time) {
	c.healthyFor = 0
	c.lastTick = now
	c.initialized = false
}

// RecordRestart increments the counter after a restart attempt and re-arms the
// stable-window anchor, so backoff/restart downtime is not credited as healthy
// uptime on the next ObserveHealthy.
func (c *RestartCounter) RecordRestart() {
	c.count++
	c.healthyFor = 0
	c.initialized = false
}

// ShouldRestart reports whether another restart is allowed for this counter.
func (c *RestartCounter) ShouldRestart() bool {
	return ShouldRestart(c.policy, c.count)
}

// HealthCheck describes a periodic readiness/liveness probe.
type HealthCheck struct {
	Type     string // http | tcp | exec
	Target   string
	Interval time.Duration
	Timeout  time.Duration
	Retries  int
}

// ShouldRestart reports whether another restart is allowed given how many
// restarts have already been performed in the current window.
// restarts is the count of restarts already done (0 = first failure, not yet restarted).
func ShouldRestart(policy RestartPolicy, restarts int) bool {
	if !policy.Enabled {
		return false
	}
	if restarts < 0 {
		return false
	}
	if policy.Max <= 0 {
		// Unlimited when enabled and Max is 0 or negative.
		return true
	}
	return restarts < policy.Max
}

// NextBackoff returns the delay before the next restart attempt.
//
// With no Multiplier/MaxBackoff/Jitter set the schedule is linear
// (policy.Backoff * (restarts+1)), preserving the historical behavior. When any
// of those fields are set it becomes exponential: min(initial * mult^restarts,
// MaxBackoff) with an optional +/- Jitter fraction. When Backoff is zero it
// returns zero (immediate retry).
func NextBackoff(policy RestartPolicy, restarts int) time.Duration {
	if policy.Backoff <= 0 {
		return 0
	}
	n := restarts
	if n < 0 {
		n = 0
	}
	// Backward-compatible linear schedule when no exponential fields are set.
	if policy.Multiplier <= 1 && policy.MaxBackoff <= 0 && policy.Jitter <= 0 {
		return policy.Backoff * time.Duration(n+1)
	}
	mult := policy.Multiplier
	if mult < 1 {
		mult = 1
	}
	d := float64(policy.Backoff)
	capped := float64(policy.MaxBackoff)
	for range n {
		d *= mult
		if policy.MaxBackoff > 0 && d >= capped {
			d = capped
			break
		}
	}
	if policy.MaxBackoff > 0 && d > capped {
		d = capped
	}
	if policy.Jitter > 0 {
		j := policy.Jitter
		if j > 1 {
			j = 1
		}
		// j is clamped to [0,1] above, so the worst case (rand.Float64()==0,
		// giving a -1 factor) yields d*(1-j) >= 0: never negative.
		d += d * j * (rand.Float64()*2 - 1) //nolint:gosec // jitter is not security-sensitive
	}
	return time.Duration(d)
}

// ProbeResult is the outcome of a single health probe.
type ProbeResult struct {
	OK      bool
	Message string
}

// ProbeOptions tunes health-probe behavior.
type ProbeOptions struct {
	// AllowNonLoopback disables the loopback SSRF guard, permitting probes to
	// hosts that do not resolve to loopback. Off by default
	// (supervision-healthchecks design: probes are loopback-bound unless
	// explicitly allowed).
	AllowNonLoopback bool
}

// ProbeHTTP performs an HTTP GET against url with the given timeout, using the
// default loopback-only SSRF policy. Status codes in [200, 400) are healthy.
func ProbeHTTP(ctx context.Context, url string, timeout time.Duration) ProbeResult {
	return ProbeHTTPOpts(ctx, url, timeout, ProbeOptions{})
}

// ProbeHTTPOpts performs an HTTP GET with configurable SSRF policy.
//
// Unless opts.AllowNonLoopback is set, the target host must resolve only to
// loopback addresses, redirects are re-validated against the same policy, and
// the dial is re-checked at connect time (closing DNS-rebinding via redirects or
// multi-A responses). Proxy environment variables are ignored for probes.
func ProbeHTTPOpts(ctx context.Context, rawURL string, timeout time.Duration, opts ProbeOptions) ProbeResult {
	if rawURL == "" {
		return ProbeResult{OK: false, Message: "empty url"}
	}
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return ProbeResult{OK: false, Message: fmt.Sprintf("parse url: %v", err)}
	}
	if !opts.AllowNonLoopback {
		if err := validateLoopbackURL(ctx, u); err != nil {
			return ProbeResult{OK: false, Message: err.Error()}
		}
	}
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, rawURL, nil)
	if err != nil {
		return ProbeResult{OK: false, Message: fmt.Sprintf("build request: %v", err)}
	}
	dialer := &net.Dialer{Timeout: timeout, Control: loopbackDialControl(opts.AllowNonLoopback)}
	client := &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			Proxy:       nil, // ignore proxy env for probes
			DialContext: dialer.DialContext,
		},
		CheckRedirect: func(r *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("supervise: stopped after 10 redirects")
			}
			if opts.AllowNonLoopback {
				return nil
			}
			return validateLoopbackURL(r.Context(), r.URL)
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return ProbeResult{OK: false, Message: err.Error()}
	}
	defer func() { _ = resp.Body.Close() }()
	// Drain body so connection can be reused if client is pooled later.
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))

	if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusBadRequest {
		return ProbeResult{OK: true, Message: resp.Status}
	}
	return ProbeResult{OK: false, Message: resp.Status}
}

// lookupIPAddr resolves host addresses for the loopback SSRF guard. It is a
// variable (rather than a direct net.DefaultResolver.LookupIPAddr call) so
// tests can substitute a deterministic fake resolver via export_test.go.
var lookupIPAddr = net.DefaultResolver.LookupIPAddr

// validateLoopbackURL rejects any target whose scheme is not http/https or that
// resolves to a non-loopback address.
func validateLoopbackURL(ctx context.Context, u *url.URL) error {
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
	default:
		return fmt.Errorf("%w: scheme %q not allowed", ErrSSRF, u.Scheme)
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("%w: empty host", ErrSSRF)
	}
	if ip := net.ParseIP(host); ip != nil {
		if !ip.IsLoopback() {
			return fmt.Errorf("%w: %s is not loopback", ErrSSRF, ip)
		}
		return nil
	}
	ips, err := lookupIPAddr(ctx, host)
	if err != nil {
		return fmt.Errorf("%w: resolve %q: %w", ErrSSRF, host, err)
	}
	if len(ips) == 0 {
		return fmt.Errorf("%w: %q has no addresses", ErrSSRF, host)
	}
	for _, ipa := range ips {
		if !ipa.IP.IsLoopback() {
			return fmt.Errorf("%w: %q resolves to non-loopback %s", ErrSSRF, host, ipa.IP)
		}
	}
	return nil
}

// loopbackDialControl returns a net.Dialer Control that rejects a dial to any
// non-loopback address, defeating DNS rebinding regardless of redirect or
// multi-A resolution. When allow is true it permits any address.
func loopbackDialControl(allow bool) func(network, address string, c syscall.RawConn) error {
	return func(_, address string, _ syscall.RawConn) error {
		if allow {
			return nil
		}
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			host = address
		}
		ip := net.ParseIP(host)
		if ip == nil || !ip.IsLoopback() {
			return fmt.Errorf("%w: dial %s", ErrSSRF, address)
		}
		return nil
	}
}
