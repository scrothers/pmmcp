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

import "fmt"

// Status is the observed lifecycle state of a managed process.
type Status string

// Process status values (status vocabulary, ).
const (
	StatusStarting  Status = "starting"
	StatusRunning   Status = "running"
	StatusUnhealthy Status = "unhealthy"
	StatusStopping  Status = "stopping"
	StatusExited    Status = "exited"
	StatusFailed    Status = "failed"
	StatusCrashed   Status = "crashed"
)

// AllStatuses is the full set of valid process statuses.
var AllStatuses = []Status{
	StatusStarting,
	StatusRunning,
	StatusUnhealthy,
	StatusStopping,
	StatusExited,
	StatusFailed,
	StatusCrashed,
}

// Valid reports whether s is a known process status.
func (s Status) Valid() bool {
	switch s {
	case StatusStarting, StatusRunning, StatusUnhealthy, StatusStopping,
		StatusExited, StatusFailed, StatusCrashed:
		return true
	default:
		return false
	}
}

// ParseStatus validates and returns a Status.
func ParseStatus(s string) (Status, error) {
	st := Status(s)
	if !st.Valid() {
		return "", fmt.Errorf("domain: invalid status %q", s)
	}
	return st, nil
}

// Desired is the reconcilable desired state for a process.
type Desired string

// Desired state values (status vocabulary).
const (
	DesiredRunning Desired = "running"
	DesiredStopped Desired = "stopped"
)

// Valid reports whether d is a known desired state.
func (d Desired) Valid() bool {
	switch d {
	case DesiredRunning, DesiredStopped:
		return true
	default:
		return false
	}
}
