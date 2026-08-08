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

package linux_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scrothers/pmmcp/internal/service/linux"
)

// setHome sets HOME and USERPROFILE to dir, and unsetHome clears both — the
// linux driver calls os.UserHomeDir, which on windows consults USERPROFILE
// instead of HOME, so tests exercising the home-resolution branches must
// control both env vars to behave consistently across CI platforms.
func setHome(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
}

func unsetHome(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")
}

func TestInstallUninstall(t *testing.T) {
	// Mutates HOME — not parallel.
	home := t.TempDir()
	setHome(t, home)

	ctx := context.Background()
	pmmcpd := "/usr/local/bin/pmmcpd"
	if err := linux.Install(ctx, pmmcpd); err != nil {
		t.Fatalf("Install: %v", err)
	}
	unit := filepath.Join(home, ".config", "systemd", "user", linux.UnitName)
	data, err := os.ReadFile(unit)
	if err != nil {
		t.Fatalf("read unit: %v", err)
	}
	body := string(data)
	if !strings.Contains(body, `ExecStart="`+pmmcpd+`" run`) {
		t.Fatalf("unit missing quoted ExecStart:\n%s", body)
	}
	if !strings.Contains(body, "WantedBy=default.target") {
		t.Fatalf("unit missing Install section:\n%s", body)
	}

	if err := linux.Uninstall(ctx); err != nil {
		t.Fatalf("Uninstall: %v", err)
	}
	if _, err := os.Stat(unit); !os.IsNotExist(err) {
		t.Fatalf("unit still present after uninstall: %v", err)
	}
	// Second uninstall is idempotent.
	if err := linux.Uninstall(ctx); err != nil {
		t.Fatalf("Uninstall again: %v", err)
	}
}

func TestInstallEmptyPath(t *testing.T) {
	setHome(t, t.TempDir())
	err := linux.Install(context.Background(), "")
	if err == nil {
		t.Fatal("want error for empty path")
	}
}

func TestInstallPathWithSpacesAndPercent(t *testing.T) {
	home := t.TempDir()
	setHome(t, home)
	pmmcpd := "/home/u/My Tools/100%pmmcpd"
	if err := linux.Install(context.Background(), pmmcpd); err != nil {
		t.Fatalf("Install: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(home, ".config", "systemd", "user", linux.UnitName))
	if err != nil {
		t.Fatalf("read unit: %v", err)
	}
	body := string(data)
	// The path is a single quoted token and % is doubled to escape the specifier.
	if !strings.Contains(body, `ExecStart="/home/u/My Tools/100%%pmmcpd" run`) {
		t.Fatalf("unit did not quote/escape path:\n%s", body)
	}
}

func TestUnitPath(t *testing.T) {
	home := t.TempDir()
	setHome(t, home)
	p, err := linux.UnitPath()
	if err != nil {
		t.Fatalf("UnitPath: %v", err)
	}
	want := filepath.Join(home, ".config", "systemd", "user", linux.UnitName)
	if p != want {
		t.Fatalf("UnitPath = %q, want %q", p, want)
	}
}

func TestInstallContextCanceled(t *testing.T) {
	setHome(t, t.TempDir())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := linux.Install(ctx, "/usr/local/bin/pmmcpd"); err == nil {
		t.Fatal("want error for canceled context")
	}
	if err := linux.Uninstall(ctx); err == nil {
		t.Fatal("want error for canceled context on uninstall")
	}
}

func TestInstallHomeDirError(t *testing.T) {
	unsetHome(t)
	if err := linux.Install(context.Background(), "/usr/local/bin/pmmcpd"); err == nil {
		t.Fatal("want error when $HOME is unset")
	}
}

func TestUninstallHomeDirError(t *testing.T) {
	unsetHome(t)
	if err := linux.Uninstall(context.Background()); err == nil {
		t.Fatal("want error when $HOME is unset")
	}
}

func TestUnitPathHomeDirError(t *testing.T) {
	unsetHome(t)
	if _, err := linux.UnitPath(); err == nil {
		t.Fatal("want error when $HOME is unset")
	}
}

func TestInstallMkdirError(t *testing.T) {
	home := t.TempDir()
	setHome(t, home)
	// A regular file where .config should be a directory forces MkdirAll
	// to fail with "not a directory".
	if err := os.WriteFile(filepath.Join(home, ".config"), []byte("not a dir"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := linux.Install(context.Background(), "/usr/local/bin/pmmcpd"); err == nil {
		t.Fatal("want error when .config is not a directory")
	}
}

func TestInstallWriteFileError(t *testing.T) {
	home := t.TempDir()
	setHome(t, home)
	dir := filepath.Join(home, ".config", "systemd", "user")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// A directory at the unit path forces WriteFile to fail (is a directory).
	if err := os.MkdirAll(filepath.Join(dir, linux.UnitName), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := linux.Install(context.Background(), "/usr/local/bin/pmmcpd"); err == nil {
		t.Fatal("want error when unit path is a directory")
	}
}

func TestUninstallRemoveError(t *testing.T) {
	home := t.TempDir()
	setHome(t, home)
	dir := filepath.Join(home, ".config", "systemd", "user")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// A non-empty directory at the unit path makes os.Remove fail with an
	// error other than "not exist" (directory not empty).
	unitDir := filepath.Join(dir, linux.UnitName)
	if err := os.MkdirAll(unitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(unitDir, "child"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := linux.Uninstall(context.Background()); err == nil {
		t.Fatal("want error when unit path is a non-empty directory")
	}
}

func TestInstallRejectsControlChars(t *testing.T) {
	setHome(t, t.TempDir())
	// A newline in the path could otherwise inject [Service] directives.
	err := linux.Install(context.Background(), "/home/u/pmmcpd\nExecStartPre=/bin/rm -rf /")
	if err == nil {
		t.Fatal("want error for path with control characters")
	}
}
