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

package process

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

// Router dispatches to local or container managers per process ID.
// It is the product-path selector so Runtime/Image on StartSpec take effect.
type Router struct {
	Local   Manager
	Open    func(name string) (Manager, error) // typically process/drivers.Open
	mu      sync.Mutex
	byID    map[string]Manager
	runtime map[string]string
}

// NewRouter builds a router with a required local backend.
func NewRouter(local Manager, open func(string) (Manager, error)) *Router {
	return &Router{
		Local:   local,
		Open:    open,
		byID:    make(map[string]Manager),
		runtime: make(map[string]string),
	}
}

// Start implements Manager.
func (r *Router) Start(ctx context.Context, spec StartSpec) (*Handle, error) {
	m, rt, err := r.pick(spec)
	if err != nil {
		return nil, err
	}
	h, err := m.Start(ctx, spec)
	if err != nil {
		return nil, err
	}
	r.mu.Lock()
	r.byID[spec.ID] = m
	r.runtime[spec.ID] = rt
	r.mu.Unlock()
	return h, nil
}

func (r *Router) pick(spec StartSpec) (Manager, string, error) {
	rt := strings.ToLower(strings.TrimSpace(spec.Runtime))
	if rt == "" {
		if spec.Image != "" {
			rt = "container"
		} else {
			rt = "local"
		}
	}
	if rt == "local" {
		return r.Local, "local", nil
	}
	// Normalize runtime aliases to a process/drivers name.
	// container | container:auto → container; bare podman/docker → container:<engine>.
	name := rt
	switch rt {
	case "container", "container:auto":
		name = "container"
	case "podman":
		name = "container:podman"
	case "docker":
		name = "container:docker"
	}
	if r.Open == nil {
		return nil, rt, fmt.Errorf("%w: no container driver open func", ErrInvalidSpec)
	}
	m, err := r.Open(name)
	if err != nil {
		return nil, rt, err
	}
	return m, rt, nil
}

func (r *Router) mgr(id string) Manager {
	r.mu.Lock()
	defer r.mu.Unlock()
	if m, ok := r.byID[id]; ok {
		return m
	}
	return r.Local
}

// Stop implements Manager.
func (r *Router) Stop(ctx context.Context, id string, timeout time.Duration) error {
	return r.mgr(id).Stop(ctx, id, timeout)
}

// Wait implements Manager.
func (r *Router) Wait(ctx context.Context, id string) (*Handle, error) {
	return r.mgr(id).Wait(ctx, id)
}

// Inspect implements Manager.
func (r *Router) Inspect(ctx context.Context, id string) (*Handle, error) {
	return r.mgr(id).Inspect(ctx, id)
}

// Signal implements Manager.
func (r *Router) Signal(ctx context.Context, id string, sig os.Signal) error {
	return r.mgr(id).Signal(ctx, id, sig)
}

// RuntimeOf returns the runtime label recorded at Start.
func (r *Router) RuntimeOf(id string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.runtime[id]
}

// Forget drops routing state after remove.
func (r *Router) Forget(id string) {
	r.mu.Lock()
	delete(r.byID, id)
	delete(r.runtime, id)
	r.mu.Unlock()
}

var _ Manager = (*Router)(nil)
