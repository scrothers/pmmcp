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

package daemon

import (
	"context"
	"path/filepath"
	"time"

	"github.com/scrothers/pmmcp/internal/api"
	pmmcpv1 "github.com/scrothers/pmmcp/internal/api/gen/pmmcp/v1"
	"github.com/scrothers/pmmcp/internal/authz"
	"github.com/scrothers/pmmcp/internal/logcap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// grpcAdapter exposes Server over gRPC.
type grpcAdapter struct {
	pmmcpv1.UnimplementedDaemonServer
	s *Server
}

// Call maps unary JSON method dispatch onto existing handle.
func (g *grpcAdapter) Call(ctx context.Context, req *pmmcpv1.CallRequest) (*pmmcpv1.CallResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "nil request")
	}
	ar := api.Request{
		APIVersion: req.GetApiVersion(),
		Method:     req.GetMethod(),
		Session:    req.GetSession(),
		Role:       req.GetRole(),
		Payload:    req.GetPayload(),
	}
	// Do not backfill an empty api_version — an absent version must fail the
	// handshake in handle, not silently pass.
	resp := g.s.handle(ctx, ar)
	return &pmmcpv1.CallResponse{
		Ok:        resp.OK,
		ErrorCode: resp.ErrorCode,
		Error:     resp.Error,
		Retryable: resp.Retryable,
		Payload:   resp.Payload,
	}, nil
}

// SubscribeLogs streams log chunks until max duration or client cancel.
// The stream is gated: version handshake, logs:read capability, and outgoing
// redaction all apply on this high-volume path.
func (g *grpcAdapter) SubscribeLogs(req *pmmcpv1.SubscribeLogsRequest, stream pmmcpv1.Daemon_SubscribeLogsServer) error {
	ctx := stream.Context()
	if err := api.Compatible(req.GetApiVersion(), api.APIVersion); err != nil {
		return status.Error(codes.FailedPrecondition, err.Error())
	}
	p := g.s.principal(req.GetRole(), req.GetSession())
	if err := authz.Require(p, authz.CapLogsRead); err != nil {
		g.s.auditDeny(ctx, p, api.MethodLogsSubscribe, authz.CapLogsRead)
		return status.Error(codes.PermissionDenied, err.Error())
	}
	maxDur := time.Duration(req.GetMaxDurationSec()) * time.Second
	if maxDur <= 0 {
		maxDur = 30 * time.Second
	}
	if maxDur > 5*time.Minute {
		maxDur = 5 * time.Minute // hard backpressure ceiling
	}
	pid := req.GetProcessId()
	if pid == "" {
		return status.Error(codes.InvalidArgument, "process_id required")
	}
	rec, err := g.s.store.Get(ctx, pid)
	if err != nil {
		return status.Errorf(codes.NotFound, "process: %v", err)
	}
	streamName := req.GetStream()
	if streamName == "" {
		streamName = "both"
	}
	deadline := time.Now().Add(maxDur)
	// initial dump
	text, err := logcap.Tail(rec.LogDir, logcap.TailOptions{Stream: streamName, Lines: 50})
	if err == nil && text != "" {
		if err := stream.Send(&pmmcpv1.LogChunk{ProcessId: pid, Stream: streamName, Text: redactText(text)}); err != nil {
			return err
		}
	}
	stdout := filepath.Join(rec.LogDir, "stdout.log")
	stderr := filepath.Join(rec.LogDir, "stderr.log")
	offOut := fileSize(stdout)
	offErr := fileSize(stderr)
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			_ = stream.Send(&pmmcpv1.LogChunk{ProcessId: pid, Eof: true})
			return nil
		case <-g.s.runDone():
			return stream.Send(&pmmcpv1.LogChunk{ProcessId: pid, Eof: true})
		case <-ticker.C:
			var out, errText string
			out, offOut = readFollow(stdout, offOut)
			errText, offErr = readFollow(stderr, offErr)
			chunk := out + errText
			if chunk == "" {
				continue
			}
			if err := stream.Send(&pmmcpv1.LogChunk{ProcessId: pid, Stream: streamName, Text: redactText(chunk)}); err != nil {
				return err
			}
		}
	}
	return stream.Send(&pmmcpv1.LogChunk{ProcessId: pid, Eof: true})
}

// readFollow reads new bytes from off and returns the text plus the advanced
// offset (by bytes actually read, not a post-read stat). If the file shrank
// below off (rotation), it re-reads from the start.
func readFollow(path string, off int64) (string, int64) {
	if fileSize(path) < off {
		off = 0
	}
	text := readSince(path, off)
	return text, off + int64(len(text))
}

// SubscribeEvents streams domain events until max duration. Gated on version
// handshake and events:read capability.
func (g *grpcAdapter) SubscribeEvents(req *pmmcpv1.SubscribeEventsRequest, stream pmmcpv1.Daemon_SubscribeEventsServer) error {
	ctx := stream.Context()
	if err := api.Compatible(req.GetApiVersion(), api.APIVersion); err != nil {
		return status.Error(codes.FailedPrecondition, err.Error())
	}
	p := g.s.principal(req.GetRole(), req.GetSession())
	if err := authz.Require(p, authz.CapEventsRead); err != nil {
		g.s.auditDeny(ctx, p, api.MethodEventsSub, authz.CapEventsRead)
		return status.Error(codes.PermissionDenied, err.Error())
	}
	maxDur := time.Duration(req.GetMaxDurationSec()) * time.Second
	if maxDur <= 0 {
		maxDur = 30 * time.Second
	}
	if maxDur > 5*time.Minute {
		maxDur = 5 * time.Minute
	}
	deadline := time.Now().Add(maxDur)
	seen := map[string]bool{}
	// seed
	for _, e := range g.s.events.Query(ctx, req.GetProcessId(), 50) {
		seen[e.ID] = true
		if err := stream.Send(&pmmcpv1.EventChunk{
			Id: e.ID, Type: e.Type, ProcessId: e.ProcessID, Message: e.Message,
			AtUnixMs: e.At.UnixMilli(),
		}); err != nil {
			return err
		}
	}
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return stream.Send(&pmmcpv1.EventChunk{Eof: true})
		case <-g.s.runDone():
			return stream.Send(&pmmcpv1.EventChunk{Eof: true})
		case <-ticker.C:
			for _, e := range g.s.events.Query(ctx, req.GetProcessId(), 100) {
				if seen[e.ID] {
					continue
				}
				seen[e.ID] = true
				if err := stream.Send(&pmmcpv1.EventChunk{
					Id: e.ID, Type: e.Type, ProcessId: e.ProcessID, Message: e.Message,
					AtUnixMs: e.At.UnixMilli(),
				}); err != nil {
					return err
				}
			}
		}
	}
	return stream.Send(&pmmcpv1.EventChunk{Eof: true})
}

// RegisterGRPC registers the daemon on a gRPC server.
func (s *Server) RegisterGRPC(gs *grpc.Server) {
	pmmcpv1.RegisterDaemonServer(gs, &grpcAdapter{s: s})
}
