# agents: process/local

## role
OS process `process.Manager`: **exec argv only (no shell)**, process-group / Job Object tree kill, log capture hooks, env inheritance, memory soft limits, and **multi-OS sandbox wrap** (Linux bwrap, Darwin sandbox-exec, Windows strict fail-closed).

## surface
| Symbol / area | Notes |
|---------------|--------|
| `Manager`, `New()` | In-memory map of live procs; `var _ process.Manager` |
| `Start` / `Stop` / `Wait` / `Inspect` / `Signal` | Graceful SIGTERM → force; default stop 10s |
| `DefaultStopTimeout` | 10s |
| `wrapSandbox` (per-OS) | Linux: bwrap; Darwin: sandbox-exec; Windows: strict → `ErrSandboxFailed` |
| `buildChildEnv` | `InheritEnv`: minimal (default) / full / none — minimal avoids leaking daemon secrets |
| Job Objects (Windows) | Tree terminate; fail closed for strict/standard if assign fails |
| rlimits (Unix) | `MemoryBytes` soft limit |
| Low-integrity token (Windows) | Optional for non-strict profiles — not FS isolation |

## deps
- Project: `internal/process`, `internal/domain`, `internal/sandbox`, `internal/sandbox/{linux,darwin}` (via wrap)
- Third-party: `golang.org/x/sys` (unix/windows) as needed for platform files

## invariants
- **Argv, never implicit shell** — `exec.Command` with path + args only.
- Restrictive sandbox (`strict`/`standard`): fail closed when isolation tool missing or Windows strict (`process.ErrSandboxFailed`).
- Default env inheritance is **minimal** (PATH/HOME/LANG/TMP*… only) so ambient secrets do not leak into children.
- Unix: new process group; Stop tree-kills. Windows: Job Object when assigned.
- Do not log full env maps or resolved secrets.
- Sandbox wrap rewrites argv in place before start; does not claim isolation without the OS tool.

## tests
- `local_test.go`, `local_extra_test.go`, `local_internal_test.go` — lifecycle, no-shell, env, stop escalation.
- `sandbox_test.go` (Linux bwrap), `sandbox_darwin_test.go`, `sandbox_windows_test.go`.
- `wrap_linux_internal_test.go` — unknown profile.
- Unit tests hermetic where possible (`t.Parallel()` when safe). **Coverage floor: ≥80% statements** for this package (CI). Do not add production seams only to hit residual lines. Meaningful assertions beat line hits.
- Host isolation gate: `PMMCP_REQUIRE_SANDBOX=1 go test ./internal/process/local/` — hard-fail if bwrap/sandbox-exec unavailable instead of skip.

## touch map
| Change | Also touch |
|--------|------------|
| Sandbox wrap / fail-closed | `internal/sandbox/*`, SECURITY docs |
| Env inheritance keys | Security review — secret leak surface |
| Stop / signal semantics | Daemon supervise loops, CLI stop |
| Job / rlimit platform code | Windows CI; `//go:build` files |

## do-not
- Shell-wrap commands (`sh -c`) unless caller already passed explicit argv.
- Soften strict when bwrap/sandbox-exec missing.
- Claim Windows FS isolation for local strict.
- Inherit full daemon env by default.

## related
- `internal/sandbox/{linux,darwin,windows}`
- `internal/process` / `process/drivers`
