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

package project_test

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/scrothers/pmmcp/internal/project"
)

// flakyCtx implements context.Context and makes Err() return context.Canceled
// once its call counter reaches failAtN (1-indexed). Used to exercise Detect's
// ctx.Err() checks without racing a real timeout. Stores no context.Context
// field (containedctx).
type flakyCtx struct {
	calls   int32
	failAtN int32
}

func (f *flakyCtx) Deadline() (deadline time.Time, ok bool) { return time.Time{}, false }
func (f *flakyCtx) Done() <-chan struct{}                   { return nil }
func (f *flakyCtx) Value(_ any) any                         { return nil }

func (f *flakyCtx) Err() error {
	n := atomic.AddInt32(&f.calls, 1)
	if f.failAtN > 0 && n >= f.failAtN {
		return context.Canceled
	}
	return nil
}

func TestDetectConfigWalkContextCanceled(t *testing.T) {
	t.Parallel()
	// Call #1 is Detect's own top-level ctx.Err() check; call #2 is the
	// first iteration of the config-marker walkUp, which must observe the
	// cancellation and propagate it back through Detect's first walkUp
	// error branch.
	ctx := &flakyCtx{failAtN: 2}
	_, _, err := project.Detect(ctx, t.TempDir())
	if err == nil {
		t.Fatal("Detect: want error on canceled context during config walk")
	}
}

func TestDetectGitWalkContextCanceled(t *testing.T) {
	t.Parallel()
	// No pmmcp.yaml so Detect falls through to the .git walk. failAtN is
	// high enough to pass the top-level check and the empty config walk,
	// then trip inside the git walk.
	dir := t.TempDir()
	nested := filepath.Join(dir, "a", "b")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	ctx := &flakyCtx{failAtN: 4}
	_, _, err := project.Detect(ctx, nested)
	if err == nil {
		t.Fatal("Detect: want error on canceled context during git walk")
	}
}
