// Copyright 2026 Steven Crothers
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package darwin

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
)

// Label is the LaunchAgent label / plist basename without extension —
// reverse-DNS of the maintainer's domain (scrothers.com).
const Label = "com.scrothers.pmmcpd"

// PlistName is the LaunchAgent plist filename.
const PlistName = Label + ".plist"

// Install writes a LaunchAgent plist under ~/Library/LaunchAgents and creates
// the log directory the agent writes to. It does not run launchctl load — the
// operator enables the agent deliberately (no auto-start).
//
// The generated agent:
// - runs `pmmcpd run` at load and keeps it alive across crashes, but not
// across a clean (exit-0) shutdown — matching the Linux unit's
// Restart=on-failure so `launchctl kickstart -k`/a clean stop is honored;
// - throttles relaunch so a crash-looping daemon does not hammer the system;
// - runs as a Background process type;
// - captures stdout/stderr to ~/Library/Logs/pmmcp so a daemon that fails to
// start leaves a diagnosable trail instead of vanishing.
func Install(ctx context.Context, pmmcpdPath string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("service/darwin: install: %w", err)
	}
	if pmmcpdPath == "" {
		return fmt.Errorf("service/darwin: install: pmmcpd path required")
	}
	if hasControlChars(pmmcpdPath) {
		return fmt.Errorf("service/darwin: install: pmmcpd path contains control characters")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("service/darwin: install: home: %w", err)
	}
	dir := filepath.Join(home, "Library", "LaunchAgents")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("service/darwin: install: mkdir: %w", err)
	}
	logDir := logDirFor(home)
	if err := os.MkdirAll(logDir, 0o700); err != nil {
		return fmt.Errorf("service/darwin: install: log dir: %w", err)
	}
	plist := filepath.Join(dir, PlistName)
	body := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>Label</key><string>%s</string>
  <key>ProgramArguments</key><array><string>%s</string><string>run</string></array>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><dict><key>SuccessfulExit</key><false/></dict>
  <key>ThrottleInterval</key><integer>10</integer>
  <key>ProcessType</key><string>Background</string>
  <key>WorkingDirectory</key><string>%s</string>
  <key>StandardOutPath</key><string>%s</string>
  <key>StandardErrorPath</key><string>%s</string>
</dict></plist>
`,
		Label,
		xmlEscape(pmmcpdPath),
		xmlEscape(home),
		xmlEscape(filepath.Join(logDir, "pmmcpd.out.log")),
		xmlEscape(filepath.Join(logDir, "pmmcpd.err.log")),
	)
	if err := os.WriteFile(plist, []byte(body), 0o644); err != nil {
		return fmt.Errorf("service/darwin: install: write: %w", err)
	}
	return nil
}

// logDirFor returns the LaunchAgent log directory for a home dir.
func logDirFor(home string) string {
	return filepath.Join(home, "Library", "Logs", "pmmcp")
}

// Uninstall removes the LaunchAgent plist if present.
func Uninstall(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("service/darwin: uninstall: %w", err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("service/darwin: uninstall: home: %w", err)
	}
	dir := filepath.Join(home, "Library", "LaunchAgents")
	if err := os.Remove(filepath.Join(dir, PlistName)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("service/darwin: uninstall: %w", err)
	}
	return nil
}

// PlistPath returns the path where the plist would be installed.
func PlistPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", PlistName), nil
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
