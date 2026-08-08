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
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

// setSysProcAttr places the child in a new process group so tree kill works.
func setSysProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killTree signals the entire process group (negative PID).
func killTree(pid int, sig os.Signal) error {
	if pid <= 0 {
		return fmt.Errorf("local: invalid pid %d", pid)
	}
	s, ok := sig.(syscall.Signal)
	if !ok {
		// Fall back to single-process kill via os.FindProcess.
		p, err := os.FindProcess(pid)
		if err != nil {
			return err
		}
		return p.Signal(sig)
	}
	// Negative PID = process group on Unix.
	if err := syscall.Kill(-pid, s); err != nil {
		return err
	}
	return nil
}
