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
	"testing"
	"time"

	"github.com/scrothers/pmmcp/internal/domain"
	"github.com/scrothers/pmmcp/internal/ipc"
	"github.com/scrothers/pmmcp/internal/testsock"
)

// TestDaemonUnavailableWhenDown proves dial fails with daemon_unavailable when no pmmcpd listens.
func TestDaemonUnavailableWhenDown(t *testing.T) {
	t.Parallel()
	sock := testsock.Path(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := ipc.Dial(ctx, sock)
	if err == nil {
		t.Fatal("expected dial error when daemon is down")
	}
	var de *domain.Error
	if !errors.As(err, &de) {
		t.Fatalf("want *domain.Error, got %T %v", err, err)
	}
	if de.Code != domain.CodeDaemonUnavailable {
		t.Fatalf("code = %q, want %q (msg=%s)", de.Code, domain.CodeDaemonUnavailable, de.Message)
	}
	if !de.Retryable {
		t.Fatal("daemon_unavailable should be retryable")
	}
}
