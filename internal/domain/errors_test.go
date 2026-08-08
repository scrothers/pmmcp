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

package domain_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/scrothers/pmmcp/internal/domain"
)

func TestExitCodeMap(t *testing.T) {
	t.Parallel()
	cases := map[domain.Code]int{
		"":                            0,
		domain.CodeInvalidArgument:    2,
		domain.CodeDaemonUnavailable:  3,
		domain.CodeNotFound:           4,
		domain.CodePermissionDenied:   5,
		domain.CodeConflict:           6,
		domain.CodeNameConflict:       6,
		domain.CodeAlreadyExists:      6,
		domain.CodeSandboxFailed:      7,
		domain.CodeIPCVersionMismatch: 8,
		domain.CodeInternal:           1,
		domain.CodeSpawnFailed:        1,
		domain.CodeUnimplemented:      1,
		domain.CodeFailedPrecondition: 1,
	}
	for code, want := range cases {
		if got := domain.ExitCode(code); got != want {
			t.Errorf("ExitCode(%q)=%d want %d", code, got, want)
		}
	}
}

func TestExitCodeFromError(t *testing.T) {
	t.Parallel()
	if got := domain.ExitCodeFromError(nil); got != 0 {
		t.Errorf("nil err = %d, want 0", got)
	}
	if got := domain.ExitCodeFromError(domain.NewError(domain.CodeDaemonUnavailable, "down", true)); got != 3 {
		t.Errorf("daemon_unavailable = %d, want 3", got)
	}
	// A non-nil structured error with an unset Code must map to failure, not success.
	if got := domain.ExitCodeFromError(&domain.Error{Message: "boom"}); got != 1 {
		t.Errorf("empty-code error = %d, want 1", got)
	}
	// A plain non-domain error is the generic failure.
	if got := domain.ExitCodeFromError(errors.New("plain")); got != 1 {
		t.Errorf("non-domain error = %d, want 1", got)
	}
	// Wrapped domain errors are unwrapped via errors.As.
	wrapped := fmt.Errorf("context: %w", domain.NewError(domain.CodeNotFound, "gone", false))
	if got := domain.ExitCodeFromError(wrapped); got != 4 {
		t.Errorf("wrapped not_found = %d, want 4", got)
	}
}

func TestErrorFormatting(t *testing.T) {
	t.Parallel()
	base := errors.New("root cause")
	we := domain.WrapError(domain.CodeInternal, "failed", true, base)
	if got := we.Error(); !strings.Contains(got, "root cause") || !strings.Contains(got, "internal") {
		t.Errorf("Error() = %q", got)
	}
	if !errors.Is(we, base) {
		t.Error("Unwrap chain broken: errors.Is base = false")
	}
	ne := domain.NewError(domain.CodeNotFound, "gone", false)
	if got := ne.Error(); got != "not_found: gone" {
		t.Errorf("Error() = %q, want %q", got, "not_found: gone")
	}
	if ne.Unwrap() != nil {
		t.Error("Unwrap on non-wrapping error should be nil")
	}
	var nilErr *domain.Error
	if got := nilErr.Error(); got != "domain: nil error" {
		t.Errorf("nil Error() = %q", got)
	}
}

func TestErrorWithDetails(t *testing.T) {
	t.Parallel()
	e := domain.NewError(domain.CodeAlreadyExists, "dup", false).
		WithDetails(map[string]any{"name": "api", "existing_id": "proc-1"})
	if e.Details["name"] != "api" {
		t.Errorf("Details = %v", e.Details)
	}
	var nilErr *domain.Error
	if nilErr.WithDetails(map[string]any{"x": 1}) != nil {
		t.Error("WithDetails on nil should return nil")
	}
}

func TestDesiredValid(t *testing.T) {
	t.Parallel()
	if !domain.DesiredRunning.Valid() || !domain.DesiredStopped.Valid() {
		t.Error("known desired states should be valid")
	}
	if domain.Desired("paused").Valid() {
		t.Error("unknown desired state should be invalid")
	}
}

func TestProcessValidateRejectsBadStatusAndDesired(t *testing.T) {
	t.Parallel()
	badStatus := &domain.Process{Name: "x", Command: []string{"true"}, Status: domain.Status("bogus")}
	if err := badStatus.Validate(); !errors.Is(err, domain.ErrInvalidProcess) {
		t.Errorf("bad status err = %v", err)
	}
	badDesired := &domain.Process{Name: "x", Command: []string{"true"}, Desired: domain.Desired("paused")}
	if err := badDesired.Validate(); !errors.Is(err, domain.ErrInvalidProcess) {
		t.Errorf("bad desired err = %v", err)
	}
}
