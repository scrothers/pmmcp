# Package config

## Overview

`config` loads daemon and client configuration once at process start. Files are **TOML only**. Missing path fields resolve to platform defaults; environment variables overlay selected fields. The default sandbox posture is **strict**.

## When to use

- `pmmcpd` and `pmmcp` startup: `config.Load(config.LoadOptions{Path: os.Getenv("PMMCP_CONFIG")})`
- Tests that need hermetic platform paths: pass `GOOS`, `Home`, `RuntimeDir`, `LookupEnv`, etc.
- Doctor/debug output: prefer `DoctorView()` or `Redacted()` over dumping raw `Config`

## Key types and functions

| Symbol | Purpose |
|--------|---------|
| `Config` | Root document: state dir, IPC endpoint, log, sandbox, process log limits, relaunch, token file |
| `Load` / `LoadOptions` | Read optional TOML, apply env, normalize defaults, validate |
| `Sandbox*` constants | `strict`, `standard`, `permissive`, `off` |
| `ErrInvalid` | Unsupported format or failed validation |
| `Redacted` / `String` / `DoctorView` | Safe display (token path redacted) |

### Environment overlays

| Variable | Field |
|----------|--------|
| `PMMCP_STATE_DIR` | `state_dir` |
| `PMMCP_IPC_ENDPOINT` | `ipc.endpoint` |
| `PMMCP_SANDBOX_DEFAULT` | `sandbox.default` |
| `PMMCP_TOKEN_FILE` | `token_file` |
| `PMMCP_LOG_LEVEL` | `log.level` |

Config file path for both binaries is typically `PMMCP_CONFIG` (handled by callers, not inside `Load` itself).

### Platform defaults (empty fields)

- **Linux**: state under XDG state home; IPC socket under XDG runtime dir (`…/pmmcp/pmmcpd.sock`)
- **macOS**: Application Support for state; temp runtime dir for socket
- **Windows**: LocalAppData for state; named pipe `\\.\pipe\pmmcpd-<user>`

## Design notes

- **Single load at start** — callers hold `*Config`; no global mutable config.
- **Strict by default** — empty sandbox.default becomes `strict`.
- **Process log limits** default to 50 MiB × 5 files when unset or non-positive.
- **Relaunch** defaults to enabled (boot relaunch of desired=running processes).
- **Token file** is a path to sensitive material; views always show `[redacted]`.

## Testing

```bash
go test ./internal/config/...
```

Tests inject `GOOS`/`Home`/`LookupEnv` so defaults are hermetic.

## Related packages

- [`cmd/pmmcpd`](../../cmd/pmmcpd/) — daemon loads config then starts `internal/daemon`
- [`internal/cli`](../cli/) — client loads config to dial IPC
- [`internal/daemon`](../daemon/) — consumes `*config.Config` for state dir, endpoint, sandbox default
- [`internal/doctor`](../doctor/) — endpoint health; CLI prints `DoctorView` first
