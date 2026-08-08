# process

Process backend interface, shared types, and runtime router.

## Overview

pmmcp runs workloads through a pluggable **`Manager`**:

| Backend | Package | Focus |
|---------|---------|-----------------|
| Local OS processes | `process/local` | exec, tree-kill, sandbox, rlimits, log files |
| Containers | `process/container` | via `engine.Engine` (podman/docker) |
| Tests | `process/fake` | in-memory, no OS spawn |

This **parent** package defines the interface and value types only. It does **not** import drivers. The cycle-safe selector is `process/drivers.Open`. Product code typically uses **`Router`**, which picks local vs container per `StartSpec.Runtime` / `Image`.

## When to use

- **Daemon wiring:** `process.NewRouter(local.New, drivers.Open)` as the default manager.
- **Tests:** inject `process/fake.New` or a custom `Manager`.
- **New backend:** implement `Manager` in a subpackage + one branch in `drivers.Open` — never edit the parent to import the driver.

## Key API

```go
type Manager interface {
 Start(ctx context.Context, spec StartSpec) (*Handle, error)
 Stop(ctx context.Context, id string, timeout time.Duration) error
 Wait(ctx context.Context, id string) (*Handle, error)
 Inspect(ctx context.Context, id string) (*Handle, error)
 Signal(ctx context.Context, id string, sig os.Signal) error
}

r:= process.NewRouter(local.New, drivers.Open)
h, err:= r.Start(ctx, process.StartSpec{
 ID: "proc-01AR…", Command: []string{"node", "server.js"}, LogDir: dir,
})
// container path:
h, err = r.Start(ctx, process.StartSpec{
 ID: id, Runtime: "container", Image: "alpine:3", Command: []string{"sleep", "1"},
})
_ = r.RuntimeOf(id)
r.Forget(id) // after remove
```

**`StartSpec` fields of note**

- `Command` — argv only (may be empty for container image entrypoint)
- `Runtime` / `Image` — backend selection
- `Sandbox` — profile name applied by local driver (strict/standard fail closed)
- `MemoryBytes` — best-effort soft limit
- `LogDir` — capturer directory for stdout/stderr

## Design notes

- **Driver-subpackage pattern.** Parent = interfaces + types; drivers = concrete `New`; `drivers` = only package that imports every driver.
- **Router is product-path glue** so Runtime/Image take effect without the parent knowing concrete types. `Open` is injected (`func(string) (Manager, error)`), usually `drivers.Open`.
- **Handles differ by backend:** local sets `PID`; container sets `ContainerID` and leaves PID 0.
- **Sentinel errors** are package-level for `errors.Is` across drivers.
- Supervision (restart, health, watch) stays above this package in daemon/supervise.

## Testing

```bash
go test./internal/process/...
go test./internal/process/local/...
go test./internal/process/container/...
go test./internal/process/drivers/...
go test./internal/process/fake/...
```

Parent `router_test.go` uses real `local` + `container` with `engine/fake`. Prefer `process/fake` for hermetic daemon unit tests.

## Related

- Process drivers, container engines, argv-not-shell, sandbox defaults
- Domain status: `internal/domain`
- Logs: `internal/logcap` (used by `local`)
- Engines: `internal/engine` (+ `engine/drivers`)
- Consumer: `internal/daemon`
