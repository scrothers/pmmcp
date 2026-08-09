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

package daemon_test

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/scrothers/pmmcp/internal/api"
)

func TestGRPCSubscribeLogsStream(t *testing.T) {
	t.Parallel()
	ctx, _, c, _ := startTestDaemon(t)
	var start api.StartResult
	if err := c.Call(ctx, api.MethodStart, api.StartPayload{
		// "sh" is resolved via PATH: /bin/sh on Linux/macOS, Git's sh.exe on
		// Windows CI runners (which ships true/echo/sleep too) — a hardcoded
		// /bin/sh path doesn't exist on Windows.
		Name: "logstream", Command: []string{"sh", "-c", "echo STREAM_MARKER; sleep 0.2"},
		Sandbox: "off",
	}, &start); err != nil {
		t.Fatal(err)
	}
	// Allow process to write.
	time.Sleep(100 * time.Millisecond)

	sctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	// 1s max duration: the stream must deliver the initial dump immediately
	// and then hit the natural-deadline EOF branch.
	stream, err := c.SubscribeLogs(sctx, start.ID, "both", 1)
	if err != nil {
		t.Fatalf("SubscribeLogs: %v", err)
	}
	saw := false
	for {
		chunk, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			// context cancel / deadline after eof is fine once we saw data
			if saw {
				break
			}
			t.Fatalf("Recv: %v", err)
		}
		if chunk.GetText() != "" {
			saw = true
		}
		if chunk.GetEof() {
			break
		}
	}
	if !saw {
		t.Fatal("expected at least one log chunk from gRPC SubscribeLogs stream")
	}
}
