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

package daemon

import (
	"context"
	"log/slog"
	"time"

	"github.com/scrothers/pmmcp/internal/event"
	"github.com/scrothers/pmmcp/internal/webhook"
)

// webhookDeliverFunc delivers one event to one hook. Production uses
// webhook.Deliverer.DeliverEvent (SSRF-guarded, HMAC-signed when the hook has a
// secret); tests inject a recorder to assert delivery without real egress.
type webhookDeliverFunc func(ctx context.Context, hook webhook.Hook, ev webhook.Event) error

// defaultWebhookDeliver is the production delivery path.
func defaultWebhookDeliver(ctx context.Context, hook webhook.Hook, ev webhook.Event) error {
	return (&webhook.Deliverer{}).DeliverEvent(ctx, hook, ev)
}

// runWebhookDispatch polls the event bus and delivers new events to matching
// webhooks. The cursor is seeded to the latest event at startup so a
// daemon restart does not replay history, and it advances while idle so no
// backlog accumulates when there are no hooks.
func (s *Server) runWebhookDispatch(ctx context.Context) {
	interval := s.webhookPoll
	if interval <= 0 {
		interval = 2 * time.Second
	}
	cursor := s.latestEventSeq(ctx)
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			hooks := s.hooks.List()
			if len(hooks) == 0 {
				cursor = s.latestEventSeq(ctx)
				continue
			}
			for _, e := range s.events.QuerySince(ctx, cursor, "", 100) {
				if e.Seq > cursor {
					cursor = e.Seq
				}
				s.dispatchEventToHooks(ctx, hooks, e)
			}
		}
	}
}

// latestEventSeq returns the sequence number of the most recent event, or 0.
func (s *Server) latestEventSeq(ctx context.Context) int64 {
	evs := s.events.Query(ctx, "", 1)
	if len(evs) == 0 {
		return 0
	}
	return evs[len(evs)-1].Seq
}

// dispatchEventToHooks delivers one event to every hook whose filter matches.
func (s *Server) dispatchEventToHooks(ctx context.Context, hooks []webhook.Hook, e event.Event) {
	ev := webhook.Event{
		Type: e.Type,
		Payload: map[string]any{
			"id":         e.ID,
			"type":       e.Type,
			"process_id": e.ProcessID,
			"message":    e.Message,
			"at":         e.At,
		},
	}
	for _, h := range hooks {
		if !h.Matches(e.Type) {
			continue
		}
		s.deliverWithRetry(ctx, h, ev)
	}
}

// deliverWithRetry attempts delivery with bounded exponential backoff, giving up
// (and logging) after three tries. It stops early if the context is cancelled so
// daemon shutdown is not delayed.
func (s *Server) deliverWithRetry(ctx context.Context, h webhook.Hook, ev webhook.Event) {
	const maxAttempts = 3
	backoff := s.webhookRetryBackoff
	for attempt := range maxAttempts {
		err := s.deliver(ctx, h, ev)
		if err == nil {
			return
		}
		if ctx.Err() != nil {
			return
		}
		if attempt == maxAttempts-1 {
			slog.Warn("webhook delivery failed", "hook", h.ID, "error", err.Error())
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff *= 2
	}
}
