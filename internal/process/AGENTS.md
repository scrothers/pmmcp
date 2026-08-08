# agents: process

## role
Parent package for process backends: **`Manager` interface**, start/handle types, and **`Router`** that dispatches local vs container by `StartSpec.Runtime` / `Image`. Imports **no** drivers; wiring lives in `process/drivers`.

## surface
| Symbol / area | Notes |
|---------------|--------|
| `Manager` | `Start` / `Stop` / `Wait` / `Inspect` / `Signal` — argv only, no shell |
| `StartSpec` | ID, Command, Cwd, Env, InheritEnv, LogDir, Sandbox, Runtime, Image, Ports, MemoryBytes |
| `Handle` | ID, PID (0 for container), ContainerID, Status, ExitCode |
| Sentinels | `ErrNotFound`, `ErrAlreadyExists`, `ErrInvalidSpec`, `ErrNotRunning`, `ErrSandboxFailed` |
| `Router` | `NewRouter(local, open)`; picks manager; tracks runtime per ID; `RuntimeOf` / `Forget` |

## deps
- Project: `internal/domain` (status on Handle)
- Third-party: none

## invariants
- Command is **argv list**; implementations must not wrap in a shell.
- Parent stays driver-free.
- Empty Runtime + empty Image → local; Image set without Runtime → container; aliases `podman`/`docker` → `container:<engine>`.
- `ErrSandboxFailed` is the fail-closed signal when restrictive sandbox cannot apply.

## tests
- `router_test.go` — local vs container pick, aliases, open errors, routing of Stop/Signal, Forget.
- Unit tests hermetic (`t.Parallel` when safe). **Coverage floor: ≥80% statements** for this package (CI). Do not add production seams only to hit residual lines. Meaningful assertions beat line hits.

## touch map
| Change | Also touch |
|--------|------------|
| `Manager` / `StartSpec` fields | `process/local`, `process/container`, `process/fake`, daemon start path, declare |
| Sentinel set | Drivers + daemon error mapping (`domain` codes) |
| Router runtime aliases | `process/drivers.Open`, docs |

## do-not
- Import `local` / `container` / engines from this package.
- Put supervision, restarts, or MCP handlers here.
- Accept shell strings as the primary command form.

## related
- `internal/process/{local,container,drivers,fake}`
- `internal/engine` — container backend API
