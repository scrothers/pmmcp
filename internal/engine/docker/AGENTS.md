# agents: engine/docker

## role
`engine.Engine` implementation via the **Docker CLI**. Available probes **daemon** (version), not binary alone. Functional options for `DOCKER_HOST`.

## surface
| Symbol / area | Notes |
|---------------|--------|
| `Engine`, `New(opts...)` | Wraps shared `engine.CLIRunner` with binary `docker` |
| `WithHost(host)` | Sets DOCKER_HOST env for every CLI call |
| Full cap set | Run/Stop/Logs + Inspect/Wait/Remove/PullImage/List/Version |
| Compile-time anchors | `var _ engine.Engine` and optional interfaces |

## deps
- Project: `internal/engine` only
- Third-party: none

## invariants
- Available = CLI on PATH **and** daemon answers version — avoids selecting docker when daemon is down.
- Missing docker → `engine.ErrUnavailable`; unknown container → `engine.ErrNotFound`.
- No shell; no direct socket client API in this package.
- Does not import sibling `podman` or selector.

## tests
- `docker_test.go` — helper-process fake CLI for Available/Run/Stop/Logs/Inspect/etc.; `export_test.go` injects CLI.
- Unit tests hermetic (`t.Parallel()` when safe). **Coverage floor: ≥80% statements** for this package (CI). Do not add production seams only to hit residual lines. Meaningful assertions beat line hits.
- Live daemon not required for unit green.

## do-not
- Import `engine/podman` or `engine/drivers`.
- Treat binary presence alone as Available.
- Log env secrets from CLI output.

## related
- `internal/engine`, `internal/engine/drivers`
