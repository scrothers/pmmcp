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

// Package docker implements engine.Engine using the Docker CLI.
// Commands are built as argv slices (no shell). Available uses exec.LookPath("docker"). When the
// binary is missing, Run, Stop, and Logs return engine.ErrUnavailable. This package does not dial
// the Docker API socket.
package docker
