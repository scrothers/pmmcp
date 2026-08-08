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

package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/scrothers/pmmcp/internal/domain"
	"github.com/scrothers/pmmcp/internal/service"
)

// installService writes the user-level service definition. The daemon binary is
// resolved to an absolute path so systemd/launchd/Task Scheduler can execute it
// (a bare name only resolves against the service manager's compiled-in PATH,
// which excludes ~/.local/bin). A non-empty override (from --bin) wins.
func installService(ctx context.Context, override string) error {
	pmmcpd, err := resolveDaemonPath(override)
	if err != nil {
		return err
	}
	if err := service.Install(ctx, pmmcpd); err != nil {
		return err
	}
	fmt.Println("installed user service definition for", pmmcpd)
	fmt.Println("enable/start per platform docs (e.g. systemctl --user enable --now pmmcpd.service)")
	return nil
}

// resolveDaemonPath returns an absolute pmmcpd path: the --bin override, a
// pmmcpd sibling of the pmmcp executable, or one found on PATH. It errors rather
// than emitting a bare, unresolvable name.
func resolveDaemonPath(override string) (string, error) {
	if override != "" {
		abs, err := filepath.Abs(override)
		if err != nil {
			return "", domain.WrapError(domain.CodeInvalidArgument, "resolve --bin path", false, err)
		}
		return abs, nil
	}
	if exe, err := os.Executable(); err == nil {
		sibling := filepath.Join(filepath.Dir(exe), "pmmcpd")
		if _, err := os.Stat(sibling); err == nil {
			if abs, err := filepath.Abs(sibling); err == nil {
				return abs, nil
			}
			return sibling, nil
		}
	}
	if found, err := exec.LookPath("pmmcpd"); err == nil {
		if abs, err := filepath.Abs(found); err == nil {
			return abs, nil
		}
		return found, nil
	}
	return "", domain.NewError(domain.CodeNotFound,
		"pmmcpd not found next to pmmcp or on PATH; pass --bin /path/to/pmmcpd", false)
}

func uninstallService(ctx context.Context) error {
	if err := service.Uninstall(ctx); err != nil {
		return err
	}
	fmt.Println("removed user service definition")
	return nil
}
