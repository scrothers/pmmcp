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

// Package store defines durable state repository interfaces.
//
// Only the daemon process opens the database. Clients never import
// store drivers; they talk to the daemon over private IPC.
package store

import (
	"context"
	"errors"

	"github.com/scrothers/pmmcp/internal/domain"
)

// ErrNotFound is returned when a process record does not exist.
var ErrNotFound = errors.New("store: not found")

// ErrConflict is returned when a write violates a uniqueness or concurrency
// invariant: a duplicate ID, a duplicate live (project_id, name) scope, or a
// failed optimistic compare-and-swap in UpdateWithCAS.
var ErrConflict = errors.New("store: conflict")

// ProcessFilter limits List results. Zero values mean unrestricted.
type ProcessFilter struct {
	ProjectID string
	Status    domain.Status
	Name      string
}

// ProcessStore is the durable process / desired-state repository.
type ProcessStore interface {
	// Migrate applies schema migrations. Safe to call multiple times.
	Migrate(ctx context.Context) error

	// Create inserts a new process. ID must be unique.
	Create(ctx context.Context, p *domain.Process) error

	// Get returns a process by ID.
	Get(ctx context.Context, id string) (*domain.Process, error)

	// Update replaces an existing process record by ID (last-writer-wins).
	Update(ctx context.Context, p *domain.Process) error

	// UpdateWithCAS replaces an existing record only if its persisted timestamp
	// still matches p.UpdatedAt (the value the caller last read). It returns
	// ErrConflict if another writer advanced the row and ErrNotFound if the row
	// is gone, letting read-modify-write callers retry instead of silently
	// clobbering a concurrent change. On success p.UpdatedAt is advanced.
	UpdateWithCAS(ctx context.Context, p *domain.Process) error

	// Delete removes a process by ID.
	Delete(ctx context.Context, id string) error

	// List returns processes matching the filter.
	List(ctx context.Context, f ProcessFilter) ([]*domain.Process, error)

	// Close releases resources.
	Close() error
}
