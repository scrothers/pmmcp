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

//go:build linux

package linux_test

import (
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"unsafe"

	"github.com/scrothers/pmmcp/internal/sandbox"
	"github.com/scrothers/pmmcp/internal/sandbox/linux"
	"golang.org/x/sys/unix"
)

// landlock_restrict_self is irreversible for the OS thread that issues it, and
// Go may reschedule any goroutine onto that thread afterwards. Every test that
// reaches restrict_self therefore re-execs this test binary
// (TestLandlockHelperProcess) so the restriction dies with a throwaway process;
// the helper additionally pins itself to one OS thread, which the runtime
// destroys when the helper goroutine exits. Everything short of restrict_self
// (ABI probe, ruleset creation, rule addition) is exercised in-process.
const (
	helperModeEnv  = "PMMCP_LANDLOCK_HELPER"
	helperAllowEnv = "PMMCP_LANDLOCK_ALLOW"
	helperDenyEnv  = "PMMCP_LANDLOCK_DENY"

	// helperOK is printed by a helper that completed its mode successfully, so
	// the parent cannot mistake an inert (skipped) child for a pass.
	helperOK = "PMMCP-LANDLOCK-HELPER-OK"
)

const (
	modeRestrict      = "restrict"
	modeNesting       = "nesting"
	modeNoNewPrivs    = "no-new-privs"
	modeCreateEMFILE  = "create-emfile"
	modeAddPathEMFILE = "addpath-emfile"
)

func TestLandlockAvailable(t *testing.T) {
	t.Parallel()
	// On modern Fedora kernels this is true; don't fail CI on older hosts.
	if got, want := linux.LandlockAvailable(), linux.LandlockABIFlags(uintptr(unix.LANDLOCK_CREATE_RULESET_VERSION)) > 0; got != want {
		t.Errorf("LandlockAvailable() = %v, want %v", got, want)
	}
}

// TestLandlockABIFlagsRejectedQuery covers the errno arm of the ABI probe
// without depending on a pre-5.13 kernel: VERSION and ERRATA are mutually
// exclusive query flags, so the kernel must reject the combination.
func TestLandlockABIFlagsRejectedQuery(t *testing.T) {
	t.Parallel()
	bad := uintptr(unix.LANDLOCK_CREATE_RULESET_VERSION | unix.LANDLOCK_CREATE_RULESET_ERRATA)
	if got := linux.LandlockABIFlags(bad); got != 0 {
		t.Errorf("LandlockABIFlags(%#x) = %d, want 0 (rejected query reports unavailable)", bad, got)
	}
	if !linux.LandlockAvailable() {
		t.Skip("landlock not available")
	}
	if got := linux.LandlockABIFlags(uintptr(unix.LANDLOCK_CREATE_RULESET_VERSION)); got <= 0 {
		t.Errorf("LandlockABIFlags(VERSION) = %d, want a positive ABI", got)
	}
}

// TestLandlockRestrictPathsUnavailable covers the abi<=0 bail-out in-process:
// it returns before touching the kernel, so nothing is restricted.
func TestLandlockRestrictPathsUnavailable(t *testing.T) {
	t.Parallel()
	pol := sandbox.Policy{Profile: sandbox.Strict, WritableRoots: []string{t.TempDir()}}
	err := linux.LandlockRestrictPathsABI(0, linux.LandlockSysRoots, pol)
	if err == nil || !strings.Contains(err.Error(), "not available") {
		t.Fatalf("err = %v, want a landlock-unavailable error", err)
	}
}

// TestLandlockCreateRuleset exercises ruleset creation for both attribute
// layouts (the 8-byte pre-ABI-4 struct and the current one). Creating a ruleset
// never restricts anything, so this is safe in the main test process.
func TestLandlockCreateRuleset(t *testing.T) {
	t.Parallel()
	if !linux.LandlockAvailable() {
		t.Skip("landlock not available")
	}
	for _, abi := range []int{1, 3, 4, linux.LandlockABIFlags(uintptr(unix.LANDLOCK_CREATE_RULESET_VERSION))} {
		fd, err := linux.LandlockCreateRuleset(abi)
		if err != nil {
			t.Fatalf("LandlockCreateRuleset(%d): %v", abi, err)
		}
		if fd < 0 {
			t.Errorf("LandlockCreateRuleset(%d) fd = %d", abi, fd)
		}
		if err := unix.Close(fd); err != nil {
			t.Errorf("close ruleset fd: %v", err)
		}
	}
}

// TestLandlockAddPath covers rule addition and both of its failure arms.
// landlockAddPath never calls restrict_self, so the main test process is safe.
func TestLandlockAddPath(t *testing.T) {
	t.Parallel()
	if !linux.LandlockAvailable() {
		t.Skip("landlock not available")
	}
	fd, err := linux.LandlockCreateRuleset(linux.LandlockABIFlags(uintptr(unix.LANDLOCK_CREATE_RULESET_VERSION)))
	if err != nil {
		t.Fatalf("create ruleset: %v", err)
	}
	t.Cleanup(func() { _ = unix.Close(fd) })

	dir := t.TempDir()
	if err := linux.LandlockAddPath(fd, dir, linux.AccessFSWrite); err != nil {
		t.Errorf("add writable rule for %s: %v", dir, err)
	}
	if err := linux.LandlockAddPath(fd, "/usr", linux.AccessFSRead); err != nil {
		t.Errorf("add read rule for /usr: %v", err)
	}
	missing := filepath.Join(dir, "no-such-path")
	if err := linux.LandlockAddPath(fd, missing, linux.AccessFSRead); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("add rule for %s = %v, want ErrNotExist", missing, err)
	}
	if err := linux.LandlockAddPath(-1, "/usr", linux.AccessFSRead); !errors.Is(err, unix.EBADF) {
		t.Errorf("add rule on a closed ruleset = %v, want EBADF", err)
	}
}

// TestLandlockRestrictPathsChild is the real proof: a throwaway child process
// restricts itself to one writable root and then demonstrates that a sibling
// directory it could read a moment earlier is now denied, while the writable
// root and the read-only system roots still work. The parent stays unrestricted,
// which is itself asserted afterwards.
func TestLandlockRestrictPathsChild(t *testing.T) {
	if !linux.LandlockAvailable() {
		t.Skip("landlock not available")
	}
	allow, deny := landlockDirs(t)
	runLandlockHelper(t, modeRestrict, helperAllowEnv+"="+allow, helperDenyEnv+"="+deny)

	if _, err := os.ReadFile(filepath.Join(deny, "secret.txt")); err != nil {
		t.Errorf("parent lost access to %s: %v (the child's restriction must not escape)", deny, err)
	}
	if _, err := os.Stat(filepath.Join(allow, "written.txt")); err != nil {
		t.Errorf("child did not write inside its writable root: %v", err)
	}
}

// TestLandlockRestrictPathsLayerLimit covers the restrict_self error arm with a
// real kernel refusal: Landlock caps a thread at 16 stacked domains, so the
// seventeenth call fails with E2BIG. The injected system-root list also covers
// the "skip roots that are absent or not directories" arm.
func TestLandlockRestrictPathsLayerLimit(t *testing.T) {
	if !linux.LandlockAvailable() {
		t.Skip("landlock not available")
	}
	allow, _ := landlockDirs(t)
	runLandlockHelper(t, modeNesting, helperAllowEnv+"="+allow)
}

// TestLandlockRestrictPathsNoNewPrivs covers the PR_SET_NO_NEW_PRIVS failure
// arm. The helper installs a seccomp filter that fails every prctl, so the
// kernel really does reject the call.
func TestLandlockRestrictPathsNoNewPrivs(t *testing.T) {
	if !linux.LandlockAvailable() {
		t.Skip("landlock not available")
	}
	allow, _ := landlockDirs(t)
	runLandlockHelper(t, modeNoNewPrivs, helperAllowEnv+"="+allow)
}

// TestLandlockCreateRulesetExhausted covers the create_ruleset error arm: the
// syscall returns a descriptor, so a tight RLIMIT_NOFILE makes it fail EMFILE.
func TestLandlockCreateRulesetExhausted(t *testing.T) {
	if !linux.LandlockAvailable() {
		t.Skip("landlock not available")
	}
	allow, _ := landlockDirs(t)
	runLandlockHelper(t, modeCreateEMFILE, helperAllowEnv+"="+allow)
}

// TestLandlockAddPathExhausted covers both rule-addition arms of the ruleset
// build: system roots are best-effort (failures are skipped) while a writable
// root that cannot be opened aborts the whole restriction.
func TestLandlockAddPathExhausted(t *testing.T) {
	if !linux.LandlockAvailable() {
		t.Skip("landlock not available")
	}
	allow, _ := landlockDirs(t)
	runLandlockHelper(t, modeAddPathEMFILE, helperAllowEnv+"="+allow)
}

// landlockDirs builds the allowed/denied pair the helpers probe. The denied
// directory is a sibling of the allowed one and is never a writable root.
func landlockDirs(t *testing.T) (allow, deny string) {
	t.Helper()
	base := t.TempDir()
	allow = filepath.Join(base, "allow")
	deny = filepath.Join(base, "deny")
	for _, d := range []string{allow, deny} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(deny, "secret.txt"), []byte("secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// A regular file the nesting helper can point a "system root" at.
	if err := os.WriteFile(filepath.Join(allow, "regular-file"), []byte("x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return allow, deny
}

// runLandlockHelper re-execs this test binary in helper mode. The child inherits
// the run's coverage directory so its counters fold into the parent's profile.
func runLandlockHelper(t *testing.T, mode string, env ...string) {
	t.Helper()
	args := []string{"-test.run=^TestLandlockHelperProcess$", "-test.v"}
	if f := flag.Lookup("test.gocoverdir"); f != nil && f.Value.String() != "" {
		args = append(args, "-test.gocoverdir="+f.Value.String())
	}
	cmd := exec.Command(os.Args[0], args...)
	cmd.Env = append(os.Environ(), append([]string{helperModeEnv + "=" + mode}, env...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("landlock helper %q failed: %v\n%s", mode, err, out)
	}
	if !strings.Contains(string(out), helperOK+" "+mode) {
		t.Fatalf("landlock helper %q did not reach its success marker:\n%s", mode, out)
	}
}

// TestLandlockHelperProcess is the re-exec entry point. It is inert during a
// normal `go test` run and only does work when the parent sets the mode env var.
func TestLandlockHelperProcess(t *testing.T) {
	mode := os.Getenv(helperModeEnv)
	if mode == "" {
		t.Skip("not a landlock helper re-exec")
	}
	// Landlock and seccomp are thread-scoped. Pin this goroutine to its OS
	// thread so the restriction lands where it is probed, and never unlock: the
	// runtime destroys a locked thread when its goroutine exits, so the
	// restricted thread can never be handed to another goroutine. Coverage is
	// flushed later by the main goroutine on a different, unrestricted thread.
	runtime.LockOSThread()

	abi := linux.LandlockABIFlags(uintptr(unix.LANDLOCK_CREATE_RULESET_VERSION))
	if abi <= 0 {
		t.Fatalf("helper %q: landlock unavailable in child", mode)
	}

	switch mode {
	case modeRestrict:
		helperRestrict(t)
	case modeNesting:
		helperNesting(t, abi)
	case modeNoNewPrivs:
		helperNoNewPrivs(t)
	case modeCreateEMFILE:
		helperCreateEMFILE(t)
	case modeAddPathEMFILE:
		helperAddPathEMFILE(t)
	default:
		t.Fatalf("unknown helper mode %q", mode)
	}
	fmt.Println(helperOK, mode)
}

func helperRestrict(t *testing.T) {
	t.Helper()
	allow, deny := os.Getenv(helperAllowEnv), os.Getenv(helperDenyEnv)
	secret := filepath.Join(deny, "secret.txt")
	if _, err := os.ReadFile(secret); err != nil {
		t.Fatalf("pre-restriction read of %s: %v", secret, err)
	}

	pol := sandbox.Policy{
		Profile: sandbox.Strict,
		// The empty root exercises the guard that stops filepath.Clean("") from
		// widening the ruleset to the process working directory.
		WritableRoots: []string{allow, ""},
	}
	if err := linux.LandlockRestrictPaths(pol); err != nil {
		t.Fatalf("LandlockRestrictPaths: %v", err)
	}

	if err := os.WriteFile(filepath.Join(allow, "written.txt"), []byte("ok\n"), 0o600); err != nil {
		t.Fatalf("write inside the writable root: %v", err)
	}
	if _, err := os.ReadFile(secret); !errors.Is(err, fs.ErrPermission) {
		t.Fatalf("read of %s after restriction = %v, want permission denied", secret, err)
	}
	if _, err := os.ReadDir(deny); !errors.Is(err, fs.ErrPermission) {
		t.Fatalf("readdir of %s after restriction = %v, want permission denied", deny, err)
	}
	if err := os.WriteFile(filepath.Join(deny, "nope.txt"), []byte("x\n"), 0o600); !errors.Is(err, fs.ErrPermission) {
		t.Fatalf("write into %s after restriction = %v, want permission denied", deny, err)
	}
	// The read-only system roots stay reachable so a restricted helper can still
	// find and execute its tooling.
	if _, err := os.ReadDir("/usr"); err != nil {
		t.Fatalf("readdir of /usr after restriction: %v", err)
	}
	// The working directory was never granted, so the empty writable root really
	// was dropped rather than resolved to ".".
	if _, err := os.ReadDir("."); !errors.Is(err, fs.ErrPermission) {
		t.Fatalf("readdir of the working directory = %v, want permission denied", err)
	}
}

func helperNesting(t *testing.T, abi int) {
	t.Helper()
	allow := os.Getenv(helperAllowEnv)
	// A missing path and a regular file exercise the "not a directory" skip;
	// /usr keeps the ruleset realistic.
	sysRoots := []string{"/usr", filepath.Join(allow, "no-such-root"), filepath.Join(allow, "regular-file")}
	pol := sandbox.Policy{Profile: sandbox.Strict, WritableRoots: []string{allow}}

	var err error
	for i := 1; i <= 24; i++ {
		if err = linux.LandlockRestrictPathsABI(abi, sysRoots, pol); err != nil {
			t.Logf("restrict_self refused layer %d: %v", i, err)
			break
		}
	}
	if !errors.Is(err, unix.E2BIG) {
		t.Fatalf("stacked restrictions error = %v, want E2BIG at the landlock layer limit", err)
	}
	if !strings.Contains(err.Error(), "restrict_self") {
		t.Fatalf("error did not come from restrict_self: %v", err)
	}
}

func helperNoNewPrivs(t *testing.T) {
	t.Helper()
	// Unprivileged seccomp requires no_new_privs first; the filter then makes
	// every later prctl fail, including the one LandlockRestrictPaths issues.
	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		t.Fatalf("prctl(PR_SET_NO_NEW_PRIVS): %v", err)
	}
	prog := []unix.SockFilter{
		{Code: unix.BPF_LD | unix.BPF_W | unix.BPF_ABS, K: 0}, // seccomp_data.nr
		{Code: unix.BPF_JMP | unix.BPF_JEQ | unix.BPF_K, K: uint32(unix.SYS_PRCTL), Jt: 0, Jf: 1},
		{Code: unix.BPF_RET | unix.BPF_K, K: unix.SECCOMP_RET_ERRNO | uint32(unix.EPERM)},
		{Code: unix.BPF_RET | unix.BPF_K, K: unix.SECCOMP_RET_ALLOW},
	}
	fprog := unix.SockFprog{Len: uint16(len(prog)), Filter: &prog[0]}
	if err := unix.Prctl(unix.PR_SET_SECCOMP, unix.SECCOMP_MODE_FILTER, uintptr(unsafe.Pointer(&fprog)), 0, 0); err != nil {
		t.Fatalf("install seccomp filter: %v", err)
	}

	pol := sandbox.Policy{Profile: sandbox.Strict, WritableRoots: []string{os.Getenv(helperAllowEnv)}}
	err := linux.LandlockRestrictPaths(pol)
	if !errors.Is(err, unix.EPERM) || !strings.Contains(fmt.Sprint(err), "no_new_privs") {
		t.Fatalf("LandlockRestrictPaths under a prctl-denying filter = %v, want no_new_privs EPERM", err)
	}
}

func helperCreateEMFILE(t *testing.T) {
	t.Helper()
	pol := sandbox.Policy{Profile: sandbox.Strict, WritableRoots: []string{os.Getenv(helperAllowEnv)}}
	restore := budgetFDs(t, 0)
	err := linux.LandlockRestrictPaths(pol)
	restore()
	if !errors.Is(err, unix.EMFILE) || !strings.Contains(fmt.Sprint(err), "create_ruleset") {
		t.Fatalf("LandlockRestrictPaths with no spare descriptors = %v, want create_ruleset EMFILE", err)
	}
}

func helperAddPathEMFILE(t *testing.T) {
	t.Helper()
	pol := sandbox.Policy{Profile: sandbox.Strict, WritableRoots: []string{os.Getenv(helperAllowEnv)}}
	// One spare descriptor: enough for the ruleset, not for any path handle.
	restore := budgetFDs(t, 1)
	err := linux.LandlockRestrictPaths(pol)
	restore()
	if !errors.Is(err, unix.EMFILE) || !strings.Contains(fmt.Sprint(err), "add writable") {
		t.Fatalf("LandlockRestrictPaths with one spare descriptor = %v, want add-writable EMFILE", err)
	}
}

// budgetFDs lowers RLIMIT_NOFILE so exactly spare further descriptors can be
// allocated, and returns a restore func. Only safe inside a helper process.
func budgetFDs(t *testing.T, spare int) func() {
	t.Helper()
	var lim unix.Rlimit
	if err := unix.Getrlimit(unix.RLIMIT_NOFILE, &lim); err != nil {
		t.Fatalf("getrlimit: %v", err)
	}
	probe, err := unix.Open(os.DevNull, unix.O_RDONLY|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatalf("probe open: %v", err)
	}
	tight := lim
	tight.Cur = uint64(probe) + 1 + uint64(spare)
	if err := unix.Setrlimit(unix.RLIMIT_NOFILE, &tight); err != nil {
		_ = unix.Close(probe)
		t.Fatalf("setrlimit: %v", err)
	}
	return func() {
		_ = unix.Setrlimit(unix.RLIMIT_NOFILE, &lim)
		_ = unix.Close(probe)
	}
}
