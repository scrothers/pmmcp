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

package ipc_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/scrothers/pmmcp/internal/api"
	pmmcpv1 "github.com/scrothers/pmmcp/internal/api/gen/pmmcp/v1"
	"github.com/scrothers/pmmcp/internal/domain"
	"github.com/scrothers/pmmcp/internal/ipc"
	"github.com/scrothers/pmmcp/internal/testsock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// fakeDaemonServer is a minimal, hand-written pmmcpv1.DaemonServer used to
// exercise the ipc.Client transport without booting the real daemon.
type fakeDaemonServer struct {
	pmmcpv1.UnimplementedDaemonServer

	mu           sync.Mutex
	lastReq      *pmmcpv1.CallRequest
	helloVersion string // overrides the reported daemon api_version; "" = api.APIVersion
	callFunc     func(*pmmcpv1.CallRequest) (*pmmcpv1.CallResponse, error)
	subFunc      func(*pmmcpv1.SubscribeLogsRequest, pmmcpv1.Daemon_SubscribeLogsServer) error
}

func (f *fakeDaemonServer) Call(_ context.Context, req *pmmcpv1.CallRequest) (*pmmcpv1.CallResponse, error) {
	f.mu.Lock()
	f.lastReq = req
	f.mu.Unlock()
	if req.GetMethod() == api.MethodHello {
		v := f.helloVersion
		if v == "" {
			v = api.APIVersion
		}
		payload, err := json.Marshal(api.HelloResult{APIVersion: v})
		if err != nil {
			return nil, err
		}
		return &pmmcpv1.CallResponse{Ok: true, Payload: payload}, nil
	}
	if f.callFunc != nil {
		return f.callFunc(req)
	}
	return &pmmcpv1.CallResponse{Ok: true}, nil
}

func (f *fakeDaemonServer) SubscribeLogs(req *pmmcpv1.SubscribeLogsRequest, stream pmmcpv1.Daemon_SubscribeLogsServer) error {
	if f.subFunc != nil {
		return f.subFunc(req, stream)
	}
	return stream.Send(&pmmcpv1.LogChunk{ProcessId: req.GetProcessId(), Text: "hi", Eof: true})
}

func (f *fakeDaemonServer) lastRequest() *pmmcpv1.CallRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lastReq
}

// startFakeDaemon boots a real gRPC server over a UDS backed by ipc.Listen
// and returns its endpoint plus a stop func.
func startFakeDaemon(t *testing.T, fake *fakeDaemonServer) string {
	t.Helper()
	endpoint := testsock.Path(t)
	ln, err := ipc.Listen(endpoint)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	gs := grpc.NewServer()
	pmmcpv1.RegisterDaemonServer(gs, fake)
	go func() { _ = gs.Serve(ln) }()
	t.Cleanup(gs.Stop)
	return endpoint
}

func TestDialSuccessAndClose(t *testing.T) {
	t.Parallel()
	endpoint := startFakeDaemon(t, &fakeDaemonServer{})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, err := ipc.Dial(ctx, endpoint)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	if c.DaemonRPC() == nil {
		t.Fatal("DaemonRPC returned nil")
	}
	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestCloseNilClient(t *testing.T) {
	t.Parallel()
	var c *ipc.Client
	if err := c.Close(); err != nil {
		t.Fatalf("Close on nil client: %v", err)
	}
}

func TestDialVersionMajorMismatchFailsClosed(t *testing.T) {
	t.Parallel()
	endpoint := startFakeDaemon(t, &fakeDaemonServer{helloVersion: "99.0"})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := ipc.Dial(ctx, endpoint); err == nil {
		t.Fatal("Dial with major version mismatch: want error, got nil")
	}
}

func TestDialVersionEmptyFailsClosed(t *testing.T) {
	t.Parallel()
	endpoint := startFakeDaemon(t, &fakeDaemonServer{helloVersion: " "})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := ipc.Dial(ctx, endpoint); err == nil {
		t.Fatal("Dial with empty daemon version: want error, got nil")
	}
}

func TestDialUnreachableEndpoint(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := ipc.Dial(ctx, testsock.Path(t)); err == nil {
		t.Fatal("Dial to nonexistent socket: want error, got nil")
	}
}

func TestDialInvalidTargetFailsAtNewClient(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	// A control character in the endpoint makes "unix://"+endpoint an
	// unparsable gRPC target, failing inside grpc.NewClient itself rather
	// than at the handshake.
	if _, err := ipc.Dial(ctx, "\x00bad"); err == nil {
		t.Fatal("Dial with invalid target: want error, got nil")
	}
}

func TestClientCallSuccessRoundTrip(t *testing.T) {
	t.Parallel()
	fake := &fakeDaemonServer{
		callFunc: func(req *pmmcpv1.CallRequest) (*pmmcpv1.CallResponse, error) {
			var in struct{ X int }
			if err := json.Unmarshal(req.GetPayload(), &in); err != nil {
				return nil, err
			}
			payload, _ := json.Marshal(struct{ Y int }{Y: in.X + 1})
			return &pmmcpv1.CallResponse{Ok: true, Payload: payload}, nil
		},
	}
	endpoint := startFakeDaemon(t, fake)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, err := ipc.Dial(ctx, endpoint)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer c.Close()

	c.SetSession("sess-1", "operator")
	var out struct{ Y int }
	if err := c.Call(ctx, "some.method", struct{ X int }{X: 41}, &out); err != nil {
		t.Fatalf("Call: %v", err)
	}
	if out.Y != 42 {
		t.Fatalf("Y = %d, want 42", out.Y)
	}
	last := fake.lastRequest()
	if last.GetSession() != "sess-1" || last.GetRole() != "operator" {
		t.Fatalf("session/role not passed through: %+v", last)
	}

	// SetSession with an empty role keeps the previously set role.
	c.SetSession("sess-2", "")
	if err := c.Call(ctx, "some.method", struct{ X int }{X: 1}, &out); err != nil {
		t.Fatalf("Call: %v", err)
	}
	last = fake.lastRequest()
	if last.GetSession() != "sess-2" || last.GetRole() != "operator" {
		t.Fatalf("empty-role SetSession overwrote role: %+v", last)
	}
}

func TestClientCallNilInAndOut(t *testing.T) {
	t.Parallel()
	endpoint := startFakeDaemon(t, &fakeDaemonServer{})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, err := ipc.Dial(ctx, endpoint)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer c.Close()
	if err := c.Call(ctx, "noop", nil, nil); err != nil {
		t.Fatalf("Call with nil in/out: %v", err)
	}
}

func TestClientCallAddsDefaultDeadline(t *testing.T) {
	t.Parallel()
	endpoint := startFakeDaemon(t, &fakeDaemonServer{})
	dialCtx, dialCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer dialCancel()
	c, err := ipc.Dial(dialCtx, endpoint)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer c.Close()
	// No deadline on this context: Call must apply its own 60s default
	// rather than blocking forever.
	if err := c.Call(context.Background(), "noop", nil, nil); err != nil {
		t.Fatalf("Call without caller deadline: %v", err)
	}
}

func TestClientCallPreservesExistingDeadline(t *testing.T) {
	t.Parallel()
	endpoint := startFakeDaemon(t, &fakeDaemonServer{})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, err := ipc.Dial(ctx, endpoint)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer c.Close()
	callCtx, callCancel := context.WithTimeout(ctx, time.Second)
	defer callCancel()
	if err := c.Call(callCtx, "noop", nil, nil); err != nil {
		t.Fatalf("Call: %v", err)
	}
}

func TestClientCallPayloadMarshalError(t *testing.T) {
	t.Parallel()
	endpoint := startFakeDaemon(t, &fakeDaemonServer{})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, err := ipc.Dial(ctx, endpoint)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer c.Close()
	if err := c.Call(ctx, "noop", make(chan int), nil); err == nil {
		t.Fatal("Call with unmarshalable payload: want error, got nil")
	}
}

func TestClientCallDecodeResultError(t *testing.T) {
	t.Parallel()
	fake := &fakeDaemonServer{
		callFunc: func(*pmmcpv1.CallRequest) (*pmmcpv1.CallResponse, error) {
			return &pmmcpv1.CallResponse{Ok: true, Payload: []byte("not json")}, nil
		},
	}
	endpoint := startFakeDaemon(t, fake)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, err := ipc.Dial(ctx, endpoint)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer c.Close()
	var out struct{ Y int }
	if err := c.Call(ctx, "noop", nil, &out); err == nil {
		t.Fatal("Call with undecodable payload: want error, got nil")
	}
}

func TestClientCallRPCErrorMapsToDaemonUnavailable(t *testing.T) {
	t.Parallel()
	fake := &fakeDaemonServer{
		callFunc: func(*pmmcpv1.CallRequest) (*pmmcpv1.CallResponse, error) {
			return nil, status.Error(codes.Unavailable, "down")
		},
	}
	endpoint := startFakeDaemon(t, fake)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, err := ipc.Dial(ctx, endpoint)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer c.Close()
	err = c.Call(ctx, "noop", nil, nil)
	if err == nil {
		t.Fatal("Call with rpc error: want error, got nil")
	}
	var derr *domain.Error
	if !errors.As(err, &derr) {
		t.Fatalf("Call error is not a domain.Error: %v", err)
	}
	if derr.Code != domain.CodeDaemonUnavailable || !derr.Retryable {
		t.Fatalf("Call error = %+v, want daemon_unavailable retryable", derr)
	}
}

func TestClientCallErrorResponseExplicitCode(t *testing.T) {
	t.Parallel()
	fake := &fakeDaemonServer{
		callFunc: func(*pmmcpv1.CallRequest) (*pmmcpv1.CallResponse, error) {
			return &pmmcpv1.CallResponse{Ok: false, ErrorCode: string(domain.CodeInvalidArgument), Error: "bad", Retryable: false}, nil
		},
	}
	endpoint := startFakeDaemon(t, fake)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, err := ipc.Dial(ctx, endpoint)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer c.Close()
	err = c.Call(ctx, "noop", nil, nil)
	var derr *domain.Error
	if !errors.As(err, &derr) {
		t.Fatalf("Call error is not a domain.Error: %v", err)
	}
	if derr.Code != domain.CodeInvalidArgument {
		t.Fatalf("Code = %q, want invalid_argument", derr.Code)
	}
}

func TestClientCallErrorResponseEmptyCodeDefaultsToInternal(t *testing.T) {
	t.Parallel()
	fake := &fakeDaemonServer{
		callFunc: func(*pmmcpv1.CallRequest) (*pmmcpv1.CallResponse, error) {
			return &pmmcpv1.CallResponse{Ok: false, Error: "boom"}, nil
		},
	}
	endpoint := startFakeDaemon(t, fake)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, err := ipc.Dial(ctx, endpoint)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer c.Close()
	err = c.Call(ctx, "noop", nil, nil)
	var derr *domain.Error
	if !errors.As(err, &derr) {
		t.Fatalf("Call error is not a domain.Error: %v", err)
	}
	if derr.Code != domain.CodeInternal {
		t.Fatalf("Code = %q, want internal", derr.Code)
	}
}

func TestClientSubscribeLogsRolePassthrough(t *testing.T) {
	t.Parallel()
	var gotRole string
	fake := &fakeDaemonServer{
		subFunc: func(req *pmmcpv1.SubscribeLogsRequest, stream pmmcpv1.Daemon_SubscribeLogsServer) error {
			gotRole = req.GetRole()
			return stream.Send(&pmmcpv1.LogChunk{ProcessId: req.GetProcessId(), Text: "line", Eof: true})
		},
	}
	endpoint := startFakeDaemon(t, fake)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, err := ipc.Dial(ctx, endpoint)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer c.Close()
	c.SetSession("sess-x", "readonly")

	stream, err := c.SubscribeLogs(ctx, "proc-1", "both", 5)
	if err != nil {
		t.Fatalf("SubscribeLogs: %v", err)
	}
	chunk, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv: %v", err)
	}
	if chunk.GetText() != "line" {
		t.Fatalf("Text = %q, want %q", chunk.GetText(), "line")
	}
	if gotRole != "readonly" {
		t.Fatalf("role passthrough = %q, want readonly", gotRole)
	}
}
