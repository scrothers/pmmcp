# agents: daemoncmd

## role
Thin cobra command tree for the **`pmmcpd`** binary. Keeps `cmd/pmmcpd` minimal: load config (flag > env > file > default), construct `daemon.Server`, serve until context cancel. No supervision logic lives here.

## surface
| Symbol / area | Notes |
|---------------|--------|
| `Execute(ctx)` | Entry from `cmd/pmmcpd`; propagates signal-cancel ctx |
| Root / `run` | Both start the daemon (`runDaemon`) |
| `version` | Prints `version.String()` |
| Persistent flags | `config.RegisterDaemonFlags` on root |

## deps
- Project: `config`, `daemon`, `version`
- Third-party: `github.com/spf13/cobra`

## invariants
- Binary stays thin — all control plane in `internal/daemon`.
- Context from `main` (signal.Notify) must reach `ListenAndServe`.
- Config precedence: flag > env > file > default (via viper/flags integration in config).
- No public network listeners configured here.

## tests
- `root_test.go` — command tree / version / config path wiring as present.
- Unit tests hermetic (`t.Parallel()` when safe). **Coverage floor: ≥80% statements** for this package (CI). Do not add production seams only to hit residual lines. Meaningful assertions beat line hits.

## touch map
- New daemon CLI flags → `config.RegisterDaemonFlags` + docs/configuration.md
- Serve lifecycle changes → `internal/daemon`, not this package

## do-not
- Put process managers, stores, or authz logic here.
- Auto-install OS services from this package (that is `pmmcp` client / `internal/service`).
- Swallow context cancel or detach from main’s signal handling.

## related
`internal/daemon`, `cmd/pmmcpd`, `docs/operations.md`
