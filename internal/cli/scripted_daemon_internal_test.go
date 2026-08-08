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

package cli

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/scrothers/pmmcp/internal/api"
	pmmcpv1 "github.com/scrothers/pmmcp/internal/api/gen/pmmcp/v1"
	"google.golang.org/grpc"
)

// scriptedResponse describes how a scriptedDaemon answers one IPC method.
type scriptedResponse struct {
	payload   []byte
	errMsg    string
	errCode   string
	retryable bool
}

// scriptedDaemon answers api.MethodHello with a compatible handshake and every
// other method via a caller-supplied table, so tests can exercise both the
// success and daemon-error branches of CLI dispatch code without a real
// pmmcpd. Methods absent from the table get an empty `{}` success payload.
type scriptedDaemon struct {
	pmmcpv1.UnimplementedDaemonServer
	responses map[string]scriptedResponse
}

func (d scriptedDaemon) Call(_ context.Context, req *pmmcpv1.CallRequest) (*pmmcpv1.CallResponse, error) {
	if req.GetMethod() == api.MethodHello {
		b, _ := json.Marshal(api.HelloResult{APIVersion: api.APIVersion, DaemonVersion: "9.9.9"})
		return &pmmcpv1.CallResponse{Ok: true, Payload: b}, nil
	}
	resp, ok := d.responses[req.GetMethod()]
	if !ok {
		return &pmmcpv1.CallResponse{Ok: true, Payload: []byte(`{}`)}, nil
	}
	if resp.errMsg != "" {
		return &pmmcpv1.CallResponse{Ok: false, Error: resp.errMsg, ErrorCode: resp.errCode, Retryable: resp.retryable}, nil
	}
	return &pmmcpv1.CallResponse{Ok: true, Payload: resp.payload}, nil
}

// startScriptedDaemon spins up a real in-process gRPC daemon over a UDS
// answering per the given method table (see scriptedDaemon) and returns its
// endpoint.
func startScriptedDaemon(t *testing.T, responses map[string]scriptedResponse) string {
	t.Helper()
	sock := filepath.Join(t.TempDir(), "d.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	s := grpc.NewServer()
	pmmcpv1.RegisterDaemonServer(s, scriptedDaemon{responses: responses})
	go func() { _ = s.Serve(ln) }()
	t.Cleanup(s.Stop)
	return sock
}

// withClosedStdin replaces os.Stdin with the read end of a pipe whose write
// end is closed and then itself closed before fn runs, so any read against it
// fails immediately with "file already closed" — used to exercise
// io.ReadAll(os.Stdin) error branches deterministically. Mutates the
// process-global os.Stdin, so callers must not run in parallel.
func withClosedStdin(t *testing.T, fn func()) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	orig := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = orig })
	fn()
}
