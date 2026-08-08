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
	"net"
)

// SetLookupIPAddrForTest overrides the DNS-resolution hook used by
// validateLoopbackURL and returns a function that restores the previous
// hook. It exists so black-box tests can deterministically exercise
// resolver error paths (failure, zero addresses, mixed loopback/non-loopback
// results) without touching a real network or DNS server.
func SetLookupIPAddrForTest(f func(ctx context.Context, host string) ([]net.IPAddr, error)) (restore func()) {
	prev := lookupIPAddr
	lookupIPAddr = f
	return func() { lookupIPAddr = prev }
}
