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

package service_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/scrothers/pmmcp/internal/service"
)

func TestInstallUninstallCurrentOS(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("LOCALAPPDATA", filepath.Join(home, "LocalAppData"))

	ctx := context.Background()
	pmmcpd := filepath.Join(home, "bin", "pmmcpd")
	if err := service.Install(ctx, pmmcpd); err != nil {
		t.Fatalf("Install: %v", err)
	}

	var path string
	switch runtime.GOOS {
	case "linux":
		path = filepath.Join(home, ".config", "systemd", "user", "pmmcpd.service")
	case "darwin":
		path = filepath.Join(home, "Library", "LaunchAgents", "com.scrothers.pmmcpd.plist")
	case "windows":
		path = filepath.Join(home, "LocalAppData", "pmmcp", "pmmcpd-start.bat")
	default:
		t.Skipf("unsupported GOOS %s", runtime.GOOS)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected install artifact at %s: %v", path, err)
	}

	if err := service.Uninstall(ctx); err != nil {
		t.Fatalf("Uninstall: %v", err)
	}
}

// setHome sets HOME and USERPROFILE to dir so os.UserHomeDir resolves dir
// consistently regardless of which GOOS this test binary actually runs on
// (the linux/darwin backends read HOME; only USERPROFILE matters on
// windows, so both must be set to exercise a non-native GOOS arm).
func setHome(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
}

// TestInstallForEveryOS exercises every OS dispatch arm of the goos seam,
// regardless of which OS runs this test binary.
func TestInstallForEveryOS(t *testing.T) {
	tests := []struct {
		goos     string
		artifact func(home string) string
	}{
		{"linux", func(home string) string {
			return filepath.Join(home, ".config", "systemd", "user", "pmmcpd.service")
		}},
		{"darwin", func(home string) string {
			return filepath.Join(home, "Library", "LaunchAgents", "com.scrothers.pmmcpd.plist")
		}},
		{"windows", func(home string) string {
			return filepath.Join(home, "LocalAppData", "pmmcp", "pmmcpd-start.bat")
		}},
	}
	for _, tt := range tests {
		t.Run(tt.goos, func(t *testing.T) {
			home := t.TempDir()
			setHome(t, home)
			t.Setenv("LOCALAPPDATA", filepath.Join(home, "LocalAppData"))

			ctx := context.Background()
			pmmcpd := filepath.Join(home, "bin", "pmmcpd")
			if err := service.InstallFor(ctx, pmmcpd, tt.goos); err != nil {
				t.Fatalf("InstallFor(%s): %v", tt.goos, err)
			}
			artifact := tt.artifact(home)
			if _, err := os.Stat(artifact); err != nil {
				t.Fatalf("expected install artifact at %s: %v", artifact, err)
			}
			if err := service.UninstallFor(ctx, tt.goos); err != nil {
				t.Fatalf("UninstallFor(%s): %v", tt.goos, err)
			}
			if _, err := os.Stat(artifact); !os.IsNotExist(err) {
				t.Fatalf("artifact still present after uninstall: %v", err)
			}
		})
	}
}

// TestInstallForUnsupportedOS covers the default arm of both dispatch switches.
func TestInstallForUnsupportedOS(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	ctx := context.Background()

	err := service.InstallFor(ctx, "/usr/local/bin/pmmcpd", "plan9")
	if err == nil {
		t.Fatal("InstallFor: want error for unsupported OS")
	}
	if !strings.Contains(err.Error(), "plan9") {
		t.Fatalf("InstallFor error = %v, want it to mention the unsupported GOOS", err)
	}

	err = service.UninstallFor(ctx, "plan9")
	if err == nil {
		t.Fatal("UninstallFor: want error for unsupported OS")
	}
	if !strings.Contains(err.Error(), "plan9") {
		t.Fatalf("UninstallFor error = %v, want it to mention the unsupported GOOS", err)
	}
}

// TestInstallUnsupportedOSViaPublicAPI ensures the public Install/Uninstall
// functions still surface the unsupported-OS error when the host itself is
// unsupported; on a supported host (linux/darwin/windows) this is a no-op
// beyond the runtime.GOOS sanity check already exercised above.
func TestInstallUnsupportedOSViaPublicAPI(t *testing.T) {
	switch runtime.GOOS {
	case "linux", "darwin", "windows":
		t.Skip("host OS is supported; unsupported-OS arm is covered via InstallFor/UninstallFor")
	}
	t.Setenv("HOME", t.TempDir())
	ctx := context.Background()
	if err := service.Install(ctx, "/usr/local/bin/pmmcpd"); err == nil {
		t.Fatal("Install: want error on unsupported host OS")
	}
	if err := service.Uninstall(ctx); err == nil {
		t.Fatal("Uninstall: want error on unsupported host OS")
	}
}

// TestErrorsAreNotWrapped documents that the unsupported-OS errors are plain
// (no %w), matching the sentinel-free design of this dispatch layer.
func TestErrorsAreNotWrapped(t *testing.T) {
	err := service.InstallFor(context.Background(), "/x", "plan9")
	if errors.Unwrap(err) != nil {
		t.Fatalf("unexpected wrapped error: %v", errors.Unwrap(err))
	}
}
