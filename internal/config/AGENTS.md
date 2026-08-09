# agents: config

## role
Load daemon (and client-shared) configuration once at process start: file (TOML/YAML/JSON via viper), `PMMCP_*` env overlays, CLI flag bindings, then platform defaults. Default sandbox posture is strict. Views redact token paths.

## surface
| Symbol / area | Notes |
|---------------|--------|
| `Config` | `Version`, `StateDir`, `IPC`, `Log`, `Sandbox`, `Logs`, `Relaunch`, `Webhook`, `TokenFile` |
| Nested | `IPCConfig`, `LogConfig`, `SandboxConfig`, `ProcessLogs`, `RelaunchConfig`, `WebhookConfig` |
| Sandbox consts | `strict`, `standard`, `permissive`, `off` |
| `LoadOptions` | Path, GOOS, Home, RuntimeDir, StateHome, ConfigHome, LookupEnv, Username, Flags |
| `RegisterDaemonFlags` | defines override flags; bound in `Load` when `Flags` set |
| `Load` / `DefaultPath` | flag > env > file > default; documented config search order |
| `ErrInvalid` | malformed / unsupported version / bad sandbox |
| `Redacted` / `String` / `DoctorView` | safe dumps (`token_file` → `[redacted]`) |

## deps
- Project: none
- Third-party: `github.com/spf13/viper`, `github.com/spf13/pflag`, `github.com/go-viper/mapstructure/v2`

## invariants
- Precedence: bound flag > env (`PMMCP_` + dotted→underscore) > file > defaults
- Strict unmarshal (`ErrorUnused`) — unknown keys fail load
- Empty `state_dir` / `ipc.endpoint` filled from platform defaults (XDG / Application Support / LocalAppData + named pipe)
- Default sandbox is `strict`; invalid sandbox fails validation
- `[ipc].token_file` folds into `TokenFile`; legacy top-level key exists; non-empty `PMMCP_TOKEN_FILE` still wins
- Webhook allowlist empty by default (webhooks off) — SSRF-safe default
- No package-level mutable config singleton; load at startup only

## tests
- `config_test.go`, `config_platform_test.go`, `config_search_test.go`, `config_invalid_test.go`, `config_coverage_test.go`, helpers/export tests — defaults, overlays, search order, redaction, invalid keys
- Unit tests hermetic (`t.Parallel()` when safe). **Coverage floor: ≥80% statements** for this package (CI). Do not add production seams only to hit residual lines. Meaningful assertions beat line hits.

## touch map
- `cmd/pmmcpd` / `cmd/pmmcp` — flags + Load
- Doctor and logging use Redacted/DoctorView; webhook registry consumes allowlist

## do-not
- Do not print raw token file contents or paths in doctor/string views
- Do not change default sandbox away from `strict` without product decision
- Do not scatter bare `os.Getenv` for config values outside Load/LoadOptions seams
- Do not bind a public network control plane via “convenience” defaults
- Do not auto-start the daemon from config load

## related
- `internal/doctor`, `internal/webhook`, `internal/sandbox`, `https://github.com/scrothers/pmmcp/wiki/Configuration`
