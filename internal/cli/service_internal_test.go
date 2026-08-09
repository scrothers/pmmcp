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
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/scrothers/pmmcp/internal/domain"
)

// TestInstallServiceMissingBinValue covers install-service's --bin-requires-a-
// value branch, now enforced by cobra's flag parsing.
func TestInstallServiceMissingBinValue(t *testing.T) {
	t.Parallel()
	err := Run(context.Background(), []string{"install-service", "--bin"})
	if err == nil {
		t.Fatal("want error for --bin with no value")
	}
}

// TestInstallServiceResolveError covers installService's branch that
// propagates a resolveDaemonPath failure: no --bin override, no pmmcpd next
// to the test binary, and a PATH that cannot resolve pmmcpd.
func TestInstallServiceResolveError(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Skipf("os.Executable: %v", err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(exe), "pmmcpd")); err == nil {
		t.Skip("a pmmcpd sibling already exists next to the test binary")
	}
	t.Setenv("PATH", t.TempDir())
	if err := installService(context.Background(), ""); err == nil {
		t.Fatal("want resolveDaemonPath error to propagate")
	}
}

// unsetHome clears every env var os.UserHomeDir consults on any of our
// supported GOOS (HOME on unix/darwin, USERPROFILE on windows), so a test
// forcing a home-resolution error gets that error on every CI platform.
func unsetHome(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")
}

// TestInstallServiceInstallError covers installService's service.Install
// error branch: an unset $HOME makes the linux/darwin drivers fail to
// resolve a home directory.
func TestInstallServiceInstallError(t *testing.T) {
	unsetHome(t)
	t.Setenv("LOCALAPPDATA", "")
	err := installService(context.Background(), "/usr/local/bin/pmmcpd")
	if err == nil {
		t.Fatal("want service.Install error when $HOME is unset")
	}
}

// TestUninstallServiceError covers uninstallService's service.Uninstall
// error branch.
func TestUninstallServiceError(t *testing.T) {
	unsetHome(t)
	t.Setenv("LOCALAPPDATA", "")
	if err := uninstallService(context.Background()); err == nil {
		t.Fatal("want service.Uninstall error when $HOME is unset")
	}
}

// TestResolveDaemonPathSibling covers the branch where pmmcpd is found next
// to the running executable.
func TestResolveDaemonPathSibling(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Skipf("os.Executable: %v", err)
	}
	sibling := filepath.Join(filepath.Dir(exe), "pmmcpd")
	if _, err := os.Stat(sibling); err == nil {
		t.Skip("a pmmcpd sibling already exists next to the test binary")
	}
	if err := os.WriteFile(sibling, []byte(""), 0o644); err != nil {
		t.Skipf("cannot write sibling next to test binary: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(sibling) })

	got, err := resolveDaemonPath("")
	if err != nil {
		t.Fatalf("resolveDaemonPath: %v", err)
	}
	if !strings.HasSuffix(got, "pmmcpd") || !filepath.IsAbs(got) {
		t.Fatalf("resolveDaemonPath = %q, want absolute path ending in pmmcpd", got)
	}
}

// TestResolveDaemonPathViaPATH covers the branch where pmmcpd is resolved via
// exec.LookPath.
func TestResolveDaemonPathViaPATH(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PATH lookup test targets POSIX exec bit semantics")
	}
	exe, err := os.Executable()
	if err != nil {
		t.Skipf("os.Executable: %v", err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(exe), "pmmcpd")); err == nil {
		t.Skip("a pmmcpd sibling already exists next to the test binary")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "pmmcpd")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)

	got, err := resolveDaemonPath("")
	if err != nil {
		t.Fatalf("resolveDaemonPath: %v", err)
	}
	if !strings.HasSuffix(got, "pmmcpd") || !filepath.IsAbs(got) {
		t.Fatalf("resolveDaemonPath = %q, want absolute path ending in pmmcpd", got)
	}
}

// TestResolveDaemonPathNotFound covers the terminal not-found branch.
func TestResolveDaemonPathNotFound(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Skipf("os.Executable: %v", err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(exe), "pmmcpd")); err == nil {
		t.Skip("a pmmcpd sibling already exists next to the test binary")
	}
	t.Setenv("PATH", t.TempDir())
	_, err = resolveDaemonPath("")
	if err == nil {
		t.Fatal("want not-found error")
	}
	var derr *domain.Error
	var de *domain.Error
	if errors.As(err, &de) {
		derr = de
	}
	if derr == nil || derr.Code != domain.CodeNotFound {
		t.Fatalf("resolveDaemonPath error = %v, want CodeNotFound", err)
	}
}

// TestResolveDaemonPathAbsError covers the --bin override's filepath.Abs
// error branch: with the working directory removed out from under the
// process, resolving a relative path via os.Getwd fails.
func TestResolveDaemonPathAbsError(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("removed-cwd Abs failure is Linux-specific")
	}
	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })
	if err := os.Remove(dir); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	if _, err := resolveDaemonPath("relative/pmmcpd"); err == nil {
		t.Fatal("want filepath.Abs error from a removed working directory")
	}
}
