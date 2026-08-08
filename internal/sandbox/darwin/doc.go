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

// Package darwin applies sandbox profiles on macOS.
// Apply validates profile and project root (strict fails closed without a root). Seatbelt profiles
// deny secret path classes when sandbox-exec is available; the local driver wraps children accordingly.
// Missing sandbox-exec fails closed for restrictive profiles.
package darwin
