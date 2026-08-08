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

package doctor_test

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/scrothers/pmmcp/internal/api"
	pmmcpv1 "github.com/scrothers/pmmcp/internal/api/gen/pmmcp/v1"
	"github.com/scrothers/pmmcp/internal/doctor"
	"google.golang.org/grpc"
)

// fakeDaemon is a minimal gRPC Daemon that answers Hello.
type fakeDaemon struct {
	pmmcpv1.UnimplementedDaemonServer
	apiVersion    string
	daemonVersion string
}

func (f *fakeDaemon) Call(_ context.Context, req *pmmcpv1.CallRequest) (*pmmcpv1.CallResponse, error) {
	if req.GetMethod() == api.MethodHello {
		b, _ := json.Marshal(api.HelloResult{
			APIVersion:    f.apiVersion,
			DaemonVersion: f.daemonVersion,
			UID:           "1000",
		})
		return &pmmcpv1.CallResponse{Ok: true, Payload: b}, nil
	}
	return &pmmcpv1.CallResponse{Ok: false, ErrorCode: "unimplemented", Error: "unimplemented"}, nil
}

// helloOnceDaemon answers Hello successfully exactly once, then rejects
// every subsequent Hello. ipc.Dial performs its own internal Hello handshake
// before returning a client, so this lets checkUnix's *second*, explicit
// Hello call (post-dial) observe a failure independent of the dial outcome.
type helloOnceDaemon struct {
	pmmcpv1.UnimplementedDaemonServer
	calls atomic.Int32
}

func (d *helloOnceDaemon) Call(_ context.Context, req *pmmcpv1.CallRequest) (*pmmcpv1.CallResponse, error) {
	if req.GetMethod() != api.MethodHello {
		return &pmmcpv1.CallResponse{Ok: false, ErrorCode: "unimplemented", Error: "unimplemented"}, nil
	}
	if d.calls.Add(1) == 1 {
		b, _ := json.Marshal(api.HelloResult{APIVersion: api.APIVersion, DaemonVersion: "1.0.0"})
		return &pmmcpv1.CallResponse{Ok: true, Payload: b}, nil
	}
	return &pmmcpv1.CallResponse{Ok: false, ErrorCode: "boom", Error: "second hello rejected"}, nil
}

// helloRejectDaemon refuses the Hello handshake outright (Ok: false).
type helloRejectDaemon struct {
	pmmcpv1.UnimplementedDaemonServer
}

func (helloRejectDaemon) Call(_ context.Context, _ *pmmcpv1.CallRequest) (*pmmcpv1.CallResponse, error) {
	return &pmmcpv1.CallResponse{Ok: false, ErrorCode: "denied", Error: "hello denied"}, nil
}

// helloGarbagePayloadDaemon accepts the Hello call but returns a payload that
// isn't valid JSON, exercising the decode-failure path.
type helloGarbagePayloadDaemon struct {
	pmmcpv1.UnimplementedDaemonServer
}

func (helloGarbagePayloadDaemon) Call(_ context.Context, _ *pmmcpv1.CallRequest) (*pmmcpv1.CallResponse, error) {
	return &pmmcpv1.CallResponse{Ok: true, Payload: []byte("{not json")}, nil
}

func serve(t *testing.T, ln net.Listener, d pmmcpv1.DaemonServer) {
	t.Helper()
	s := grpc.NewServer()
	pmmcpv1.RegisterDaemonServer(s, d)
	go func() { _ = s.Serve(ln) }()
	t.Cleanup(s.Stop)
}

func TestCheckMissingSocket(t *testing.T) {
	t.Parallel()
	sock := filepath.Join(t.TempDir(), "missing.sock")
	r := doctor.Check(context.Background(), sock)
	if r.OK {
		t.Fatal("OK = true, want false for missing socket")
	}
	text := strings.Join(r.Lines, "\n")
	if !strings.Contains(text, "pmmcpd") {
		t.Fatalf("want pmmcpd in remediation; lines:\n%s", text)
	}
	if !strings.Contains(text, "install-service") {
		t.Fatalf("want install-service in remediation; lines:\n%s", text)
	}
}

func TestCheckEmptyEndpoint(t *testing.T) {
	t.Parallel()
	r := doctor.Check(context.Background(), "")
	if r.OK {
		t.Fatal("OK = true for empty endpoint")
	}
	if !strings.Contains(strings.Join(r.Lines, "\n"), "pmmcpd") {
		t.Fatalf("lines: %v", r.Lines)
	}
}

func TestCheckUnixHelloOK(t *testing.T) {
	t.Parallel()
	sock := filepath.Join(t.TempDir(), "d.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen unix: %v", err)
	}
	serve(t, ln, &fakeDaemon{apiVersion: api.APIVersion, daemonVersion: "9.9.9"})

	r := doctor.Check(context.Background(), sock)
	if !r.OK {
		t.Fatalf("OK = false, want true; lines:\n%s", strings.Join(r.Lines, "\n"))
	}
	if !strings.Contains(strings.Join(r.Lines, "\n"), "9.9.9") {
		t.Fatalf("want daemon version in output; lines:\n%s", strings.Join(r.Lines, "\n"))
	}
}

func TestCheckTCPHelloOK(t *testing.T) {
	t.Parallel()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen tcp: %v", err)
	}
	serve(t, ln, &fakeDaemon{apiVersion: api.APIVersion, daemonVersion: "1.2.3"})

	r := doctor.Check(context.Background(), ln.Addr().String())
	if !r.OK {
		t.Fatalf("OK = false, want true; lines:\n%s", strings.Join(r.Lines, "\n"))
	}
	if !strings.Contains(strings.Join(r.Lines, "\n"), "tcp") {
		t.Fatalf("want tcp marker; lines:\n%s", strings.Join(r.Lines, "\n"))
	}
}

func TestCheckTCPSquatterNotOK(t *testing.T) {
	t.Parallel()
	// A listener that accepts and immediately closes speaks no gRPC — doctor
	// must not report a healthy daemon (the pre-fix behavior).
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen tcp: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()

	r := doctor.Check(context.Background(), ln.Addr().String())
	if r.OK {
		t.Fatalf("squatting listener reported OK; lines:\n%s", strings.Join(r.Lines, "\n"))
	}
}

func TestCheckTCPVersionSkewNotOK(t *testing.T) {
	t.Parallel()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen tcp: %v", err)
	}
	serve(t, ln, &fakeDaemon{apiVersion: "2.0", daemonVersion: "9.9.9"})

	r := doctor.Check(context.Background(), ln.Addr().String())
	if r.OK {
		t.Fatalf("version-skewed daemon reported OK; lines:\n%s", strings.Join(r.Lines, "\n"))
	}
	if !strings.Contains(strings.Join(r.Lines, "\n"), "mismatch") {
		t.Fatalf("want version mismatch note; lines:\n%s", strings.Join(r.Lines, "\n"))
	}
}

func TestCheckUnixStaleSocketFile(t *testing.T) {
	t.Parallel()
	// A stale socket path: the file exists (e.g. left behind by a crashed
	// daemon) but nothing is listening, so dialing it must fail rather than
	// hang or falsely report OK — distinct from the ENOENT (missing) case.
	sock := filepath.Join(t.TempDir(), "stale.sock")
	if err := os.WriteFile(sock, []byte("not a socket"), 0o600); err != nil {
		t.Fatalf("write stale file: %v", err)
	}

	r := doctor.Check(context.Background(), sock)
	if r.OK {
		t.Fatal("OK = true, want false for a stale non-socket file")
	}
	text := strings.Join(r.Lines, "\n")
	if !strings.Contains(text, "dial:") {
		t.Fatalf("want dial error note; lines:\n%s", text)
	}
	if !strings.Contains(text, "pmmcpd") {
		t.Fatalf("want remediation; lines:\n%s", text)
	}
}

func TestCheckUnixSecondHelloFails(t *testing.T) {
	t.Parallel()
	// ipc.Dial performs its own Hello handshake before handing back a
	// client; checkUnix then issues a second, explicit Hello. Make the
	// second call fail to exercise that post-dial error branch.
	sock := filepath.Join(t.TempDir(), "d.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen unix: %v", err)
	}
	serve(t, ln, &helloOnceDaemon{})

	r := doctor.Check(context.Background(), sock)
	if r.OK {
		t.Fatalf("OK = true, want false when the post-dial hello fails; lines:\n%s", strings.Join(r.Lines, "\n"))
	}
	text := strings.Join(r.Lines, "\n")
	if !strings.Contains(text, "hello:") {
		t.Fatalf("want hello error note; lines:\n%s", text)
	}
	if !strings.Contains(text, "pmmcpd") {
		t.Fatalf("want remediation; lines:\n%s", text)
	}
}

func TestCheckTCPHelloRejected(t *testing.T) {
	t.Parallel()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen tcp: %v", err)
	}
	serve(t, ln, helloRejectDaemon{})

	r := doctor.Check(context.Background(), ln.Addr().String())
	if r.OK {
		t.Fatalf("OK = true, want false when daemon refuses hello; lines:\n%s", strings.Join(r.Lines, "\n"))
	}
	if !strings.Contains(strings.Join(r.Lines, "\n"), "hello denied") {
		t.Fatalf("want hello-refused detail; lines:\n%s", strings.Join(r.Lines, "\n"))
	}
}

func TestCheckTCPHelloBadPayload(t *testing.T) {
	t.Parallel()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen tcp: %v", err)
	}
	serve(t, ln, helloGarbagePayloadDaemon{})

	r := doctor.Check(context.Background(), ln.Addr().String())
	if r.OK {
		t.Fatalf("OK = true, want false when hello payload isn't valid JSON; lines:\n%s", strings.Join(r.Lines, "\n"))
	}
	if !strings.Contains(strings.Join(r.Lines, "\n"), "decode hello") {
		t.Fatalf("want decode error detail; lines:\n%s", strings.Join(r.Lines, "\n"))
	}
}

func TestCheckTCPEmptyAPIVersionNoSkew(t *testing.T) {
	t.Parallel()
	// An empty daemon api_version is treated as "no skew reported" by
	// versionSkew, distinct from the matching- and differing-version cases.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen tcp: %v", err)
	}
	serve(t, ln, &fakeDaemon{apiVersion: "", daemonVersion: "0.0.0"})

	r := doctor.Check(context.Background(), ln.Addr().String())
	if !r.OK {
		t.Fatalf("OK = false, want true when daemon reports no api version; lines:\n%s", strings.Join(r.Lines, "\n"))
	}
}

func TestCheckMalformedTCPEndpoint(t *testing.T) {
	t.Parallel()
	// host:port-shaped but not a valid gRPC target string: grpc.NewClient
	// itself fails before any dial is attempted.
	r := doctor.Check(context.Background(), "%zz:80")
	if r.OK {
		t.Fatal("OK = true for a malformed tcp endpoint")
	}
	if !strings.Contains(strings.Join(r.Lines, "\n"), "hello (tcp):") {
		t.Fatalf("want tcp hello error note; lines:\n%s", strings.Join(r.Lines, "\n"))
	}
}

func TestCheckEndpointWithoutColonTreatedAsUnixPath(t *testing.T) {
	t.Parallel()
	// No "/" and no ":" — isTCPEndpoint's net.SplitHostPort call fails, so
	// this falls through to the unix path (and then fails to dial, since no
	// such socket exists).
	r := doctor.Check(context.Background(), "not-a-host-port-or-a-path")
	if r.OK {
		t.Fatal("OK = true for a nonexistent bare-word endpoint")
	}
	if !strings.Contains(strings.Join(r.Lines, "\n"), "dial:") {
		t.Fatalf("want unix dial error note; lines:\n%s", strings.Join(r.Lines, "\n"))
	}
}

// TestRemediationForEveryOS exercises every OS-specific hint in the
// remediation seam, regardless of which OS runs this test binary.
func TestRemediationForEveryOS(t *testing.T) {
	t.Parallel()
	tests := []struct {
		goos string
		want string
	}{
		{"darwin", "launchctl load"},
		{"windows", "register the logon task"},
		{"linux", "systemctl --user enable --now"},
		{"plan9", "systemctl --user enable --now"}, // default arm covers linux and anything else
	}
	for _, tt := range tests {
		t.Run(tt.goos, func(t *testing.T) {
			t.Parallel()
			lines := doctor.RemediationFor(tt.goos)
			joined := strings.Join(lines, "\n")
			if !strings.Contains(joined, "pmmcpd run") {
				t.Fatalf("RemediationFor(%s) missing base hint; lines:\n%s", tt.goos, joined)
			}
			if !strings.Contains(joined, tt.want) {
				t.Fatalf("RemediationFor(%s) = %v, want it to contain %q", tt.goos, lines, tt.want)
			}
		})
	}
}
