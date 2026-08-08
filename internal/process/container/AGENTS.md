# agents: process/container

## role
`process.Manager` for **container-backed** workloads. Depends only on `engine.Engine` (no docker/podman imports). Handles use PID=0 and expose engine container id on `Handle.ContainerID`.

## surface
| Symbol / area | Notes |
|---------------|--------|
| `Manager`, `New(eng)` | Nil engine panics; `var _ process.Manager` |
| Lifecycle | Start → `eng.Run`; Stop → `eng.Stop`; Wait/Inspect reconcile exit; Signal limited (not full POSIX) |
| `Logs(ctx, id, tail)` | Extra helper beyond core Manager (engine logs) |
| `buildRunSpec` | Maps StartSpec → `engine.RunSpec`; strict/standard hardening |
| Labels | `io.pmmcp.proc_id`, optional `io.pmmcp.name` for reconcile |
| `DefaultStopTimeout` | 10s |

## deps
- Project: `internal/process`, `internal/engine`, `internal/domain`
- Third-party: none

## invariants
- Image required; Command may be empty (image default entrypoint); non-empty Command validated via `domain.ValidateCommand` (argv).
- Strict/standard: `CapDrop=ALL`, read-only rootfs, `no-new-privileges`, Privileged false; reject env containing docker/podman socket paths; ports default **loopback** (`PublishAllInterfaces=false`).
- Permissive/off: may publish all interfaces.
- No direct import of engine drivers.

## tests
- `container_test.go` — lifecycle with race/fake engine, strict hardening, sock-env reject, wait/signal/logs edges.
- Unit tests hermetic (`t.Parallel()` when safe). **Coverage floor: ≥80% statements** for this package (CI). Do not add production seams only to hit residual lines. Meaningful assertions beat line hits.
- Real docker/podman: integration tier / `engine` tests — not required for unit green.

## do-not
- Import `engine/docker` or `engine/podman`.
- Host-network or privileged under strict/standard.
- Shell-inject container commands.

## related
- `internal/engine`, `internal/process/drivers`
