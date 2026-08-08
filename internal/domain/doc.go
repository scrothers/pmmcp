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

// Package domain holds pure process-manager value types, validation, status enums, and error codes.
// It must not import OS, network, filesystem, or SQL packages. Process.Command is argv only
// (ValidateCommand rejects empty lists and empty elements). Status and Desired are closed enums.
// Error and ExitCode map machine codes to CLI exit statuses. IDs are plain strings here; generation
// lives in package id.
package domain
