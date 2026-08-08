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
	"time"

	"github.com/scrothers/pmmcp/internal/authz"
	"github.com/scrothers/pmmcp/internal/group"
	"github.com/scrothers/pmmcp/internal/profile"
	"github.com/scrothers/pmmcp/internal/webhook"
)

// subInfo is an in-memory logs/events subscription registration.
// Polling still happens via logs/events methods; subscribe only returns an id.
type subInfo struct {
	ID        string
	Kind      string // "logs" | "events"
	ProcessID string
	CreatedAt time.Time
}

// productState holds registries and runtime maps for product-path handlers.
// Server embeds these fields directly; this file documents and helpers only.
// webhookAllowlist configures the outbound webhook egress allowlist;
// an empty allowlist disables webhooks (secure by default).
func newProductState(webhookAllowlist []string) (
	groups *group.Registry,
	profiles *profile.Store,
	hooks *webhook.Registry,
	shares *authz.ShareBook,
	projects map[string]string,
	secrets map[string]string,
	watches map[string]string,
	subs map[string]subInfo,
	healthURL map[string]string,
	autoRestart map[string]bool,
	ports map[string][]string,
	procEnv map[string][]string,
) {
	return group.NewRegistry(),
		profile.NewStore(),
		webhook.NewRegistry(webhook.WithAllowlist(webhookAllowlist...)),
		authz.NewShareBook(),
		make(map[string]string),
		make(map[string]string),
		make(map[string]string),
		make(map[string]subInfo),
		make(map[string]string),
		make(map[string]bool),
		make(map[string][]string),
		make(map[string][]string)
}
