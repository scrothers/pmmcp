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

// Package engine defines the container Engine interface and shared CLI runner types.
// It imports no concrete engines. Podman and Docker drivers live in subpackages; selection is in
// engine/drivers. Fake is a hermetic test double. Run/Stop/Logs are argv-safe (no shell). When a
// binary is missing, drivers return ErrUnavailable.
package engine
