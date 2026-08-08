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

package drivers

// ChooseEngine exposes chooseEngine for tests: drivers_test.go (package
// drivers_test) can inject engine/fake.Engine candidates to exercise every
// selection branch (first available, first unavailable + second available,
// none available) without depending on which container binaries happen to
// be installed on the host running the tests. Test-only: this file is
// compiled solely when running `go test` (export_test.go idiom).
var ChooseEngine = chooseEngine
