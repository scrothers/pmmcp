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

// Package config loads daemon and client configuration once at process start.
// Load merges flag, environment, optional config file, and platform defaults (Linux XDG, macOS
// Application Support, Windows LocalAppData and named pipes). Empty path fields resolve to those
// defaults. The default sandbox posture is strict. Doctor and String helpers redact token paths.
//
// Callers must not scatter os.Getenv; inject LookupEnv via LoadOptions in tests.
package config
