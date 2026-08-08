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
	"time"
)

// Sentinel errors for domain validation.
var (
	ErrInvalidCommand = errors.New("domain: invalid command")
	ErrInvalidProcess = errors.New("domain: invalid process")
)

// Process is a managed workload record (pure value type).
type Process struct {
	ID        string
	Name      string
	Command   []string
	Cwd       string
	Status    Status
	Desired   Desired
	ProjectID string
	Profile   string
	SessionID string
	Sandbox   string
	Runtime   string // local | container
	PID       int
	ExitCode  *int
	LastError string
	LogDir    string
	// EnvKeys lists injected env key names only (never values).
	EnvKeys []string
	// PredecessorID / SuccessorID link restart generations.
	PredecessorID string
	SuccessorID   string
	CreatedAt     time.Time
	UpdatedAt     time.Time
	StartedAt     *time.Time
	ExitedAt      *time.Time
}

// ValidateCommand checks that command is a non-empty argv list (no implicit shell).
// Each element must be non-empty after considering the list as argv, not a shell string.
func ValidateCommand(command []string) error {
	if len(command) == 0 {
		return fmt.Errorf("%w: empty argv", ErrInvalidCommand)
	}
	for i, a := range command {
		if a == "" {
			return fmt.Errorf("%w: empty argument at index %d", ErrInvalidCommand, i)
		}
	}
	return nil
}

// Validate performs field checks on a Process value (identity and command shape).
// It does not talk to the OS or store.
func (p *Process) Validate() error {
	if p == nil {
		return fmt.Errorf("%w: nil", ErrInvalidProcess)
	}
	if p.Name == "" {
		return fmt.Errorf("%w: empty name", ErrInvalidProcess)
	}
	if err := ValidateCommand(p.Command); err != nil {
		return err
	}
	if p.Status != "" && !p.Status.Valid() {
		return fmt.Errorf("%w: status %q", ErrInvalidProcess, p.Status)
	}
	if p.Desired != "" && !p.Desired.Valid() {
		return fmt.Errorf("%w: desired %q", ErrInvalidProcess, p.Desired)
	}
	return nil
}

// Group is a named collection of processes with optional ordering metadata.
type Group struct {
	ID        string
	Name      string
	ProjectID string
	// MemberNames are process names in start order (depends_on resolved elsewhere).
	MemberNames []string
}

// Project is a path-scoped workspace key.
type Project struct {
	ID string
	// Path is the canonical absolute project root (opaque to domain purity: just a string).
	Path string
	Name string
}

// Profile is a named configuration overlay within a project.
type Profile struct {
	ID        string
	Name      string
	ProjectID string
}
