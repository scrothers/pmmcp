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

//go:build windows

package daemon

import (
	"errors"

	"golang.org/x/sys/windows"
)

// pidAlive reports whether an OS process with pid is currently alive. On
// Windows, os.Process.Signal implements only Kill/Interrupt — a nil-signal
// probe always returns "unsupported", which made the generic fallback report
// every process as dead. Query the process handle instead: open with the
// narrowest right and check the exit code for STILL_ACTIVE.
func pidAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		// Access denied still proves the process exists (alive, other owner) —
		// mirroring the EPERM case in the unix implementation.
		return errors.Is(err, windows.ERROR_ACCESS_DENIED)
	}
	defer func() { _ = windows.CloseHandle(h) }()
	var code uint32
	if err := windows.GetExitCodeProcess(h, &code); err != nil {
		return false
	}
	// STILL_ACTIVE (259): the process has not exited.
	const stillActive = 259
	return code == stillActive
}
