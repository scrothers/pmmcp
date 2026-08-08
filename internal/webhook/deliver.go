// Copyright 2026 Steven Crothers
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"
)

// userAgent identifies webhook deliveries.
const userAgent = "pmmcp-webhook/1"

// maxRedirects bounds redirect chains; each hop is revalidated against policy.
const maxRedirects = 10

// Event is a typed delivery envelope.
type Event struct {
	// Type is the event type name, sent as X-PMMCP-Event-Type.
	Type string
	// Payload is JSON-marshaled into the request body.
	Payload any
}

// Deliverer posts JSON payloads to hooks under SSRF policy. Its zero value is
// usable: it validates the destination, pins the resolved IP at dial time,
// revalidates redirects, and drains a bounded response body.
type Deliverer struct {
	// Client is the HTTP client; nil builds a guarded client whose transport
	// resolves and validates every dialed IP. A caller-supplied client is used
	// as-is except that its redirect policy is replaced with revalidation; such
	// a client does NOT get dial-time IP pinning, so supply one only for tests
	// or when the transport already enforces egress policy.
	Client *http.Client
	// MaxBody is the max response body to drain (default 64KiB).
	MaxBody int64
	// Allowlist, when non-empty, additionally restricts delivery (and every
	// redirect hop) to matching destinations. Empty relies on registration-time
	// admission plus the always-on SSRF/IP checks.
	Allowlist []string

	// allowLoopback permits loopback destinations; test-only seam.
	allowLoopback bool
	// lookupIP overrides DNS resolution; test-only seam.
	lookupIP func(ctx context.Context, host string) ([]net.IP, error)
}

// Deliver POSTs payload as JSON to hook.URL with an empty event type.
func (d *Deliverer) Deliver(ctx context.Context, hook Hook, payload any) error {
	return d.DeliverEvent(ctx, hook, Event{Payload: payload})
}

// DeliverEvent POSTs ev to hook.URL.
//
// SSRF policy: only http/https; loopback, link-local, metadata, and
// unspecified addresses are blocked; the resolved IP is validated and pinned at
// dial time (DNS-rebinding safe); redirects are revalidated per hop; resolver
// failure fails closed. When hook.Secret is set the body is signed with
// HMAC-SHA256 (X-PMMCP-Signature: sha256=<hex>).
func (d *Deliverer) DeliverEvent(ctx context.Context, hook Hook, ev Event) error {
	if err := d.validate(hook.URL); err != nil {
		return err
	}
	body, err := json.Marshal(ev.Payload)
	if err != nil {
		return fmt.Errorf("webhook: marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, hook.URL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("webhook: request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("X-PMMCP-Event-Type", ev.Type)
	req.Header.Set("X-PMMCP-Delivery-Id", newDeliveryID())
	if hook.Secret != "" {
		mac := hmac.New(sha256.New, []byte(hook.Secret))
		_, _ = mac.Write(body)
		req.Header.Set("X-PMMCP-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	}

	resp, err := d.httpClient().Do(req)
	if err != nil {
		return fmt.Errorf("webhook: deliver: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	maxBody := d.MaxBody
	if maxBody <= 0 {
		maxBody = 64 * 1024
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxBody))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook: deliver: status %d", resp.StatusCode)
	}
	return nil
}

// validate applies scheme/host/allowlist policy (no DNS; IP pinning is at dial).
func (d *Deliverer) validate(raw string) error {
	u, err := parseHTTPURL(raw)
	if err != nil {
		return err
	}
	if len(d.Allowlist) > 0 && !allowlisted(raw, u, d.Allowlist) {
		return fmt.Errorf("%w: %q not on allowlist", ErrSSRF, u.Hostname())
	}
	if isBlockedHostname(u.Hostname()) {
		return fmt.Errorf("%w: host %q", ErrSSRF, u.Hostname())
	}
	if ip := net.ParseIP(u.Hostname()); ip != nil && d.blocked(ip) {
		return fmt.Errorf("%w: address %s", ErrSSRF, ip)
	}
	return nil
}

func (d *Deliverer) httpClient() *http.Client {
	if d.Client != nil {
		c := *d.Client
		c.CheckRedirect = d.checkRedirect
		return &c
	}
	return &http.Client{
		Timeout:       10 * time.Second,
		CheckRedirect: d.checkRedirect,
		Transport: &http.Transport{
			DialContext:           d.dialContext,
			MaxIdleConns:          10,
			IdleConnTimeout:       30 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: time.Second,
		},
	}
}

// checkRedirect revalidates each redirect hop's URL against policy and bounds
// the chain length. The dial-time pin re-checks IPs on the redirected dial.
func (d *Deliverer) checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= maxRedirects {
		return fmt.Errorf("webhook: too many redirects")
	}
	return d.validate(req.URL.String())
}

// dialContext resolves the host itself, validates every candidate IP, and dials
// a vetted IP literal (pin-after-resolve). Resolver failure fails closed.
func (d *Deliverer) dialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("webhook: dial addr: %w", err)
	}
	ips, err := d.resolve(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("%w: resolve %s: %w", ErrSSRF, host, err)
	}
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	var lastErr error
	for _, ip := range ips {
		if d.blocked(ip) {
			lastErr = fmt.Errorf("%w: address %s", ErrSSRF, ip)
			continue
		}
		conn, derr := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if derr != nil {
			lastErr = derr
			continue
		}
		return conn, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("%w: no permitted address for %s", ErrSSRF, host)
	}
	return nil, lastErr
}

// resolve returns the candidate IPs for host (a literal IP resolves to itself).
func (d *Deliverer) resolve(ctx context.Context, host string) ([]net.IP, error) {
	if ip := net.ParseIP(host); ip != nil {
		return []net.IP{ip}, nil
	}
	if d.lookupIP != nil {
		return d.lookupIP(ctx, host)
	}
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	ips := make([]net.IP, 0, len(addrs))
	for _, a := range addrs {
		ips = append(ips, a.IP)
	}
	return ips, nil
}

// blocked reports whether ip is disallowed for egress, honoring the test-only
// loopback allowance.
func (d *Deliverer) blocked(ip net.IP) bool {
	if d.allowLoopback && ip.IsLoopback() {
		return false
	}
	return isBlockedIP(ip)
}

// newDeliveryID returns a dlv-prefixed lowercase ULID.
func newDeliveryID() string {
	u, err := ulid.New(ulid.Timestamp(time.Now().UTC()), rand.Reader)
	if err != nil {
		return "dlv-" + strings.Repeat("0", 26)
	}
	return "dlv-" + strings.ToLower(u.String())
}

// Deliver is a package-level helper using a default Deliverer.
func Deliver(ctx context.Context, hook Hook, payload any) error {
	return (&Deliverer{}).Deliver(ctx, hook, payload)
}
