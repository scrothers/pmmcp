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

package service

import (
	"context"
	"fmt"
	"runtime"

	"github.com/scrothers/pmmcp/internal/service/darwin"
	"github.com/scrothers/pmmcp/internal/service/linux"
	"github.com/scrothers/pmmcp/internal/service/windows"
)

// Install writes the user-level service definition for the current OS.
// pmmcpdPath is the absolute (or resolvable) path to the daemon binary.
func Install(ctx context.Context, pmmcpdPath string) error {
	return installFor(ctx, pmmcpdPath, runtime.GOOS)
}

// installFor implements Install for an explicit goos value. It is the seam
// tests use to exercise every OS dispatch arm (including the unsupported-OS
// error) regardless of which platform runs the test binary; Install always
// calls it with runtime.GOOS.
func installFor(ctx context.Context, pmmcpdPath, goos string) error {
	switch goos {
	case "linux":
		return linux.Install(ctx, pmmcpdPath)
	case "darwin":
		return darwin.Install(ctx, pmmcpdPath)
	case "windows":
		return windows.Install(ctx, pmmcpdPath)
	default:
		return fmt.Errorf("service: install: unsupported OS %s (start manually: pmmcpd run)", goos)
	}
}

// Uninstall removes the user-level service definition for the current OS.
func Uninstall(ctx context.Context) error {
	return uninstallFor(ctx, runtime.GOOS)
}

// uninstallFor implements Uninstall for an explicit goos value; see installFor.
func uninstallFor(ctx context.Context, goos string) error {
	switch goos {
	case "linux":
		return linux.Uninstall(ctx)
	case "darwin":
		return darwin.Uninstall(ctx)
	case "windows":
		return windows.Uninstall(ctx)
	default:
		return fmt.Errorf("service: uninstall: unsupported OS %s", goos)
	}
}
