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

//go:build unix

package local

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/unix"
)

// applyMemoryLimit is a no-op pre-start; limit applied post-start via prlimit on Linux.
func applyMemoryLimit(cmd *exec.Cmd, bytes uint64) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
	_ = bytes
}

func applyMemoryLimitPID(pid int, bytes uint64) error {
	if pid <= 0 || bytes == 0 {
		return nil
	}
	lim := unix.Rlimit{Cur: bytes, Max: bytes}
	return setRlimitPID(pid, &lim)
}
