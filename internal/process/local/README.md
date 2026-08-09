# process/local

Default process driver: run and supervise **host OS processes** with argv-only execution, process-group / Job Object tree kill, optional filesystem sandboxing, log capture, and best-effort memory limits.

## Responsibilities

- Implement `process.Manager` for local children (`Start` / `Stop` / `Wait` / `Inspect` / `Signal`).
- Never wrap commands in a shell.
- Place children in a new process group (Unix) or Windows Job Object for reliable tree kill.
- Apply **strict** / **standard** sandbox profiles by rewriting argv to platform isolators:
 - **Linux:** bubblewrap (`bwrap`) via `internal/sandbox/linux`
 - **macOS:** `sandbox-exec` seatbelt via `internal/sandbox/darwin`
 - **Windows:** wrap is a no-op; Job Object provides kill-on-close / tree terminate (FS isolation is best-effort elsewhere)
- Capture stdout/stderr under `LogDir` with secret redaction (`logcap`).
- Soft memory limit via `RLIMIT_AS` / `prlimit` on Linux when `MemoryBytes` is set.
- Allow restart under the same process ID after a previous run has exited (auto-restart path).

## Non-responsibilities

- Container runtimes (see [`process/container`](../container/)).
- Defining sandbox **policy** contents (see [`internal/sandbox`](../../sandbox/)).
- Project/profile selection, groups, health checks (daemon / supervise).

## Usage

```go
mgr:= local.New
h, err:= mgr.Start(ctx, process.StartSpec{
 ID: "proc-01ARZ3NDEKTSV4RRFFQ69G5FAV",
 Command: []string{"node", "server.js"},
 Cwd: "/path/to/project",
 Env: []string{"PORT=3000"},
 LogDir: "/path/to/logs/proc-…",
 Sandbox: "strict",
 MemoryBytes: 512 << 20,
})
// h.PID is the OS process id

err = mgr.Stop(ctx, h.ID, 10*time.Second) // SIGTERM then SIGKILL / job terminate
```

Product code usually receives this manager through `process.Router` (daemon always installs `local.New` as the local backend).

## Start pipeline

1. Validate context, ID, and command (`domain.ValidateCommand`).
2. If an entry exists for ID and is **not** terminal → `ErrAlreadyExists`. If terminal → drop and allow reuse.
3. If sandbox is `strict` or `standard`, call platform `wrapSandbox(cmd, root, profile)`:
 - builds default policy from project root (`Cwd` or cwd)
 - returns rewritten argv (`bwrap … -- cmd…` or `sandbox-exec …`)
 - on failure → `process.ErrSandboxFailed` (fail closed)
4. Build `exec.Cmd` (no shell), set cwd/env, `Setpgid` / process group flags.
5. Optionally open log files and attach `logcap.RedactWriter`.
6. `cmd.Start`; on Windows assign Job Object (fail closed for strict/standard).
7. Apply post-start memory limit when configured.
8. Spawn reaper goroutine (`cmd.Wait` → status Exited + close logs/done).

## Stop pipeline

1. Mark `StatusStopping`.
2. Very short timeout (≤5ms): force kill immediately.
3. Otherwise: SIGTERM to process group (Unix) or process kill path (Windows).
4. On timeout or context cancel: force via Job Object terminate (Windows) or SIGKILL tree (Unix).
5. Wait for reaper (bounded).

Default stop timeout is **10 seconds** when the caller passes a non-positive duration.

## Sandbox matrix

| Profile | Linux | Darwin | Windows | Other GOOS |
|---------|-------|--------|---------|------------|
| `strict` / `standard` | bwrap required | sandbox-exec required | Job Object required for assignment | error |
| `permissive` / `off` / empty | no wrap | no wrap | job best-effort | no wrap |

See also [Security](https://github.com/scrothers/pmmcp/wiki/Security).

## Files (by concern)

| Area | Files |
|------|--------|
| Core lifecycle | `local.go` |
| Unix process group / kill | `process_unix.go` |
| Windows job + kill | `process_windows.go`, `job_stub.go` |
| Sandbox argv wrap | `wrap_linux.go`, `wrap_darwin.go`, `wrap_windows.go`, `wrap_other.go` |
| Memory limits | `rlimit_*.go` |
| Tests | `local_test.go`, `sandbox_*.go` |

## Errors

Parent sentinels: `ErrInvalidSpec`, `ErrAlreadyExists`, `ErrNotFound`, `ErrNotRunning`, `ErrSandboxFailed`. Operational failures wrap as `local: start|stop|signal|logcap: …`.

## How it fits

```text
process.Manager (interface)
 ↑
process/local.Manager ← default; sandbox + tree-kill
process/container.Manager
 ↑
process/drivers.Open → daemon Router
```

 gate (Linux): 
`PMMCP_REQUIRE_SANDBOX=1 go test./internal/process/local/ -count=1`

## Related

- argv-not-shell, drivers, sandbox MVP, multi-OS parity
- Features: the product design, `sandbox-*.md`
- Packages: [`sandbox`](../../sandbox/), [`logcap`](../../logcap/), [`process/drivers`](../drivers/)
