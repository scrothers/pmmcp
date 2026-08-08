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

package ipc

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"time"

	"github.com/scrothers/pmmcp/internal/api"
	pmmcpv1 "github.com/scrothers/pmmcp/internal/api/gen/pmmcp/v1"
	"github.com/scrothers/pmmcp/internal/domain"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Client is a private gRPC IPC client to pmmcpd.
type Client struct {
	conn    *grpc.ClientConn
	rpc     pmmcpv1.DaemonClient
	session string
	role    string
}

// Dial connects to the daemon private endpoint via gRPC (UDS or named pipe).
func Dial(ctx context.Context, endpoint string) (*Client, error) {
	target := grpcTarget(endpoint)
	conn, err := grpc.NewClient(target,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return dialTransport(ctx, endpoint)
		}),
	)
	if err != nil {
		return nil, domain.WrapError(domain.CodeDaemonUnavailable, "dial daemon", true, err)
	}
	// Perform the version handshake on connect and fail closed on skew:
	// the daemon's reported api_version must be compatible with ours.
	rpc := pmmcpv1.NewDaemonClient(conn)
	c := &Client{conn: conn, rpc: rpc, role: "full"}
	cctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if _, err := c.Hello(cctx); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return c, nil
}

// Close closes the connection.
func (c *Client) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

// SetSession sets session attribution for subsequent calls.
func (c *Client) SetSession(session, role string) {
	c.session = session
	if role != "" {
		c.role = role
	}
}

// Call performs a unary RPC round trip.
func (c *Client) Call(ctx context.Context, method string, in any, out any) error {
	var payload []byte
	var err error
	if in != nil {
		payload, err = json.Marshal(in)
		if err != nil {
			return fmt.Errorf("ipc: payload: %w", err)
		}
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 60*time.Second)
		defer cancel()
	}
	resp, err := c.rpc.Call(ctx, &pmmcpv1.CallRequest{
		ApiVersion: api.APIVersion,
		Method:     method,
		Session:    c.session,
		Role:       c.role,
		Payload:    payload,
	})
	if err != nil {
		return domain.WrapError(domain.CodeDaemonUnavailable, "rpc", true, err)
	}
	if !resp.GetOk() {
		code := domain.Code(resp.GetErrorCode())
		if code == "" {
			code = domain.CodeInternal
		}
		return domain.NewError(code, resp.GetError(), resp.GetRetryable())
	}
	if out != nil && len(resp.GetPayload()) > 0 {
		if err := json.Unmarshal(resp.GetPayload(), out); err != nil {
			return fmt.Errorf("ipc: decode result: %w", err)
		}
	}
	return nil
}

// Hello performs version handshake.
func (c *Client) Hello(ctx context.Context) (*api.HelloResult, error) {
	var out api.HelloResult
	if err := c.Call(ctx, api.MethodHello, nil, &out); err != nil {
		return nil, err
	}
	//: fail closed on major mismatch or client-minor-newer, using a
	// numeric compare (never a byte slice). An empty daemon version is a
	// mismatch, not a pass.
	if err := api.Compatible(api.APIVersion, out.APIVersion); err != nil {
		return nil, err
	}
	return &out, nil
}

// SubscribeLogs opens a server stream of log chunks.
func (c *Client) SubscribeLogs(ctx context.Context, processID, stream string, maxSec int32) (pmmcpv1.Daemon_SubscribeLogsClient, error) {
	return c.rpc.SubscribeLogs(ctx, &pmmcpv1.SubscribeLogsRequest{
		ApiVersion:     api.APIVersion,
		Session:        c.session,
		Role:           c.role,
		ProcessId:      processID,
		Stream:         stream,
		MaxDurationSec: maxSec,
	})
}

// DaemonRPC exposes the raw gRPC client for advanced callers.
func (c *Client) DaemonRPC() pmmcpv1.DaemonClient { return c.rpc }
