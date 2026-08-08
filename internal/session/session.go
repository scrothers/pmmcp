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

package session

import (
	"sync"
	"time"

	"github.com/scrothers/pmmcp/internal/id"
)

// Session is a 1:1 client connection context.
type Session struct {
	ID        string
	HarnessID string // preferred external id when provided by harness
	Role      string
	CreatedAt time.Time
	EndedAt   *time.Time
}

// Registry tracks live sessions, indexed by internal id and by harness id.
type Registry struct {
	mu        sync.Mutex
	byID      map[string]*Session
	byHarness map[string]*Session
}

// NewRegistry creates an empty registry.
func NewRegistry() *Registry {
	return &Registry{
		byID:      make(map[string]*Session),
		byHarness: make(map[string]*Session),
	}
}

// Open returns a session for harnessID, reusing the existing one when a live
// session already carries that harness id so a repeated harness id keeps a
// single identity (attribution continuity) instead of minting a new session
// per request. When harnessID is empty a fresh anonymous session is created.
// The internal ID is always a server-assigned sess- ULID.
func (r *Registry) Open(harnessID, role string) (*Session, error) {
	r.mu.Lock()
	if harnessID != "" {
		if existing, ok := r.byHarness[harnessID]; ok {
			cp := *existing
			r.mu.Unlock()
			return &cp, nil
		}
	}
	r.mu.Unlock()

	// Generate the ID outside the lock (crypto/rand may block).
	sid, err := id.New(id.Session)
	if err != nil {
		return nil, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	// Re-check under the lock: a concurrent Open may have won the race.
	if harnessID != "" {
		if existing, ok := r.byHarness[harnessID]; ok {
			cp := *existing
			return &cp, nil
		}
	}
	s := &Session{
		ID:        sid,
		HarnessID: harnessID,
		Role:      role,
		CreatedAt: time.Now().UTC(),
	}
	r.byID[s.ID] = s
	if harnessID != "" {
		r.byHarness[harnessID] = s
	}
	cp := *s
	return &cp, nil
}

// Get returns a copy of the session with the given internal id. Callers receive
// a snapshot, never the registry's own pointer, so End cannot mutate a struct a
// caller is reading. A match does not authenticate the caller as the session's
// owner — under the shared-UID model the daemon must bind identity to the
// transport connection and treat request-carried ids as labels only.
func (r *Registry) Get(internalID string) (*Session, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.byID[internalID]
	if !ok {
		return nil, false
	}
	cp := *s
	return &cp, true
}

// GetByHarness returns a copy of the live session carrying harnessID, if any.
func (r *Registry) GetByHarness(harnessID string) (*Session, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if harnessID == "" {
		return nil, false
	}
	s, ok := r.byHarness[harnessID]
	if !ok {
		return nil, false
	}
	cp := *s
	return &cp, true
}

// End removes a session by internal id, bounding registry growth. It returns
// whether the session existed.
func (r *Registry) End(internalID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.byID[internalID]
	if !ok {
		return false
	}
	delete(r.byID, internalID)
	if s.HarnessID != "" {
		delete(r.byHarness, s.HarnessID)
	}
	return true
}

// PrimaryID returns harness id when set, else internal session id.
func (s *Session) PrimaryID() string {
	if s.HarnessID != "" {
		return s.HarnessID
	}
	return s.ID
}
