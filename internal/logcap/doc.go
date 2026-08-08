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

// Package logcap captures, rotates, redacts, tails, greps, and exports process stdout/stderr.
// Capturer owns a per-process log directory (0700) with fixed filenames stdout.log and stderr.log.
// Rotation gzip-compresses archives under size and retention limits. Write paths run secret.RedactLine
// before disk. Read helpers enforce an output byte cap. ExportTarGz archives active logs for ship.
package logcap
