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

// Package main is the pmmcpd daemon binary.
// It wires a signal-canceled context and delegates to internal/daemoncmd (and through it
// internal/daemon) to load config and serve the private control plane. This package owns
// only process entry and graceful shutdown signaling. All process ownership lives in the daemon.
package main
