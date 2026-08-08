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

package windows_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scrothers/pmmcp/internal/service/windows"
)

func TestInstallUninstall(t *testing.T) {
	root := t.TempDir()
	t.Setenv("LOCALAPPDATA", root)
	t.Setenv("HOME", filepath.Join(root, "unused-home"))

	ctx := context.Background()
	pmmcpd := `C:\Tools\pmmcpd.exe`
	if err := windows.Install(ctx, pmmcpd); err != nil {
		t.Fatalf("Install: %v", err)
	}
	dir := filepath.Join(root, windows.DirName)
	bat, err := os.ReadFile(filepath.Join(dir, windows.BatName))
	if err != nil {
		t.Fatalf("read bat: %v", err)
	}
	if !strings.Contains(string(bat), pmmcpd) {
		t.Fatalf("bat missing binary: %s", bat)
	}
	xml, err := os.ReadFile(filepath.Join(dir, windows.TaskXMLName))
	if err != nil {
		t.Fatalf("read xml: %v", err)
	}
	if !strings.Contains(string(xml), "LogonTrigger") {
		t.Fatalf("xml missing LogonTrigger:\n%s", xml)
	}
	readme, err := os.ReadFile(filepath.Join(dir, windows.ReadmeName))
	if err != nil {
		t.Fatalf("read readme: %v", err)
	}
	if !strings.Contains(string(readme), "elevation") && !strings.Contains(strings.ToLower(string(readme)), "administrator") {
		t.Fatalf("readme should document elevation:\n%s", readme)
	}

	if !strings.Contains(string(xml), `encoding="UTF-8"`) {
		t.Fatalf("xml must declare UTF-8 (it is written as UTF-8):\n%s", xml)
	}

	if err := windows.Uninstall(ctx); err != nil {
		t.Fatalf("Uninstall: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, windows.BatName)); !os.IsNotExist(err) {
		t.Fatalf("bat still present: %v", err)
	}
	// Second uninstall is idempotent.
	if err := windows.Uninstall(ctx); err != nil {
		t.Fatalf("Uninstall again: %v", err)
	}
}

func TestInstallDirFallback(t *testing.T) {
	home := t.TempDir()
	t.Setenv("LOCALAPPDATA", "")
	t.Setenv("HOME", home)
	p, err := windows.InstallDir()
	if err != nil {
		t.Fatalf("InstallDir: %v", err)
	}
	want := filepath.Join(home, "AppData", "Local", windows.DirName)
	if p != want {
		t.Fatalf("InstallDir = %q, want %q", p, want)
	}
}

func TestInstallContextCanceled(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := windows.Install(ctx, `C:\Tools\pmmcpd.exe`); err == nil {
		t.Fatal("want error for canceled context")
	}
	if err := windows.Uninstall(ctx); err == nil {
		t.Fatal("want error for canceled context on uninstall")
	}
}

func TestInstallEscapesCommandPath(t *testing.T) {
	root := t.TempDir()
	t.Setenv("LOCALAPPDATA", root)
	ctx := context.Background()
	// The daemon path is the task's <Command>; a "&" in it must be XML-escaped
	// or the generated task fails to import.
	if err := windows.Install(ctx, `C:\App & Tools\pmmcpd.exe`); err != nil {
		t.Fatalf("Install: %v", err)
	}
	xml, err := os.ReadFile(filepath.Join(root, windows.DirName, windows.TaskXMLName))
	if err != nil {
		t.Fatalf("read xml: %v", err)
	}
	body := string(xml)
	if strings.Contains(body, "App & Tools") {
		t.Fatalf("raw ampersand in XML Command breaks the import:\n%s", body)
	}
	if !strings.Contains(body, "App &amp; Tools") {
		t.Fatalf("command path not XML-escaped:\n%s", body)
	}
}

// TestInstallXMLIsDaemonReady locks in the task settings that make the generated
// task suitable for a long-running daemon: no execution time limit (Task
// Scheduler otherwise kills it after ~72h), a direct `pmmcpd run` action with a
// working directory, and restart-on-failure.
func TestInstallXMLIsDaemonReady(t *testing.T) {
	root := t.TempDir()
	t.Setenv("LOCALAPPDATA", root)
	if err := windows.Install(context.Background(), `C:\Tools\pmmcpd.exe`); err != nil {
		t.Fatalf("Install: %v", err)
	}
	xml, err := os.ReadFile(filepath.Join(root, windows.DirName, windows.TaskXMLName))
	if err != nil {
		t.Fatalf("read xml: %v", err)
	}
	body := string(xml)
	for _, want := range []string{
		"<ExecutionTimeLimit>PT0S</ExecutionTimeLimit>", // no time limit for a daemon
		"<Command>C:\\Tools\\pmmcpd.exe</Command>",      // exec pmmcpd directly
		"<Arguments>run</Arguments>",
		"<WorkingDirectory>",
		"<RestartOnFailure>",
		"<StartWhenAvailable>true</StartWhenAvailable>",
		"<Principals>",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("task XML missing %q:\n%s", want, body)
		}
	}
}

func TestInstallPercentInPath(t *testing.T) {
	root := t.TempDir()
	t.Setenv("LOCALAPPDATA", root)
	if err := windows.Install(context.Background(), `C:\a%b\pmmcpd.exe`); err != nil {
		t.Fatalf("Install: %v", err)
	}
	bat, err := os.ReadFile(filepath.Join(root, windows.DirName, windows.BatName))
	if err != nil {
		t.Fatalf("read bat: %v", err)
	}
	if !strings.Contains(string(bat), `C:\a%%b\pmmcpd.exe`) {
		t.Fatalf("percent not doubled for cmd.exe:\n%s", bat)
	}
}

func TestInstallEmptyPath(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	if err := windows.Install(context.Background(), ""); err == nil {
		t.Fatal("want error for empty path")
	}
}

func TestInstallDirHomeDirError(t *testing.T) {
	t.Setenv("LOCALAPPDATA", "")
	t.Setenv("HOME", "")
	if _, err := windows.InstallDir(); err == nil {
		t.Fatal("want error when LOCALAPPDATA and HOME are both unset")
	}
}

func TestInstallInstallDirError(t *testing.T) {
	t.Setenv("LOCALAPPDATA", "")
	t.Setenv("HOME", "")
	if err := windows.Install(context.Background(), `C:\Tools\pmmcpd.exe`); err == nil {
		t.Fatal("want error when installDir cannot be resolved")
	}
}

func TestUninstallInstallDirError(t *testing.T) {
	t.Setenv("LOCALAPPDATA", "")
	t.Setenv("HOME", "")
	if err := windows.Uninstall(context.Background()); err == nil {
		t.Fatal("want error when installDir cannot be resolved")
	}
}

func TestInstallMkdirError(t *testing.T) {
	root := t.TempDir()
	// A regular file where LOCALAPPDATA should be a directory forces
	// MkdirAll(LOCALAPPDATA/pmmcp) to fail with "not a directory".
	localAppData := filepath.Join(root, "not-a-dir")
	if err := os.WriteFile(localAppData, []byte("not a dir"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LOCALAPPDATA", localAppData)
	if err := windows.Install(context.Background(), `C:\Tools\pmmcpd.exe`); err == nil {
		t.Fatal("want error when LOCALAPPDATA is not a directory")
	}
}

func TestInstallBatWriteFileError(t *testing.T) {
	root := t.TempDir()
	t.Setenv("LOCALAPPDATA", root)
	dir := filepath.Join(root, windows.DirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// A directory at the bat path forces WriteFile to fail (is a directory).
	if err := os.MkdirAll(filepath.Join(dir, windows.BatName), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := windows.Install(context.Background(), `C:\Tools\pmmcpd.exe`); err == nil {
		t.Fatal("want error when bat path is a directory")
	}
}

func TestInstallXMLWriteFileError(t *testing.T) {
	root := t.TempDir()
	t.Setenv("LOCALAPPDATA", root)
	dir := filepath.Join(root, windows.DirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// A directory at the task-XML path forces WriteFile to fail (is a
	// directory); the bat file above it writes successfully.
	if err := os.MkdirAll(filepath.Join(dir, windows.TaskXMLName), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := windows.Install(context.Background(), `C:\Tools\pmmcpd.exe`); err == nil {
		t.Fatal("want error when task-XML path is a directory")
	}
}

func TestInstallReadmeWriteFileError(t *testing.T) {
	root := t.TempDir()
	t.Setenv("LOCALAPPDATA", root)
	dir := filepath.Join(root, windows.DirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// A directory at the readme path forces WriteFile to fail (is a
	// directory); the bat and task-XML files above it write successfully.
	if err := os.MkdirAll(filepath.Join(dir, windows.ReadmeName), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := windows.Install(context.Background(), `C:\Tools\pmmcpd.exe`); err == nil {
		t.Fatal("want error when readme path is a directory")
	}
}

func TestUninstallRemoveError(t *testing.T) {
	root := t.TempDir()
	t.Setenv("LOCALAPPDATA", root)
	dir := filepath.Join(root, windows.DirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// A non-empty directory at the bat path makes os.Remove fail with an
	// error other than "not exist" (directory not empty).
	batDir := filepath.Join(dir, windows.BatName)
	if err := os.MkdirAll(batDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(batDir, "child"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := windows.Uninstall(context.Background()); err == nil {
		t.Fatal("want error when bat path is a non-empty directory")
	}
}

func TestInstallRejectsBadPaths(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	ctx := context.Background()
	if err := windows.Install(ctx, "C:\\Tools\\pmmcpd\nmalice"); err == nil {
		t.Fatal("want error for control characters in path")
	}
	if err := windows.Install(ctx, `C:\Tools\"evil".exe`); err == nil {
		t.Fatal("want error for quote in path")
	}
}
