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

// Package secret loads env-files, resolves secret:// references, redacts sensitive material, and
// implements a file-backed keyring stand-in.
//
// URI schemes: keyring, sops, file, and env. Resolve expands refs at process start into the child
// environment only; Check validates without returning values. File and sops paths are contained to
// the project root (symlink-aware, fail-closed). SOPS decrypt uses github.com/getsops/sops/v3.
//
// Redaction scrubs key-name matches, registered values, and common secret shapes with stable
// ***REDACTED:NAME*** placeholders. FileBackend stores 0600 files under a 0700 directory.
package secret
