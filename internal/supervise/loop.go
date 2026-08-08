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

package supervise

import (
	"context"
	"time"

	"github.com/scrothers/pmmcp/internal/domain"
)

// MonitorFunc inspects whether a process is still healthy/running.
type MonitorFunc func(ctx context.Context, processID string) (running bool, healthy bool, err error)

// RestartFunc restarts a process by id.
type RestartFunc func(ctx context.Context, processID string) error

// DesiredFunc reports the durable desired state of a process. When it returns a
// value other than DesiredRunning, CrashLoop treats the process as
// intentionally stopped and never restarts it (restart-policy design:
// intentional stops must not trigger restarts).
type DesiredFunc func(ctx context.Context, processID string) domain.Desired

// ExhaustedFunc is invoked once per process when it exhausts its restart budget,
// so the caller can mark it failed and emit process.restart_exhausted.
type ExhaustedFunc func(ctx context.Context, processID string)

// CrashLoopConfig configures CrashLoop.
type CrashLoopConfig struct {
	// Period is the monitor tick interval (default 2s).
	Period time.Duration
	// IDs are the process ids to supervise.
	IDs []string
	// Policy governs restart budget, stable-window reset, and backoff.
	Policy RestartPolicy
	// Monitor reports running/healthy per id (required).
	Monitor MonitorFunc
	// Restart performs the restart (required).
	Restart RestartFunc
	// Desired, when set, gates restarts on desired == running so intentional
	// stops are not restarted. Nil treats every id as desired-running.
	Desired DesiredFunc
	// OnExhausted, when set, is called once per id when its budget is exhausted.
	OnExhausted ExhaustedFunc
}

// CrashLoop supervises processes and restarts them per policy until ctx is
// cancelled.
//
// Backoff is non-blocking: an id in backoff is skipped on subsequent ticks until
// its next-attempt deadline passes, so one id's backoff never stalls monitoring
// of the others and cancellation is honored promptly. Budget is tracked per id
// via RestartCounter, so a stable healthy window resets the counter and every
// attempt (success or failure) consumes budget. Intentional stops (Desired not
// running) never restart.
func CrashLoop(ctx context.Context, cfg CrashLoopConfig) {
	period := cfg.Period
	if period <= 0 {
		period = 2 * time.Second
	}
	t := time.NewTicker(period)
	defer t.Stop()

	counters := make(map[string]*RestartCounter, len(cfg.IDs))
	nextAttempt := make(map[string]time.Time, len(cfg.IDs))
	exhausted := make(map[string]bool, len(cfg.IDs))

	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			for _, id := range cfg.IDs {
				c := counters[id]
				if c == nil {
					c = NewRestartCounter(cfg.Policy)
					counters[id] = c
				}
				running, healthy, err := cfg.Monitor(ctx, id)
				if err == nil && running && healthy {
					c.ObserveHealthy(now)
					if c.Count() == 0 {
						exhausted[id] = false
					}
					continue
				}
				if cfg.Desired != nil && cfg.Desired(ctx, id) != domain.DesiredRunning {
					continue
				}
				c.ObserveUnhealthy(now)
				if !c.ShouldRestart() {
					if !exhausted[id] {
						exhausted[id] = true
						if cfg.OnExhausted != nil {
							cfg.OnExhausted(ctx, id)
						}
					}
					continue
				}
				if deadline, ok := nextAttempt[id]; ok && now.Before(deadline) {
					continue
				}
				// Count the attempt whether or not the restart succeeds: a
				// persistent spawn failure must consume budget, not spin forever.
				_ = cfg.Restart(ctx, id)
				c.RecordRestart()
				nextAttempt[id] = now.Add(NextBackoff(cfg.Policy, c.Count()))
			}
		}
	}
}

// MapStatus applies a health probe result onto a domain status.
func MapStatus(running bool, healthy bool) domain.Status {
	if !running {
		return domain.StatusExited
	}
	if !healthy {
		return domain.StatusUnhealthy
	}
	return domain.StatusRunning
}

// MapStatusExit refines MapStatus with exit context: a process that is not
// running maps to StatusCrashed when it exited non-zero, else StatusExited.
func MapStatusExit(running bool, healthy bool, exitCode *int) domain.Status {
	if running {
		if !healthy {
			return domain.StatusUnhealthy
		}
		return domain.StatusRunning
	}
	if exitCode != nil && *exitCode != 0 {
		return domain.StatusCrashed
	}
	return domain.StatusExited
}
