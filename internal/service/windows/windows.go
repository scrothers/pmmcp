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

package windows

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DirName is the subdirectory under LocalAppData for install artifacts.
const DirName = "pmmcp"

// BatName is the helper batch script that starts pmmcpd.
const BatName = "pmmcpd-start.bat"

// TaskXMLName is a sample Task Scheduler XML note for logon start.
const TaskXMLName = "pmmcpd-logon-task.xml"

// ReadmeName documents elevation and manual registration steps.
const ReadmeName = "INSTALL.txt"

// Install writes a non-admin-friendly start script, sample task XML, and a
// short INSTALL.txt under %LOCALAPPDATA%\pmmcp\ (or HOME/AppData/Local/pmmcp).
//
// It does not register a Windows Service (that often needs elevation). The
// preferred path is a per-user logon Scheduled Task pointing at the .bat.
func Install(ctx context.Context, pmmcpdPath string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("service/windows: install: %w", err)
	}
	if pmmcpdPath == "" {
		return fmt.Errorf("service/windows: install: pmmcpd path required")
	}
	if hasControlChars(pmmcpdPath) {
		return fmt.Errorf("service/windows: install: pmmcpd path contains control characters")
	}
	if strings.Contains(pmmcpdPath, `"`) {
		return fmt.Errorf("service/windows: install: pmmcpd path must not contain a quote")
	}
	dir, err := installDir()
	if err != nil {
		return fmt.Errorf("service/windows: install: %w", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("service/windows: install: mkdir: %w", err)
	}

	bat := filepath.Join(dir, BatName)
	// Double any % so cmd.exe does not treat a path segment as %VAR% expansion.
	batBody := fmt.Sprintf("@echo off\r\n\"%s\" run\r\n", strings.ReplaceAll(pmmcpdPath, "%", "%%"))
	if err := os.WriteFile(bat, []byte(batBody), 0o644); err != nil {
		return fmt.Errorf("service/windows: install: bat: %w", err)
	}

	// WorkingDirectory for the task action. Best-effort: fall back to the install
	// dir so a missing HOME never fails the install (installDir already handled
	// the LOCALAPPDATA→HOME fallback above, so this adds no new HOME dependency).
	workDir, herr := os.UserHomeDir()
	if herr != nil || workDir == "" {
		workDir = dir
	}
	xmlPath := filepath.Join(dir, TaskXMLName)
	xmlBody := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!--
  Task Scheduler definition for pmmcpd (per-user logon daemon).
  Import with the Task Scheduler GUI, or:
    schtasks /Create /TN "pmmcpd" /XML "(this file)"

  Elevation: not required for a per-user logon task. A system-wide service
  would need administrator rights and is not the default path.
-->
<Task version="1.2" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <RegistrationInfo>
    <Description>pmmcp daemon (agent-native process manager) for the current user.</Description>
    <URI>\pmmcpd</URI>
  </RegistrationInfo>
  <Triggers>
    <LogonTrigger>%s
      <Enabled>true</Enabled>
    </LogonTrigger>
  </Triggers>
  <Principals>
    <Principal id="Author">%s
      <LogonType>InteractiveToken</LogonType>
      <RunLevel>LeastPrivilege</RunLevel>
    </Principal>
  </Principals>
  <Actions Context="Author">
    <Exec>
      <Command>%s</Command>
      <Arguments>run</Arguments>
      <WorkingDirectory>%s</WorkingDirectory>
    </Exec>
  </Actions>
  <Settings>
    <MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>
    <DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>
    <StopIfGoingOnBatteries>false</StopIfGoingOnBatteries>
    <StartWhenAvailable>true</StartWhenAvailable>
    <AllowHardTerminate>true</AllowHardTerminate>
    <RunOnlyIfNetworkAvailable>false</RunOnlyIfNetworkAvailable>
    <IdleSettings>
      <StopOnIdleEnd>false</StopOnIdleEnd>
    </IdleSettings>
    <RestartOnFailure>
      <Interval>PT1M</Interval>
      <Count>3</Count>
    </RestartOnFailure>
    <ExecutionTimeLimit>PT0S</ExecutionTimeLimit>
    <Enabled>true</Enabled>
  </Settings>
</Task>
`, logonUserElem(), principalUserElem(), xmlEscape(pmmcpdPath), xmlEscape(workDir))
	if err := os.WriteFile(xmlPath, []byte(xmlBody), 0o644); err != nil {
		return fmt.Errorf("service/windows: install: xml: %w", err)
	}

	readme := filepath.Join(dir, ReadmeName)
	readmeBody := fmt.Sprintf(`pmmcpd Windows install artifacts
================================

Files in this directory:
  %s  — start pmmcpd in the foreground
  %s  — sample Task Scheduler XML (logon)

Recommended (no elevation):
  1. Open Task Scheduler
  2. Create Task -> Trigger: At log on (your user)
  3. Action: Start a program -> %s
  Or: schtasks /Create /TN pmmcpd /XML "%s"

Daemon binary used: %s

A Windows Service (services.msc) typically requires Administrator elevation
and is not the default pmmcp install path.
`, BatName, TaskXMLName, bat, xmlPath, pmmcpdPath)
	if err := os.WriteFile(readme, []byte(readmeBody), 0o644); err != nil {
		return fmt.Errorf("service/windows: install: readme: %w", err)
	}
	return nil
}

// Uninstall removes install artifacts under LocalAppData\pmmcp.
func Uninstall(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("service/windows: uninstall: %w", err)
	}
	dir, err := installDir()
	if err != nil {
		return fmt.Errorf("service/windows: uninstall: %w", err)
	}
	for _, name := range []string{BatName, TaskXMLName, ReadmeName} {
		path := filepath.Join(dir, name)
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("service/windows: uninstall: %w", err)
		}
	}
	// Best-effort remove empty dir.
	_ = os.Remove(dir)
	return nil
}

// InstallDir returns the artifact directory path.
func InstallDir() (string, error) {
	return installDir()
}

func installDir() (string, error) {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("home: %w", err)
		}
		base = filepath.Join(home, "AppData", "Local")
	}
	return filepath.Join(base, DirName), nil
}

// currentUser returns the invoking user's name from the environment, or "" if
// it is unset or contains control characters. Used to pin the generated task to
// the current user; when empty, the XML omits UserId and schtasks binds the
// task to whoever imports it.
func currentUser() string {
	for _, k := range []string{"USERNAME", "USER"} {
		if v := os.Getenv(k); v != "" && !hasControlChars(v) {
			return v
		}
	}
	return ""
}

// logonUserElem returns an indented <UserId> element for the LogonTrigger, or ""
// when the user can't be determined.
func logonUserElem() string {
	u := currentUser()
	if u == "" {
		return ""
	}
	return "\n      <UserId>" + xmlEscape(u) + "</UserId>"
}

// principalUserElem returns an indented <UserId> element for the Principal, or
// "" when the user can't be determined.
func principalUserElem() string {
	u := currentUser()
	if u == "" {
		return ""
	}
	return "\n      <UserId>" + xmlEscape(u) + "</UserId>"
}

// xmlEscape escapes a string for inclusion in XML character data.
func xmlEscape(s string) string {
	var buf bytes.Buffer
	if err := xml.EscapeText(&buf, []byte(s)); err != nil {
		return ""
	}
	return buf.String()
}

// hasControlChars reports whether s contains any ASCII control character.
func hasControlChars(s string) bool {
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}
