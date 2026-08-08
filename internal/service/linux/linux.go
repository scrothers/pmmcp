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

package linux

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// UnitName is the systemd --user unit filename.
const UnitName = "pmmcpd.service"

// Install writes a systemd --user unit for pmmcpd under $HOME/.config/systemd/user.
// It does not run systemctl; the caller/docs tell the operator to enable the unit.
func Install(ctx context.Context, pmmcpdPath string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("service/linux: install: %w", err)
	}
	if pmmcpdPath == "" {
		return fmt.Errorf("service/linux: install: pmmcpd path required")
	}
	if hasControlChars(pmmcpdPath) {
		return fmt.Errorf("service/linux: install: pmmcpd path contains control characters")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("service/linux: install: home: %w", err)
	}
	dir := filepath.Join(home, ".config", "systemd", "user")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("service/linux: install: mkdir: %w", err)
	}
	unit := filepath.Join(dir, UnitName)
	body := fmt.Sprintf(`[Unit]
Description=pmmcp process manager daemon
After=default.target

[Service]
Type=simple
ExecStart=%s run
Restart=on-failure

[Install]
WantedBy=default.target
`, quoteSystemd(pmmcpdPath))
	if err := os.WriteFile(unit, []byte(body), 0o644); err != nil {
		return fmt.Errorf("service/linux: install: write: %w", err)
	}
	return nil
}

// Uninstall removes the systemd --user unit file if present.
func Uninstall(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("service/linux: uninstall: %w", err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("service/linux: uninstall: home: %w", err)
	}
	unit := filepath.Join(home, ".config", "systemd", "user", UnitName)
	if err := os.Remove(unit); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("service/linux: uninstall: %w", err)
	}
	return nil
}

// quoteSystemd renders a path as a single systemd double-quoted token. Backslash
// and quote are C-escaped; a literal % is doubled so systemd does not treat it
// as a specifier. This keeps paths with spaces or % as one ExecStart argument.
func quoteSystemd(p string) string {
	esc := strings.NewReplacer(`\`, `\\`, `"`, `\"`, `%`, `%%`)
	return `"` + esc.Replace(p) + `"`
}

// hasControlChars reports whether s contains any ASCII control character
// (including newline), which could inject unit directives.
func hasControlChars(s string) bool {
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}

// UnitPath returns the path where the unit would be installed for the current user.
func UnitPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "systemd", "user", UnitName), nil
}
