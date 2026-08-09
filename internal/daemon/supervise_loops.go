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

package daemon

import (
	"context"
	"fmt"
	"time"

	"github.com/scrothers/pmmcp/internal/domain"
	"github.com/scrothers/pmmcp/internal/event"
	"github.com/scrothers/pmmcp/internal/process"
	"github.com/scrothers/pmmcp/internal/supervise"
	"github.com/scrothers/pmmcp/internal/watch"
)

// runAutoRestartLoop restarts processes marked autoRestart when they exit or go unhealthy.
// Restart counters reset after a stable healthy window.
func (s *Server) runAutoRestartLoop(ctx context.Context) {
	policy := supervise.RestartPolicy{
		Enabled:      true,
		Max:          s.autoRestartMax,
		Backoff:      s.autoRestartBackoff,
		StableWindow: 5 * time.Second,
	}
	t := time.NewTicker(s.autoRestartTick)
	defer t.Stop()
	counters := map[string]*supervise.RestartCounter{}
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			s.mu.Lock()
			ids := make([]string, 0, len(s.autoRestart))
			for id, on := range s.autoRestart {
				if on {
					ids = append(ids, id)
				}
			}
			health := make(map[string]string, len(s.healthURL))
			for k, v := range s.healthURL {
				health[k] = v
			}
			s.mu.Unlock()
			for _, id := range ids {
				c := counters[id]
				if c == nil {
					c = supervise.NewRestartCounter(policy)
					counters[id] = c
				}
				running, healthy := s.probeRunningHealthy(ctx, id, health[id])
				if running && healthy {
					c.ObserveHealthy(now)
					continue
				}
				c.ObserveUnhealthy(now)
				if !c.ShouldRestart() {
					continue
				}
				time.Sleep(supervise.NextBackoff(policy, c.Count()))
				if err := s.restartByID(ctx, id); err == nil {
					c.RecordRestart()
					_, _ = s.events.Append(ctx, event.Event{
						Type: "process.auto_restarted", ProcessID: id, Message: "auto_restart",
					})
				}
			}
		}
	}
}

func (s *Server) probeRunningHealthy(ctx context.Context, id, healthURL string) (running, healthy bool) {
	h, err := s.mgr.Inspect(ctx, id)
	if err != nil || h == nil {
		return false, false
	}
	switch h.Status {
	case domain.StatusRunning, domain.StatusUnhealthy, domain.StatusStarting:
		// continue to health probe
	default:
		return false, false
	}
	if healthURL == "" {
		return true, true
	}
	pr := supervise.ProbeHTTP(ctx, healthURL, 2*time.Second)
	if !pr.OK {
		if rec, err := s.store.Get(ctx, id); err == nil {
			rec.Status = domain.StatusUnhealthy
			_ = s.store.Update(ctx, rec)
		}
		return true, false
	}
	return true, true
}

func (s *Server) restartByID(ctx context.Context, id string) error {
	rec, err := s.store.Get(ctx, id)
	if err != nil {
		return err
	}
	_ = s.mgr.Stop(ctx, id, 5*time.Second)
	s.mu.Lock()
	env := append([]string(nil), s.procEnv[id]...)
	mem := s.memLimit[id]
	s.mu.Unlock()
	h, err := s.mgr.Start(ctx, process.StartSpec{
		ID: rec.ID, Name: rec.Name, Command: rec.Command, Cwd: rec.Cwd,
		Env: env, LogDir: rec.LogDir, Sandbox: rec.Sandbox, Runtime: rec.Runtime,
		MemoryBytes: mem,
	})
	if err != nil {
		return err
	}
	rec.Status = domain.StatusRunning
	rec.Desired = domain.DesiredRunning
	rec.PID = h.PID
	rec.ExitCode = nil
	now := time.Now().UTC()
	rec.StartedAt = &now
	rec.ExitedAt = nil
	s.recordStartTime(rec.ID, h.PID)
	return s.store.Update(ctx, rec)
}

// runWatchDispatchers is a no-op placeholder; actual watchers start in doWatchSet.
func (s *Server) runWatchDispatchers(ctx context.Context) {
	<-ctx.Done()
}

func (s *Server) stopAllWatchers() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, w := range s.watchers {
		_ = w.Close()
		delete(s.watchers, id)
	}
}

// startWatchForProcess starts a debounced watcher that restarts the process on
// change. The consumer goroutine below calls restartByID/events.Append —
// real store writes — so it is tracked in s.bg the same as the fixed
// supervision loops: Close must wait for it before closing the store, or a
// watch event landing right at shutdown races the SQLite handle going away
// (and, on Windows, leaves the database file undeletable).
func (s *Server) startWatchForProcess(ctx context.Context, processID, path string) error {
	s.mu.Lock()
	if s.closing {
		s.mu.Unlock()
		return fmt.Errorf("daemon: shutting down")
	}
	if old, ok := s.watchers[processID]; ok {
		_ = old.Close()
		delete(s.watchers, processID)
	}
	s.mu.Unlock()

	w := watch.New(
		watch.WithDebounce(100*time.Millisecond),
		watch.WithPollInterval(50*time.Millisecond),
	)
	if err := w.Add(path); err != nil {
		return err
	}
	w.Start(ctx)
	s.mu.Lock()
	// Re-check under the same lock Close takes for its closing flag: had Close
	// run between the unlock above and here, bg.Add below could otherwise race
	// a bg.Wait already in progress (sync.WaitGroup requires Add-from-zero to
	// happen-before the Wait it competes with).
	if s.closing {
		s.mu.Unlock()
		_ = w.Close()
		return fmt.Errorf("daemon: shutting down")
	}
	s.watchers[processID] = w
	s.watches[processID] = path
	s.bg.Add(1)
	s.mu.Unlock()

	go func() {
		defer s.bg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-w.Events():
				if !ok {
					return
				}
				_ = ev
				if err := s.restartByID(ctx, processID); err != nil {
					_, _ = s.events.Append(ctx, event.Event{
						Type: "process.watch_restart_error", ProcessID: processID, Message: err.Error(),
					})
					continue
				}
				_, _ = s.events.Append(ctx, event.Event{
					Type: "process.watch_restart", ProcessID: processID, Message: path,
				})
			}
		}
	}()
	return nil
}
