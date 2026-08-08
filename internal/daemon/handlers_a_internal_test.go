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

package daemon

import (
	"context"
	"testing"

	"github.com/scrothers/pmmcp/internal/api"
)

// TestDoProfileListContextCanceled exercises doProfileList's error path when
// s.profiles.List observes an already-cancelled context (internal/profile
// checks ctx.Err() up front). A real gRPC round trip can't reliably deliver
// an already-cancelled/expired context to the handler (grpc-go short-circuits
// client-side before any network call), so this is a direct whitebox call.
func TestDoProfileListContextCanceled(t *testing.T) {
	t.Parallel()
	s, ctx := newSuperviseTestServer(t, nil)
	cctx, cancel := context.WithCancel(ctx)
	cancel()
	p := s.principal("full", "sess-profile-list")
	resp := s.doProfileList(cctx, p, nil)
	if resp.OK {
		t.Fatal("doProfileList with a cancelled context: want error, got ok")
	}
}

// TestHandleProjectCurrentDetectContextCanceled exercises handle()'s
// project.current branch when project.Detect observes an already-cancelled
// context. Same rationale as above: not reliably reachable over a real gRPC
// round trip.
func TestHandleProjectCurrentDetectContextCanceled(t *testing.T) {
	t.Parallel()
	s, ctx := newSuperviseTestServer(t, nil)
	cctx, cancel := context.WithCancel(ctx)
	cancel()
	req := api.Request{APIVersion: api.APIVersion, Method: api.MethodProjectCurrent, Role: "full"}
	resp := s.handle(cctx, req)
	if resp.OK {
		t.Fatal("project.current with a cancelled context: want error, got ok")
	}
}
