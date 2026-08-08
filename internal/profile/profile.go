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

package profile

import (
	"context"
	"fmt"
	"regexp"
	"sync"

	"github.com/scrothers/pmmcp/internal/domain"
	"github.com/scrothers/pmmcp/internal/id"
)

// DefaultName is the conventional profile name when none is selected.
const DefaultName = "default"

// nameRe matches profile names: ^[a-z][a-z0-9_-]{0,63}$
var nameRe = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)

// Profile is a named workspace profile under a project.
type Profile struct {
	ID        string
	Name      string
	ProjectID string
	Env       map[string]string
}

// Store is an in-memory profile CRUD registry with per-session selection.
type Store struct {
	mu         sync.Mutex
	byID       map[string]*Profile
	byKey      map[string]string // projectID\0name -> id
	sessionUse map[string]string // session -> profile name
}

// NewStore creates an empty profile store.
func NewStore() *Store {
	return &Store{
		byID:       make(map[string]*Profile),
		byKey:      make(map[string]string),
		sessionUse: make(map[string]string),
	}
}

// Create inserts a new profile. ID is assigned if empty. Name defaults to DefaultName when empty.
func (s *Store) Create(ctx context.Context, p Profile) (Profile, error) {
	if err := ctx.Err(); err != nil {
		return Profile{}, fmt.Errorf("profile: create: %w", err)
	}
	if p.Name == "" {
		p.Name = DefaultName
	}
	if err := validateName(p.Name); err != nil {
		return Profile{}, err
	}
	if p.ProjectID == "" {
		return Profile{}, domain.NewError(domain.CodeInvalidArgument, "project_id required", false)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := profileKey(p.ProjectID, p.Name)
	if _, ok := s.byKey[key]; ok {
		return Profile{}, domain.NewError(domain.CodeConflict, "profile already exists: "+p.Name, false)
	}
	if p.ID == "" {
		pid, err := id.New(id.Profile)
		if err != nil {
			return Profile{}, fmt.Errorf("profile: create: %w", err)
		}
		p.ID = pid
	}
	if p.Env == nil {
		p.Env = map[string]string{}
	} else {
		p.Env = cloneEnv(p.Env)
	}
	cp := p
	s.byID[p.ID] = &cp
	s.byKey[key] = p.ID
	return cloneProfile(&cp), nil
}

// Get returns a profile by ID.
func (s *Store) Get(ctx context.Context, profileID string) (Profile, error) {
	if err := ctx.Err(); err != nil {
		return Profile{}, fmt.Errorf("profile: get: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.byID[profileID]
	if !ok {
		return Profile{}, domain.NewError(domain.CodeNotFound, "profile not found", false)
	}
	return cloneProfile(p), nil
}

// Update replaces mutable fields (Name, Env) for an existing profile.
func (s *Store) Update(ctx context.Context, p Profile) (Profile, error) {
	if err := ctx.Err(); err != nil {
		return Profile{}, fmt.Errorf("profile: update: %w", err)
	}
	if p.ID == "" {
		return Profile{}, domain.NewError(domain.CodeInvalidArgument, "id required", false)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cur, ok := s.byID[p.ID]
	if !ok {
		return Profile{}, domain.NewError(domain.CodeNotFound, "profile not found", false)
	}
	// ProjectID is immutable; reject an attempted move rather than silently
	// dropping it (which would leave the caller believing the profile moved).
	if p.ProjectID != "" && p.ProjectID != cur.ProjectID {
		return Profile{}, domain.NewError(domain.CodeInvalidArgument, "project_id is immutable", false)
	}
	if p.Name != "" && p.Name != cur.Name {
		if err := validateName(p.Name); err != nil {
			return Profile{}, err
		}
		oldKey := profileKey(cur.ProjectID, cur.Name)
		newKey := profileKey(cur.ProjectID, p.Name)
		if _, exists := s.byKey[newKey]; exists {
			return Profile{}, domain.NewError(domain.CodeConflict, "profile already exists: "+p.Name, false)
		}
		delete(s.byKey, oldKey)
		s.byKey[newKey] = cur.ID
		cur.Name = p.Name
	}
	if p.Env != nil {
		cur.Env = cloneEnv(p.Env)
	}
	return cloneProfile(cur), nil
}

// Delete removes a profile by ID.
func (s *Store) Delete(ctx context.Context, profileID string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("profile: delete: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.byID[profileID]
	if !ok {
		return domain.NewError(domain.CodeNotFound, "profile not found", false)
	}
	delete(s.byKey, profileKey(p.ProjectID, p.Name))
	delete(s.byID, profileID)
	return nil
}

// List returns profiles for a project (empty projectID lists all).
func (s *Store) List(ctx context.Context, projectID string) ([]Profile, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("profile: list: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Profile, 0, len(s.byID))
	for _, p := range s.byID {
		if projectID != "" && p.ProjectID != projectID {
			continue
		}
		out = append(out, cloneProfile(p))
	}
	return out, nil
}

// Use selects the active profile name for a session.
// Name may be empty to select the default profile name.
// The profile need not exist yet (selection is session metadata).
func (s *Store) Use(ctx context.Context, session, name string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("profile: use: %w", err)
	}
	if session == "" {
		return domain.NewError(domain.CodeInvalidArgument, "session required", false)
	}
	if name == "" {
		name = DefaultName
	}
	if err := validateName(name); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessionUse[session] = name
	return nil
}

// Active returns the profile name selected for session, or DefaultName when unset.
func (s *Store) Active(session string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if name, ok := s.sessionUse[session]; ok {
		return name
	}
	return DefaultName
}

// RemoveSession drops a session's profile selection. The daemon calls this from
// its session-end path to bound sessionUse growth; removing an unknown session
// is a no-op.
func (s *Store) RemoveSession(session string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessionUse, session)
}

func validateName(name string) error {
	if !nameRe.MatchString(name) {
		return domain.NewError(domain.CodeInvalidArgument, "invalid profile name: "+name, false)
	}
	return nil
}

func profileKey(projectID, name string) string {
	return projectID + "\x00" + name
}

func cloneEnv(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneProfile(p *Profile) Profile {
	cp := *p
	cp.Env = cloneEnv(p.Env)
	return cp
}
