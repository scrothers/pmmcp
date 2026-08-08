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

package watch

import "os"

// SetDirentInfoForTest overrides the per-entry stat hook used by
// snapshotDir and returns a function that restores the previous hook. It
// exists so black-box tests can deterministically exercise the
// entry-vanished-mid-scan branch without racing a real filesystem delete.
func SetDirentInfoForTest(f func(de os.DirEntry) (os.FileInfo, error)) (restore func()) {
	prev := direntInfo
	direntInfo = f
	return func() { direntInfo = prev }
}
