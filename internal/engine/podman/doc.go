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

// Package podman implements engine.Engine using the Podman CLI.
// Commands are built as argv slices (no shell). Available uses exec.LookPath("podman"). When the
// binary is missing, Run, Stop, and Logs return engine.ErrUnavailable. Operators typically expose
// a rootless API socket under $XDG_RUNTIME_DIR/podman; this package uses the CLI rather than dialing
// that socket directly.
package podman
