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
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/scrothers/pmmcp/internal/api"
)

// TestDialLoadCfgError exercises dial's loadCfg error branch: PMMCP_CONFIG
// pointing at a non-.toml extension is rejected by config.Load before any
// file is even read.
func TestDialLoadCfgError(t *testing.T) {
	t.Setenv("PMMCP_CONFIG", "/nonexistent/config.json")
	if err := Run(context.Background(), []string{"list"}); err == nil {
		t.Fatal("want error from loadCfg via dial")
	}
}

// TestRunDoctorLoadCfgError exercises runDoctor's own loadCfg error branch.
func TestRunDoctorLoadCfgError(t *testing.T) {
	t.Setenv("PMMCP_CONFIG", "/nonexistent/config.json")
	if err := Run(context.Background(), []string{"doctor"}); err == nil {
		t.Fatal("want error from loadCfg via runDoctor")
	}
}

// TestRunDoctorNotOK exercises runDoctor's rep.OK==false branch: an
// unreachable daemon endpoint makes doctor.Check report not-OK.
func TestRunDoctorNotOK(t *testing.T) {
	t.Setenv("PMMCP_IPC_ENDPOINT", filepath.Join(t.TempDir(), "absent.sock"))
	if err := Run(context.Background(), []string{"doctor"}); err == nil {
		t.Fatal("want error when daemon is unreachable")
	}
}

// TestWithIDUsageError covers withID's len(args)<1 branch for every verb that
// dispatches through it.
func TestWithIDUsageError(t *testing.T) {
	t.Parallel()
	for _, cmd := range []string{"stop", "restart", "remove", "wait", "enable", "disable", "health"} {
		if err := Run(context.Background(), []string{cmd}); err == nil {
			t.Errorf("Run([%s]) = nil, want usage error", cmd)
		}
	}
}

// TestWithIDDaemonDown covers withID's dial-error branch.
func TestWithIDDaemonDown(t *testing.T) {
	t.Setenv("PMMCP_IPC_ENDPOINT", filepath.Join(t.TempDir(), "absent.sock"))
	if err := Run(context.Background(), []string{"stop", "web"}); err == nil {
		t.Fatal("want dial error")
	}
}

// TestWithIDCallError covers withID's c.Call error branch.
func TestWithIDCallError(t *testing.T) {
	sock := startScriptedDaemon(t, map[string]scriptedResponse{
		api.MethodStop: {errMsg: "boom", errCode: "internal"},
	})
	t.Setenv("PMMCP_IPC_ENDPOINT", sock)
	if err := Run(context.Background(), []string{"stop", "web"}); err == nil {
		t.Fatal("want call error")
	}
}

// TestListBranches drives list's dial-error, call-error, empty-result, and
// populated-result (non-JSON print loop) branches via Run, plus a --json
// variant of the populated-result branch.
func TestListBranches(t *testing.T) {
	t.Run("dial error", func(t *testing.T) {
		t.Setenv("PMMCP_IPC_ENDPOINT", filepath.Join(t.TempDir(), "absent.sock"))
		if err := Run(context.Background(), []string{"list"}); err == nil {
			t.Fatal("want dial error")
		}
	})

	t.Run("call error", func(t *testing.T) {
		sock := startScriptedDaemon(t, map[string]scriptedResponse{
			api.MethodList: {errMsg: "boom", errCode: "internal"},
		})
		t.Setenv("PMMCP_IPC_ENDPOINT", sock)
		if err := Run(context.Background(), []string{"list", "--all"}); err == nil {
			t.Fatal("want call error")
		}
	})

	t.Run("empty result", func(t *testing.T) {
		sock := startScriptedDaemon(t, map[string]scriptedResponse{
			api.MethodList: {payload: []byte(`[]`)},
		})
		t.Setenv("PMMCP_IPC_ENDPOINT", sock)
		if err := Run(context.Background(), []string{"list", "--all"}); err != nil {
			t.Fatalf("list: %v", err)
		}
	})

	t.Run("populated result", func(t *testing.T) {
		b, _ := json.Marshal([]api.ProcessView{{ID: "proc-1", Name: "web", Status: "running", PID: 1, Command: []string{"./web"}}})
		sock := startScriptedDaemon(t, map[string]scriptedResponse{
			api.MethodList: {payload: b},
		})
		t.Setenv("PMMCP_IPC_ENDPOINT", sock)
		if err := Run(context.Background(), []string{"list", "--all"}); err != nil {
			t.Fatalf("list: %v", err)
		}
	})

	t.Run("populated result json", func(t *testing.T) {
		b, _ := json.Marshal([]api.ProcessView{{ID: "proc-1", Name: "web", Status: "running", PID: 1, Command: []string{"./web"}}})
		sock := startScriptedDaemon(t, map[string]scriptedResponse{
			api.MethodList: {payload: b},
		})
		t.Setenv("PMMCP_IPC_ENDPOINT", sock)
		if err := Run(context.Background(), []string{"list", "--json", "--all"}); err != nil {
			t.Fatalf("list: %v", err)
		}
	})
}

// TestStatusUsageError covers status's len(args)<1 branch.
func TestStatusUsageError(t *testing.T) {
	t.Parallel()
	if err := Run(context.Background(), []string{"status"}); err == nil {
		t.Error("want usage error")
	}
}

// TestStatusDialError explicitly targets the dial-error branch with a
// guaranteed-absent endpoint.
func TestStatusDialError(t *testing.T) {
	t.Setenv("PMMCP_IPC_ENDPOINT", filepath.Join(t.TempDir(), "absent.sock"))
	if err := Run(context.Background(), []string{"status", "proc-1"}); err == nil {
		t.Fatal("want dial error")
	}
}

// TestStatusCallError targets status's c.Call error branch.
func TestStatusCallError(t *testing.T) {
	sock := startScriptedDaemon(t, map[string]scriptedResponse{
		api.MethodStatus: {errMsg: "boom", errCode: "internal"},
	})
	t.Setenv("PMMCP_IPC_ENDPOINT", sock)
	if err := Run(context.Background(), []string{"status", "proc-1"}); err == nil {
		t.Fatal("want call error")
	}
}

// TestLogsBranches covers logs' usage errors (bare logs/grep/errors, and grep
// missing pattern), dial error, call error, and the trailing-newline branch.
func TestLogsBranches(t *testing.T) {
	for _, args := range [][]string{{"logs"}, {"grep"}, {"errors"}} {
		if err := Run(context.Background(), args); err == nil {
			t.Errorf("Run(%v) = nil, want usage error", args)
		}
	}
	// grep's missing-pattern check runs after a successful dial, so a working
	// daemon must be reachable to exercise that branch rather than failing
	// earlier at dial.
	sock := startScriptedDaemon(t, nil)
	t.Setenv("PMMCP_IPC_ENDPOINT", sock)
	if err := Run(context.Background(), []string{"grep", "web"}); err == nil {
		t.Error("grep without pattern should error")
	}
}

func TestLogsDialError(t *testing.T) {
	t.Setenv("PMMCP_IPC_ENDPOINT", filepath.Join(t.TempDir(), "absent.sock"))
	if err := Run(context.Background(), []string{"logs", "web"}); err == nil {
		t.Fatal("want dial error")
	}
}

func TestLogsCallError(t *testing.T) {
	sock := startScriptedDaemon(t, map[string]scriptedResponse{
		api.MethodLogs: {errMsg: "boom", errCode: "internal"},
	})
	t.Setenv("PMMCP_IPC_ENDPOINT", sock)
	if err := Run(context.Background(), []string{"logs", "web"}); err == nil {
		t.Fatal("want call error")
	}
}

// TestLogsNoTrailingNewline covers logs' branch that appends a newline when
// the daemon's text does not already end in one.
func TestLogsNoTrailingNewline(t *testing.T) {
	b, _ := json.Marshal(api.LogsResult{Text: "no newline here"})
	sock := startScriptedDaemon(t, map[string]scriptedResponse{
		api.MethodLogs: {payload: b},
	})
	t.Setenv("PMMCP_IPC_ENDPOINT", sock)
	if err := Run(context.Background(), []string{"logs", "web"}); err != nil {
		t.Fatalf("logs: %v", err)
	}
}

// TestRunDeclareUsageError covers runDeclare's len(args)<1 branch.
func TestRunDeclareUsageError(t *testing.T) {
	t.Parallel()
	if err := Run(context.Background(), []string{"declare"}); err == nil {
		t.Fatal("want usage error")
	}
}

// TestEventsBranches covers events' dial error, call error, and non-empty
// iteration branch (the fake daemons elsewhere always return an empty list).
func TestEventsDialError(t *testing.T) {
	t.Setenv("PMMCP_IPC_ENDPOINT", filepath.Join(t.TempDir(), "absent.sock"))
	if err := Run(context.Background(), []string{"events"}); err == nil {
		t.Fatal("want dial error")
	}
}

func TestEventsCallError(t *testing.T) {
	sock := startScriptedDaemon(t, map[string]scriptedResponse{
		api.MethodEvents: {errMsg: "boom", errCode: "internal"},
	})
	t.Setenv("PMMCP_IPC_ENDPOINT", sock)
	if err := Run(context.Background(), []string{"events"}); err == nil {
		t.Fatal("want call error")
	}
}

func TestEventsPopulated(t *testing.T) {
	b, _ := json.Marshal([]api.EventView{{ID: "proc-1", Type: "started", Message: "ok"}})
	sock := startScriptedDaemon(t, map[string]scriptedResponse{
		api.MethodEvents: {payload: b},
	})
	t.Setenv("PMMCP_IPC_ENDPOINT", sock)
	if err := Run(context.Background(), []string{"events"}); err != nil {
		t.Fatalf("events: %v", err)
	}
}

// TestRunJobFlagErrors covers runJob's missing-value and unknown-flag
// branches.
func TestRunJobFlagErrors(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{
		{"run", "--name"},
		{"run", "--cwd"},
		{"run", "--bogus"},
	} {
		if err := Run(context.Background(), args); err == nil {
			t.Errorf("Run(%v) = nil, want error", args)
		}
	}
}

// TestStartDialAndCallErrors covers start's dial-error and call-error
// branches.
func TestStartDialError(t *testing.T) {
	t.Setenv("PMMCP_IPC_ENDPOINT", filepath.Join(t.TempDir(), "absent.sock"))
	if err := Run(context.Background(), []string{"start", "--name", "api", "--", "./bin/api"}); err == nil {
		t.Fatal("want dial error")
	}
}

func TestStartCallError(t *testing.T) {
	sock := startScriptedDaemon(t, map[string]scriptedResponse{
		api.MethodStart: {errMsg: "boom", errCode: "internal"},
	})
	t.Setenv("PMMCP_IPC_ENDPOINT", sock)
	if err := Run(context.Background(), []string{"start", "--name", "api", "--", "./bin/api"}); err == nil {
		t.Fatal("want call error")
	}
}

// TestPortsProcPrefix covers ports' proc- prefixed id branch (name branch is
// already covered by run_test.go).
func TestPortsProcPrefix(t *testing.T) {
	sock := startScriptedDaemon(t, nil)
	t.Setenv("PMMCP_IPC_ENDPOINT", sock)
	if err := Run(context.Background(), []string{"ports", "proc-1"}); err != nil {
		t.Fatalf("ports: %v", err)
	}
}

// TestBareSubcommandUsageErrors covers the len(args)<1 usage branch for every
// multi-verb command that requires a subcommand.
func TestBareSubcommandUsageErrors(t *testing.T) {
	t.Parallel()
	for _, cmd := range []string{"profile", "webhook", "session", "secret", "watch", "project"} {
		if err := Run(context.Background(), []string{cmd}); err == nil {
			t.Errorf("Run([%s]) = nil, want usage error", cmd)
		}
	}
}

// TestCallJSONBranches covers callJSON's dial-error, call-error, and
// out==nil branches.
func TestCallJSONDialError(t *testing.T) {
	t.Setenv("PMMCP_IPC_ENDPOINT", filepath.Join(t.TempDir(), "absent.sock"))
	if err := (&rootState{}).callJSON(context.Background(), api.MethodMetrics, nil); err == nil {
		t.Fatal("want dial error")
	}
}

func TestCallJSONCallError(t *testing.T) {
	sock := startScriptedDaemon(t, map[string]scriptedResponse{
		api.MethodMetrics: {errMsg: "boom", errCode: "internal"},
	})
	t.Setenv("PMMCP_IPC_ENDPOINT", sock)
	if err := (&rootState{}).callJSON(context.Background(), api.MethodMetrics, nil); err == nil {
		t.Fatal("want call error")
	}
}

func TestCallJSONNilResult(t *testing.T) {
	sock := startScriptedDaemon(t, map[string]scriptedResponse{
		api.MethodMetrics: {payload: nil},
	})
	t.Setenv("PMMCP_IPC_ENDPOINT", sock)
	if err := (&rootState{}).callJSON(context.Background(), api.MethodMetrics, nil); err != nil {
		t.Fatalf("callJSON: %v", err)
	}
}

// TestSecretSetStdinReadError covers secretSet's io.ReadAll error branch by
// swapping os.Stdin for an already-closed pipe end.
func TestSecretSetStdinReadError(t *testing.T) {
	sock := startScriptedDaemon(t, nil)
	t.Setenv("PMMCP_IPC_ENDPOINT", sock)
	withClosedStdin(t, func() {
		if err := (&rootState{}).secretSet(context.Background(), []string{"--name", "x"}); err == nil {
			t.Fatal("want read error from closed stdin")
		}
	})
}

// TestPayloadFromArgsRemainingBranches covers the branches of payloadFromArgs
// not already exercised via cli_internal_test.go: a trailing --json/-j with
// no following value, a bare flag with no value (or followed by another
// flag), a key:=malformed-json fallback to the raw string, and a bare
// positional token defaulting to name.
func TestPayloadFromArgsRemainingBranches(t *testing.T) {
	t.Parallel()
	if pl := payloadFromArgs([]string{"--json"}); len(pl) != 0 {
		t.Errorf("trailing --json with no value = %v, want empty", pl)
	}
	if pl := payloadFromArgs([]string{"--follow"}); pl["follow"] != true {
		t.Errorf("bare flag with no value = %v, want follow=true", pl)
	}
	if pl := payloadFromArgs([]string{"--follow", "--all"}); pl["follow"] != true {
		t.Errorf("flag followed by another flag = %v, want follow=true", pl)
	}
	if pl := payloadFromArgs([]string{"note:=not-json"}); pl["note"] != "not-json" {
		t.Errorf("note = %v, want raw string fallback", pl["note"])
	}
	if pl := payloadFromArgs([]string{"myproc"}); pl["name"] != "myproc" {
		t.Errorf("bare token = %v, want name=myproc", pl)
	}
}

// TestStartSandboxAndBareCommand covers start's --sandbox flag and running
// against a scripted daemon that reports success for api.MethodStart.
func TestStartSandboxAndBareCommand(t *testing.T) {
	sock := startScriptedDaemon(t, map[string]scriptedResponse{
		api.MethodStart: {payload: []byte(`{}`)},
	})
	t.Setenv("PMMCP_IPC_ENDPOINT", sock)
	if err := Run(context.Background(), []string{"start", "--name", "x", "--sandbox", "strict", "--", "cmd"}); err != nil {
		t.Fatalf("Run(start): %v", err)
	}
}

// TestRunSecretSetDispatch covers runSecret's "set" case, which is only
// exercised when reached through Run's/runSecret's own switch (calling
// secretSet directly, as other tests do, does not touch that switch arm).
func TestRunSecretSetDispatch(t *testing.T) {
	sock := startScriptedDaemon(t, nil)
	t.Setenv("PMMCP_IPC_ENDPOINT", sock)
	withStdin(t, "hunter2\n", func() {
		if err := Run(context.Background(), []string{"secret", "set", "--name", "db_password"}); err != nil {
			t.Errorf("Run(secret set): %v", err)
		}
	})
}

// TestRunMCPDispatch covers Run's "mcp" case by driving it through the top
// level dispatcher instead of calling runMCPSDK directly.
func TestRunMCPDispatch(t *testing.T) {
	withClosedStdin(t, func() {
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = w.Close() }()
		origOut := os.Stdout
		os.Stdout = w
		t.Cleanup(func() { os.Stdout = origOut })
		go func() {
			buf := make([]byte, 4096)
			for {
				if _, err := r.Read(buf); err != nil {
					return
				}
			}
		}()

		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		// No PMMCP_CONFIG override and an empty $HOME so loadCfg falls back to
		// in-process defaults (no on-disk daemon.toml to find or fail on).
		t.Setenv("PMMCP_CONFIG", "")
		t.Setenv("HOME", t.TempDir())
		if err := Run(ctx, []string{"mcp"}); err == nil {
			t.Log("Run(mcp) returned nil (session ended before timeout); acceptable")
		}
	})
}
