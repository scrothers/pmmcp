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

package local

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// jobHandle holds an open Windows Job Object for tree kill / kill-on-close.
type jobHandle struct {
	h windows.Handle
}

// Win32 Job Object constants not exported by x/sys/windows.
const (
	// JOB_OBJECT_LIMIT_DIE_ON_UNHANDLED_EXCEPTION: a crash in the child does not
	// pop a Windows Error Reporting dialog that would keep the job alive.
	jobLimitDieOnUnhandledException = 0x00000400

	// JOBOBJECTINFOCLASS: JobObjectBasicUIRestrictions.
	jobObjectBasicUIRestrictions uint32 = 4

	// JOB_OBJECT_UILIMIT_* flags. HANDLES (0x1) is deliberately omitted: it
	// blocks the child from using USER handles it inherited, which can break
	// otherwise-legitimate tools. The rest isolate the child from the user's
	// desktop, clipboard, atom table, display/system settings, and deny it the
	// ability to log the user off or shut the machine down.
	uiLimitReadClipboard    = 0x00000002
	uiLimitWriteClipboard   = 0x00000004
	uiLimitSystemParameters = 0x00000008
	uiLimitDisplaySettings  = 0x00000010
	uiLimitGlobalAtoms      = 0x00000020
	uiLimitDesktop          = 0x00000040
	uiLimitExitWindows      = 0x00000080

	uiLimitsSandboxed = uiLimitReadClipboard | uiLimitWriteClipboard |
		uiLimitSystemParameters | uiLimitDisplaySettings | uiLimitGlobalAtoms |
		uiLimitDesktop | uiLimitExitWindows
)

// jobBasicUIRestrictions mirrors JOBOBJECT_BASIC_UI_RESTRICTIONS (a single DWORD).
type jobBasicUIRestrictions struct {
	UIRestrictionsClass uint32
}

// setSysProcAttr places the child in a new process group for console control.
func setSysProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}
}

// assignJob creates a hardened Job Object and assigns pid to it. The job always
// carries KILL_ON_JOB_CLOSE (tree-kill on daemon exit) and
// DIE_ON_UNHANDLED_EXCEPTION (no WER dialog keeps a crashed child alive). For the
// restrictive profiles (strict/standard) it additionally applies UI restrictions
// that isolate the child from the user's desktop, clipboard, and session.
func assignJob(pid int, profile string) (*jobHandle, error) {
	if pid <= 0 {
		return nil, fmt.Errorf("local: invalid pid %d", pid)
	}
	h, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, fmt.Errorf("local: CreateJobObject: %w", err)
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE |
		jobLimitDieOnUnhandledException
	if _, err := windows.SetInformationJobObject(
		h,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	); err != nil {
		_ = windows.CloseHandle(h)
		return nil, fmt.Errorf("local: SetInformationJobObject: %w", err)
	}
	// UI restrictions for sandboxed profiles. Best-effort: on a host that refuses
	// them the child still runs tree-killable under the job, so standard stays
	// best-effort per its contract rather than regressing.
	if p := strings.ToLower(strings.TrimSpace(profile)); p == "strict" || p == "standard" {
		ui := jobBasicUIRestrictions{UIRestrictionsClass: uiLimitsSandboxed}
		_, _ = windows.SetInformationJobObject(
			h,
			jobObjectBasicUIRestrictions,
			uintptr(unsafe.Pointer(&ui)),
			uint32(unsafe.Sizeof(ui)),
		)
	}
	ph, err := windows.OpenProcess(
		windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE|windows.PROCESS_DUP_HANDLE|windows.PROCESS_QUERY_INFORMATION,
		false,
		uint32(pid),
	)
	if err != nil {
		_ = windows.CloseHandle(h)
		return nil, fmt.Errorf("local: OpenProcess: %w", err)
	}
	defer windows.CloseHandle(ph)
	if err := windows.AssignProcessToJobObject(h, ph); err != nil {
		_ = windows.CloseHandle(h)
		return nil, fmt.Errorf("local: AssignProcessToJobObject: %w", err)
	}
	return &jobHandle{h: h}, nil
}

func (j *jobHandle) close() {
	if j == nil || j.h == 0 {
		return
	}
	_ = windows.CloseHandle(j.h)
	j.h = 0
}

func (j *jobHandle) terminate() error {
	if j == nil || j.h == 0 {
		return fmt.Errorf("local: nil job")
	}
	if err := windows.TerminateJobObject(j.h, 1); err != nil {
		return fmt.Errorf("local: TerminateJobObject: %w", err)
	}
	return nil
}

// killTree terminates the root process (job path preferred via forceKill).
func killTree(pid int, sig os.Signal) error {
	if pid <= 0 {
		return fmt.Errorf("local: invalid pid %d", pid)
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return p.Kill()
}
