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

package darwin_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scrothers/pmmcp/internal/service/darwin"
)

func TestInstallUninstall(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	ctx := context.Background()
	pmmcpd := "/opt/homebrew/bin/pmmcpd"
	if err := darwin.Install(ctx, pmmcpd); err != nil {
		t.Fatalf("Install: %v", err)
	}
	plist := filepath.Join(home, "Library", "LaunchAgents", darwin.PlistName)
	data, err := os.ReadFile(plist)
	if err != nil {
		t.Fatalf("read plist: %v", err)
	}
	body := string(data)
	if !strings.Contains(body, pmmcpd) {
		t.Fatalf("plist missing binary path:\n%s", body)
	}
	if !strings.Contains(body, "<key>RunAtLoad</key><true/>") {
		t.Fatalf("plist missing RunAtLoad:\n%s", body)
	}
	// KeepAlive restarts on crash but honors a clean (exit-0) stop, matching the
	// Linux unit's Restart=on-failure — so it is a dict, not a blanket <true/>.
	if !strings.Contains(body, "<key>KeepAlive</key><dict><key>SuccessfulExit</key><false/></dict>") {
		t.Fatalf("plist missing crash-only KeepAlive:\n%s", body)
	}
	// Daemon-appropriate settings: crash throttle, background type, and captured
	// stdout/stderr so a daemon that fails to start is diagnosable.
	for _, want := range []string{
		"<key>ThrottleInterval</key>",
		"<key>ProcessType</key><string>Background</string>",
		"<key>StandardOutPath</key>",
		"<key>StandardErrorPath</key>",
		"<key>WorkingDirectory</key>",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("plist missing %q:\n%s", want, body)
		}
	}
	// The log directory the agent writes to is created at install time.
	logDir := filepath.Join(home, "Library", "Logs", "pmmcp")
	if fi, err := os.Stat(logDir); err != nil || !fi.IsDir() {
		t.Errorf("log dir %q not created: err=%v", logDir, err)
	}

	if err := darwin.Uninstall(ctx); err != nil {
		t.Fatalf("Uninstall: %v", err)
	}
	if _, err := os.Stat(plist); !os.IsNotExist(err) {
		t.Fatalf("plist still present: %v", err)
	}
	// Second uninstall is idempotent.
	if err := darwin.Uninstall(ctx); err != nil {
		t.Fatalf("Uninstall again: %v", err)
	}
}

func TestPlistPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	p, err := darwin.PlistPath()
	if err != nil {
		t.Fatalf("PlistPath: %v", err)
	}
	want := filepath.Join(home, "Library", "LaunchAgents", darwin.PlistName)
	if p != want {
		t.Fatalf("PlistPath = %q, want %q", p, want)
	}
}

func TestInstallContextCanceled(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := darwin.Install(ctx, "/opt/pmmcpd"); err == nil {
		t.Fatal("want error for canceled context")
	}
	if err := darwin.Uninstall(ctx); err == nil {
		t.Fatal("want error for canceled context on uninstall")
	}
}

func TestLabelMatchesSpec(t *testing.T) {
	if darwin.Label != "com.scrothers.pmmcpd" {
		t.Fatalf("Label = %q, want com.scrothers.pmmcpd", darwin.Label)
	}
}

func TestInstallEmptyPath(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := darwin.Install(context.Background(), ""); err == nil {
		t.Fatal("want error for empty path")
	}
}

func TestInstallEscapesPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	pmmcpd := "/Applications/My & Tools/pmmcpd"
	if err := darwin.Install(context.Background(), pmmcpd); err != nil {
		t.Fatalf("Install: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(home, "Library", "LaunchAgents", darwin.PlistName))
	if err != nil {
		t.Fatalf("read plist: %v", err)
	}
	body := string(data)
	if strings.Contains(body, "My & Tools") {
		t.Fatalf("raw ampersand not escaped, plist is invalid XML:\n%s", body)
	}
	if !strings.Contains(body, "My &amp; Tools") {
		t.Fatalf("path not XML-escaped:\n%s", body)
	}
}

func TestInstallRejectsControlChars(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := darwin.Install(context.Background(), "/opt/pmmcpd\n<key>evil</key>"); err == nil {
		t.Fatal("want error for control characters in path")
	}
}

func TestInstallHomeDirError(t *testing.T) {
	t.Setenv("HOME", "")
	if err := darwin.Install(context.Background(), "/opt/pmmcpd"); err == nil {
		t.Fatal("want error when $HOME is unset")
	}
}

func TestUninstallHomeDirError(t *testing.T) {
	t.Setenv("HOME", "")
	if err := darwin.Uninstall(context.Background()); err == nil {
		t.Fatal("want error when $HOME is unset")
	}
}

func TestPlistPathHomeDirError(t *testing.T) {
	t.Setenv("HOME", "")
	if _, err := darwin.PlistPath(); err == nil {
		t.Fatal("want error when $HOME is unset")
	}
}

func TestInstallMkdirError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	// A regular file where the LaunchAgents parent directory should be
	// forces MkdirAll to fail with "not a directory".
	if err := os.WriteFile(filepath.Join(home, "Library"), []byte("not a dir"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := darwin.Install(context.Background(), "/opt/pmmcpd"); err == nil {
		t.Fatal("want error when LaunchAgents parent is not a directory")
	}
}

func TestInstallWriteFileError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, "Library", "LaunchAgents")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// A directory at the plist path forces WriteFile to fail (is a directory).
	if err := os.MkdirAll(filepath.Join(dir, darwin.PlistName), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := darwin.Install(context.Background(), "/opt/pmmcpd"); err == nil {
		t.Fatal("want error when plist path is a directory")
	}
}

func TestUninstallRemoveError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, "Library", "LaunchAgents")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// A non-empty directory at the plist path makes os.Remove fail with an
	// error other than "not exist" (directory not empty).
	plistDir := filepath.Join(dir, darwin.PlistName)
	if err := os.MkdirAll(plistDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(plistDir, "child"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := darwin.Uninstall(context.Background()); err == nil {
		t.Fatal("want error when plist path is a non-empty directory")
	}
}
