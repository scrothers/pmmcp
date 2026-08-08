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

// Package ipc implements private gRPC transport over Unix domain sockets and Windows named pipes.
// Listen creates the endpoint (UDS mode 0600 or owner-only pipe) and rejects other UIDs on Accept
// where peer credentials are available. Dial and Client perform a version handshake and fail closed
// on major skew. This package does not authorize roles; the daemon applies authz after dial.
package ipc
