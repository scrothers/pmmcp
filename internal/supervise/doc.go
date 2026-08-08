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

// Package supervise implements restart policy math, health probing, and boot relaunch eligibility.
// ShouldRestart and NextBackoff encode retry budgets and backoff. ProbeHTTP performs loopback-bound
// HTTP checks unless non-loopback is explicitly allowed. EligibleForRelaunch gates boot restart on
// desired state running. The daemon owns the live loops; this package stays free of process.Manager
// I/O where possible.
package supervise
