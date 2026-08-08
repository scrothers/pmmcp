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
	"errors"
	"testing"
)

// TestLoopbackDialControlAllow verifies the allow=true fast path (used by
// AllowNonLoopback) accepts any address without inspecting it.
func TestLoopbackDialControlAllow(t *testing.T) {
	t.Parallel()
	control := loopbackDialControl(true)
	if err := control("tcp", "93.184.216.34:80", nil); err != nil {
		t.Fatalf("allow=true: unexpected error: %v", err)
	}
}

// TestLoopbackDialControlNoPortFallback exercises the net.SplitHostPort
// error fallback (address has no port), for an address that is loopback.
func TestLoopbackDialControlNoPortFallback(t *testing.T) {
	t.Parallel()
	control := loopbackDialControl(false)
	if err := control("tcp", "127.0.0.1", nil); err != nil {
		t.Fatalf("bare loopback address should be allowed: %v", err)
	}
}

// TestLoopbackDialControlRejectsNonLoopback exercises the reject branch for a
// well-formed host:port address whose host is not loopback.
func TestLoopbackDialControlRejectsNonLoopback(t *testing.T) {
	t.Parallel()
	control := loopbackDialControl(false)
	err := control("tcp", "93.184.216.34:80", nil)
	if !errors.Is(err, ErrSSRF) {
		t.Fatalf("err = %v, want ErrSSRF", err)
	}
}

// TestLoopbackDialControlRejectsUnparsableHost exercises the reject branch
// when the (port-stripped) host is not a valid IP at all.
func TestLoopbackDialControlRejectsUnparsableHost(t *testing.T) {
	t.Parallel()
	control := loopbackDialControl(false)
	err := control("tcp", "not-an-ip", nil)
	if !errors.Is(err, ErrSSRF) {
		t.Fatalf("err = %v, want ErrSSRF", err)
	}
}
