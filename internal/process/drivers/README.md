# process/drivers

Selector (assembler) for process backends. This package is the single place that imports every process driver and wires an `engine.Engine` into the container manager.

## Responsibilities

- Map a runtime name string to a concrete `process.Manager`.
- Keep the dependency graph acyclic: parent `process` defines the interface; drivers implement it; only `drivers` imports both.
- Explicit construction — no package `init` registration tables.

## Supported names

| Name | Backend |
|------|---------|
| `local` or empty | Host OS processes via [`process/local`](../local/) |
| `container` / `container:auto` | [`process/container`](../container/) with engine auto-detect |
| `container:podman` | Container manager + Podman engine |
| `container:docker` | Container manager + Docker engine |

Unknown names return `drivers.ErrUnknown`.

## Usage

```go
mgr, err:= drivers.Open("local")
// …

// Product composition (daemon):
localMgr:= local.New
router:= process.NewRouter(localMgr, drivers.Open)
// StartSpec.Runtime / Image selects backend through Router → Open
```

```go
// Explicit container engine:
mgr, err:= drivers.Open("container:podman")
```

## How it fits

House **driver-subpackage** pattern (same shape as `internal/engine/drivers`):

1. **Parent** `internal/process` — `Manager`, `StartSpec`, `Handle`, sentinels. Imports no driver.
2. **Drivers** `local/`, `container/`, `fake/` — each `New` + compile-time interface check.
3. **Selector** `process/drivers` — only package that imports parent + product drivers.

`fake` is intentionally **not** registered in `Open` (test-only; construct with `fake.New`).

Daemon wiring (`internal/daemon.New`):

```text
local.New → process.NewRouter(local, drivers.Open) → Server.mgr
```

Per-start, `Router` picks `local` or calls `Open("container…")` when `StartSpec.Runtime` / `Image` demands it.

## Design notes

- **One manager instance per Open call.** Callers that need long-lived routing (the daemon) hold a `Router` that remembers which manager owns each process ID.
- **Engine selection is nested.** Process driver `container` chooses the process abstraction; `engine/drivers.Open` chooses Podman vs Docker. Do not collapse these into one string space without updating both packages.
- **** forbids editing the parent interface to favor one backend; extend drivers + selector only.

## Testing

```bash
go test./internal/process/drivers/
```

Unit tests cover local open and unknown-name errors. Opening container variants requires a usable engine on the host (covered in integration tests elsewhere).

## Related

- process drivers, container engines
- the product design
- Sibling: [`internal/engine/drivers`](../../engine/drivers/)
