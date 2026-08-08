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

// Package daemon is the pmmcpd control plane: store, process manager, IPC/gRPC, authz, and handlers.
// Server is the sole long-lived parent of managed processes. ListenAndServe opens the private
// endpoint (UDS or named pipe), enforces same-UID peer credentials, serves unary Call plus log/event
// streams, and runs supervision, watch, and webhook loops until cancel.
//
// Clients never own PIDs. Missing isolation for strict/standard fails closed. Secrets resolve into
// child env at start only; status and audit paths must not leak values.
package daemon
