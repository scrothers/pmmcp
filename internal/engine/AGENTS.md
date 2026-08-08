# agents: engine

## role
Parent package for **container engines**: core `Engine` interface, shared `RunSpec` / status types, optional capability interfaces, and shared **CLIRunner** that shells out to docker/podman CLIs with argv-only tokens (no shell). Drivers live in subpackages; selector is `engine/drivers`.

## surface
| Symbol / area | Notes |
|---------------|--------|
| `Engine` | `Name`, `Available`, `Run`, `Stop`, `Logs` |
| Optional caps | `Inspector`, `Waiter`, `Remover`, `ImagePuller`, `Lister`, `Versioner` |
| `RunSpec` | Image, Command, Env, Ports, Labels, CapDrop/Add, SecurityOpt, ReadOnlyRootfs, Privileged, Volumes, PublishAllInterfaces, NoRemove |
| `Status`, `State`, `Container`, `VersionInfo` | Inspect/list models |
| `CLIRunner` | Shared run/stop/logs/inspect/wait/remove/pull/list/version via exec argv |
| `runArgs` | Deterministic argv; loopback port default when not PublishAllInterfaces |
| Sentinels | `ErrUnavailable`, `ErrNotFound`, `ErrInvalid` (and related) |

## deps
- Project: none from drivers (parent is driver-free)
- Third-party: none in parent (stdlib `os/exec`)

## invariants
- No shell: every CLI arg is a distinct argv element.
- Ports without host IP bind **127.0.0.1** unless `PublishAllInterfaces`.
- Parent imports no docker/podman packages.
- Strict callers (process/container) set CapDrop ALL, read-only rootfs, no-new-privileges.

## tests
- `cli_test.go`, `cli_ops_test.go` — argv hardening, helper-process fake CLI, available/cancel paths.
- Unit tests hermetic (`t.Parallel` when safe). **Coverage floor: ≥80% statements** for this package (CI). Do not add production seams only to hit residual lines. Meaningful assertions beat line hits.
- Live docker/podman optional outside unit suite.

## do-not
- Import drivers into parent.
- Default-publish ports on all interfaces for strict workloads.
- Use `sh -c` for CLI invocations.

## related
- `internal/engine/{docker,podman,drivers,fake}`
- `internal/process/container`
