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

package webhook

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestDeliverAllowPath(t *testing.T) {
	t.Parallel()
	var gotType, gotSig, gotDelivery string
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s", r.Method)
		}
		gotType = r.Header.Get("X-PMMCP-Event-Type")
		gotSig = r.Header.Get("X-PMMCP-Signature")
		gotDelivery = r.Header.Get("X-PMMCP-Delivery-Id")
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	d := &Deliverer{allowLoopback: true}
	hook := Hook{ID: "h", URL: srv.URL, Secret: "topsecretkey"}
	err := d.DeliverEvent(context.Background(), hook, Event{
		Type:    "process.crashed",
		Payload: map[string]string{"id": "proc-1"},
	})
	if err != nil {
		t.Fatalf("DeliverEvent: %v", err)
	}
	if gotType != "process.crashed" {
		t.Errorf("event-type = %q", gotType)
	}
	if !strings.HasPrefix(gotDelivery, "dlv-") {
		t.Errorf("delivery-id = %q", gotDelivery)
	}
	mac := hmac.New(sha256.New, []byte("topsecretkey"))
	_, _ = mac.Write(gotBody)
	want := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if gotSig != want {
		t.Errorf("signature = %q, want %q", gotSig, want)
	}
}

func TestDeliverNoSecretNoSignature(t *testing.T) {
	t.Parallel()
	var hadSig bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hadSig = r.Header.Get("X-PMMCP-Signature") != ""
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	d := &Deliverer{allowLoopback: true}
	if err := d.Deliver(context.Background(), Hook{ID: "h", URL: srv.URL}, map[string]int{"n": 1}); err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	if hadSig {
		t.Fatal("unsigned delivery carried a signature header")
	}
}

func TestDeliverRedirectRevalidated(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Redirect(w, &http.Request{}, "http://169.254.169.254/latest/meta-data/", http.StatusFound)
	}))
	t.Cleanup(srv.Close)

	d := &Deliverer{allowLoopback: true}
	err := d.Deliver(context.Background(), Hook{ID: "h", URL: srv.URL}, map[string]int{"n": 1})
	if err == nil {
		t.Fatal("redirect to metadata should fail")
	}
	if !errors.Is(err, ErrSSRF) {
		t.Fatalf("redirect err = %v, want ErrSSRF", err)
	}
}

func TestDeliverPinsResolvedIP(t *testing.T) {
	t.Parallel()
	// Rebinding: validate sees a hostname (passes), but the dial resolves to
	// loopback, which the dial-time pin must reject.
	d := &Deliverer{
		lookupIP: func(_ context.Context, _ string) ([]net.IP, error) {
			return []net.IP{net.ParseIP("127.0.0.1")}, nil
		},
	}
	err := d.Deliver(context.Background(), Hook{ID: "h", URL: "http://rebind.example/hook"}, map[string]int{"n": 1})
	if !errors.Is(err, ErrSSRF) {
		t.Fatalf("rebinding err = %v, want ErrSSRF", err)
	}
}

func TestDeliverResolverFailsClosed(t *testing.T) {
	t.Parallel()
	d := &Deliverer{
		lookupIP: func(_ context.Context, _ string) ([]net.IP, error) {
			return nil, errors.New("SERVFAIL")
		},
	}
	err := d.Deliver(context.Background(), Hook{ID: "h", URL: "http://flaky.example/hook"}, map[string]int{"n": 1})
	if !errors.Is(err, ErrSSRF) {
		t.Fatalf("resolver-failure err = %v, want ErrSSRF (fail closed)", err)
	}
}

func TestDeliverAllowlistEnforced(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	// Deliverer allowlist that does not include the loopback host ⇒ denied even
	// with loopback dialing permitted.
	d := &Deliverer{allowLoopback: true, Allowlist: []string{"hooks.example.com"}}
	err := d.Deliver(context.Background(), Hook{ID: "h", URL: srv.URL}, map[string]int{"n": 1})
	if !errors.Is(err, ErrSSRF) {
		t.Fatalf("off-allowlist delivery = %v, want ErrSSRF", err)
	}
}

func TestAllowlisted(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		raw   string
		allow []string
		want  bool
	}{
		{"empty denies", "https://example.com/x", nil, false},
		{"exact host", "https://example.com/x", []string{"example.com"}, true},
		{"wildcard sub", "https://api.example.com/x", []string{"*.example.com"}, true},
		{"wildcard bare", "https://example.com/x", []string{"*.example.com"}, true},
		{"wildcard miss", "https://evil.com/x", []string{"*.example.com"}, false},
		{"url prefix", "https://example.com/hooks/pmmcp", []string{"https://example.com/hooks/"}, true},
		{"url prefix miss", "https://example.com/other", []string{"https://example.com/hooks/"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			u, err := parseHTTPURL(tt.raw)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if got := allowlisted(tt.raw, u, tt.allow); got != tt.want {
				t.Fatalf("allowlisted(%q, %v) = %v, want %v", tt.raw, tt.allow, got, tt.want)
			}
		})
	}
}

func TestHookMatches(t *testing.T) {
	t.Parallel()
	all := Hook{}
	if !all.Matches("anything") {
		t.Fatal("empty Events should match all")
	}
	filtered := Hook{Events: []string{"process.crashed", "process.exited"}}
	if !filtered.Matches("process.crashed") {
		t.Fatal("should match listed type")
	}
	if filtered.Matches("process.started") {
		t.Fatal("should not match unlisted type")
	}
}

func TestNewDeliveryID(t *testing.T) {
	t.Parallel()
	a, b := newDeliveryID(), newDeliveryID()
	if !strings.HasPrefix(a, "dlv-") || len(a) != len("dlv-")+26 {
		t.Fatalf("bad delivery id %q", a)
	}
	if a == b {
		t.Fatal("delivery ids should differ")
	}
}

// unmarshalable is a JSON-unmarshalable payload (channels never marshal).
type unmarshalable chan int

func TestDeliverEventMarshalError(t *testing.T) {
	t.Parallel()
	d := &Deliverer{allowLoopback: true}
	err := d.DeliverEvent(context.Background(), Hook{ID: "h", URL: "http://127.0.0.1:1/hook"}, Event{
		Payload: make(unmarshalable),
	})
	if err == nil || !strings.Contains(err.Error(), "webhook: marshal:") {
		t.Fatalf("err = %v, want marshal error", err)
	}
}

func TestDeliverEventNilContextRequestError(t *testing.T) {
	t.Parallel()
	d := &Deliverer{allowLoopback: true}
	//nolint:staticcheck // intentionally nil to exercise http.NewRequestWithContext's error branch
	err := d.DeliverEvent(nil, Hook{ID: "h", URL: "http://127.0.0.1:1/hook"}, Event{Payload: map[string]int{"n": 1}})
	if err == nil || !strings.Contains(err.Error(), "webhook: request:") {
		t.Fatalf("err = %v, want request error", err)
	}
}

func TestDeliverEventNon2xxStatus(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	d := &Deliverer{allowLoopback: true}
	err := d.Deliver(context.Background(), Hook{ID: "h", URL: srv.URL}, map[string]int{"n": 1})
	if err == nil || !strings.Contains(err.Error(), "status 500") {
		t.Fatalf("err = %v, want status 500 error", err)
	}
}

func TestValidateParseError(t *testing.T) {
	t.Parallel()
	d := &Deliverer{}
	if err := d.validate(""); !errors.Is(err, ErrInvalid) {
		t.Fatalf("validate(\"\") = %v, want ErrInvalid", err)
	}
}

func TestValidateBlockedHostname(t *testing.T) {
	t.Parallel()
	d := &Deliverer{}
	err := d.validate("http://localhost/hook")
	if !errors.Is(err, ErrSSRF) {
		t.Fatalf("validate(localhost) = %v, want ErrSSRF", err)
	}
}

func TestValidateAllowlistPositiveMatch(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	u, err := parseHTTPURL(srv.URL)
	if err != nil {
		t.Fatalf("parseHTTPURL: %v", err)
	}
	d := &Deliverer{allowLoopback: true, Allowlist: []string{u.Hostname()}}
	if err := d.Deliver(context.Background(), Hook{ID: "h", URL: srv.URL}, map[string]int{"n": 1}); err != nil {
		t.Fatalf("allowlisted delivery failed: %v", err)
	}
}

func TestHTTPClientCallerSupplied(t *testing.T) {
	t.Parallel()
	var gotRedirectCheck bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	d := &Deliverer{allowLoopback: true, Client: srv.Client()}
	if err := d.Deliver(context.Background(), Hook{ID: "h", URL: srv.URL}, map[string]int{"n": 1}); err != nil {
		t.Fatalf("Deliver with caller client: %v", err)
	}
	// Confirm the caller-supplied client's redirect policy was replaced with
	// our revalidating one, without mutating the original client.
	c := d.httpClient()
	gotRedirectCheck = c.CheckRedirect != nil
	if !gotRedirectCheck {
		t.Fatal("httpClient() did not install CheckRedirect on caller-supplied client")
	}
	if srv.Client().CheckRedirect != nil {
		t.Fatal("original caller-supplied client was mutated")
	}
}

func TestCheckRedirectTooManyRedirects(t *testing.T) {
	t.Parallel()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, srv.URL, http.StatusFound)
	}))
	t.Cleanup(srv.Close)

	d := &Deliverer{allowLoopback: true}
	err := d.Deliver(context.Background(), Hook{ID: "h", URL: srv.URL}, map[string]int{"n": 1})
	if err == nil || !strings.Contains(err.Error(), "too many redirects") {
		t.Fatalf("err = %v, want too-many-redirects error", err)
	}
	if errors.Is(err, ErrSSRF) {
		t.Fatalf("err = %v, should not be ErrSSRF", err)
	}
}

func TestDialContextSplitHostPortError(t *testing.T) {
	t.Parallel()
	d := &Deliverer{}
	_, err := d.dialContext(context.Background(), "tcp", "no-port-here")
	if err == nil || !strings.Contains(err.Error(), "webhook: dial addr:") {
		t.Fatalf("err = %v, want dial-addr error", err)
	}
}

func TestDialContextDialFailure(t *testing.T) {
	t.Parallel()
	// Reserve a loopback port, then close it immediately so the dial fails
	// fast with connection-refused instead of hanging on a timeout.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	addr := ln.Addr().String()
	if err := ln.Close(); err != nil {
		t.Fatalf("close listener: %v", err)
	}

	d := &Deliverer{
		allowLoopback: true,
		lookupIP: func(_ context.Context, _ string) ([]net.IP, error) {
			return []net.IP{net.ParseIP("127.0.0.1")}, nil
		},
	}
	_, dialErr := d.dialContext(context.Background(), "tcp", addr)
	if dialErr == nil {
		t.Fatal("dial to closed port should fail")
	}
	if errors.Is(dialErr, ErrSSRF) {
		t.Fatalf("dial failure should not be wrapped as ErrSSRF: %v", dialErr)
	}
}

func TestDialContextNoCandidateIPs(t *testing.T) {
	t.Parallel()
	d := &Deliverer{
		lookupIP: func(_ context.Context, _ string) ([]net.IP, error) {
			return nil, nil
		},
	}
	_, err := d.dialContext(context.Background(), "tcp", "example.invalid:80")
	if !errors.Is(err, ErrSSRF) || !strings.Contains(err.Error(), "no permitted address") {
		t.Fatalf("err = %v, want ErrSSRF no-permitted-address", err)
	}
}

func TestResolveLiteralIPShortCircuit(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	// No lookupIP is set: if resolve() didn't short-circuit on the literal-IP
	// host, this would panic dereferencing a nil func.
	d := &Deliverer{allowLoopback: true}
	if err := d.Deliver(context.Background(), Hook{ID: "h", URL: srv.URL}, map[string]int{"n": 1}); err != nil {
		t.Fatalf("Deliver to literal-IP host: %v", err)
	}
}

func TestResolveRealLookupSuccess(t *testing.T) {
	t.Parallel()
	d := &Deliverer{}
	ips, err := d.resolve(context.Background(), "localhost")
	if err != nil {
		t.Fatalf("resolve(localhost): %v", err)
	}
	if len(ips) == 0 {
		t.Fatal("resolve(localhost) returned no addresses")
	}
}

func TestResolveRealLookupFailure(t *testing.T) {
	t.Parallel()
	d := &Deliverer{}
	// RFC 2606 reserved TLD; resolved (or NXDOMAIN'd) locally via NSS without
	// depending on live network reachability.
	_, err := d.resolve(context.Background(), "definitely-does-not-exist-pmmcp-test.invalid")
	if err == nil {
		t.Skip("resolver unexpectedly resolved a .invalid host in this environment")
	}
}

func TestBlockedAllowLoopbackDoesNotCoverLinkLocal(t *testing.T) {
	t.Parallel()
	linkLocal := net.ParseIP("169.254.1.1")
	dAllow := &Deliverer{allowLoopback: true}
	if !dAllow.blocked(linkLocal) {
		t.Fatal("allowLoopback must not carve out link-local addresses")
	}
	dDeny := &Deliverer{}
	if !dDeny.blocked(linkLocal) {
		t.Fatal("link-local address should be blocked")
	}
}

// failingRandReader always errors; used to force ulid.New's entropy read to
// fail so newDeliveryID falls back to its all-zero ID.
type failingRandReader struct{}

func (failingRandReader) Read([]byte) (int, error) {
	return 0, fmt.Errorf("injected rand failure")
}

// gocoverdirFlag extracts this process's "-test.gocoverdir=<dir>" argument, if
// any (present when the enclosing `go test` run was given -cover/-coverprofile).
func gocoverdirFlag() string {
	const prefix = "-test.gocoverdir="
	for _, a := range os.Args {
		if strings.HasPrefix(a, prefix) {
			return strings.TrimPrefix(a, prefix)
		}
	}
	return ""
}

// TestNewDeliveryIDFallback exercises newDeliveryID's ulid.New failure branch.
// Forcing that failure means swapping the package-level crypto/rand.Reader,
// which is global mutable state that would race with every other (parallel)
// test in this package that generates a delivery ID. To keep this hermetic
// and -race-safe, the swap happens only inside a re-executed subprocess of
// this same test binary (the standard library's own os/exec tests use this
// pattern) — the parent process's rand.Reader is never touched.
//
// The subprocess is pointed at the same -test.gocoverdir the outer `go test
// -coverprofile=...` run is using (when present), so its counter data merges
// into the final coverage profile instead of being silently discarded.
func TestNewDeliveryIDFallback(t *testing.T) {
	t.Parallel()
	if os.Getenv("PMMCP_WEBHOOK_TEST_RAND_FAIL") == "1" {
		rand.Reader = failingRandReader{}
		want := "dlv-" + strings.Repeat("0", 26)
		if got := newDeliveryID(); got != want {
			t.Fatalf("newDeliveryID() = %q, want %q", got, want)
		}
		return
	}

	args := []string{"-test.run=^TestNewDeliveryIDFallback$", "-test.v"}
	if dir := gocoverdirFlag(); dir != "" {
		args = append(args, "-test.gocoverdir="+dir)
	}
	cmd := exec.Command(os.Args[0], args...)
	cmd.Env = append(os.Environ(), "PMMCP_WEBHOOK_TEST_RAND_FAIL=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("subprocess failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "--- PASS: TestNewDeliveryIDFallback") {
		t.Fatalf("subprocess did not report a pass:\n%s", out)
	}
}
