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

package domain

import (
	"errors"
	"fmt"
)

// Code is a stable machine-readable error code for CLI/MCP/gRPC mapping.
type Code string

// Well-known error codes (docs/reference-errors.md + exit map).
const (
	CodeNotFound           Code = "not_found"
	CodeConflict           Code = "conflict"
	CodeNameConflict       Code = "name_conflict"
	CodeAlreadyExists      Code = "already_exists"
	CodeInvalidArgument    Code = "invalid_argument"
	CodePermissionDenied   Code = "permission_denied"
	CodeDaemonUnavailable  Code = "daemon_unavailable"
	CodeIPCVersionMismatch Code = "ipc_version_mismatch"
	CodeSandboxFailed      Code = "sandbox_failed"
	CodeSpawnFailed        Code = "spawn_failed"
	CodeUnimplemented      Code = "unimplemented"
	CodeInternal           Code = "internal"
	CodeFailedPrecondition Code = "failed_precondition"
)

// ExitCode maps a domain error code to a CLI process exit code.
func ExitCode(code Code) int {
	switch code {
	case "":
		return 0
	case CodeInvalidArgument:
		return 2
	case CodeDaemonUnavailable:
		return 3
	case CodeNotFound:
		return 4
	case CodePermissionDenied:
		return 5
	case CodeConflict, CodeNameConflict, CodeAlreadyExists:
		return 6
	case CodeSandboxFailed:
		return 7
	case CodeIPCVersionMismatch:
		return 8
	default:
		return 1
	}
}

// ExitCodeFromError returns the CLI exit code for err.
func ExitCodeFromError(err error) int {
	if err == nil {
		return 0
	}
	var de *Error
	if errors.As(err, &de) {
		// A non-nil structured error with an unset Code must never report
		// success; treat it as the generic failure (exit 1), not exit 0.
		if de.Code == "" {
			return 1
		}
		return ExitCode(de.Code)
	}
	return 1
}

// Error is a structured domain error.
type Error struct {
	Code      Code
	Message   string
	Retryable bool
	// Details carries structured, machine-branchable context for the error
	// envelope (docs/reference-errors.md), e.g. {"name": "api"}. Optional.
	Details map[string]any
	Err     error
}

func (e *Error) Error() string {
	if e == nil {
		return "domain: nil error"
	}
	if e.Err != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *Error) Unwrap() error { return e.Err }

// NewError builds a structured error.
func NewError(code Code, msg string, retryable bool) *Error {
	return &Error{Code: code, Message: msg, Retryable: retryable}
}

// WrapError wraps an underlying error with a code.
func WrapError(code Code, msg string, retryable bool, err error) *Error {
	return &Error{Code: code, Message: msg, Retryable: retryable, Err: err}
}

// WithDetails attaches structured details and returns the same error for chaining.
func (e *Error) WithDetails(details map[string]any) *Error {
	if e == nil {
		return nil
	}
	e.Details = details
	return e
}
