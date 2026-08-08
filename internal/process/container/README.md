# process/container

Container-backed implementation of [`process.Manager`](../manager.go). Workloads are created and stopped through an [`engine.Engine`](../../engine/engine.go) (Podman, Docker, or a test fake). This package never imports a concrete engine driver.

## Responsibilities

- Implement full `process.Manager` lifecycle for container runtimes.
- Map `process.StartSpec` fields (`Image`, `Command`, `Env`, `Ports`, `Name`) onto `engine.RunSpec`.
- Track process IDs (`proc-…`) to engine container IDs in memory.
- Expose container identity on `process.Handle` (`PID=0`, `ContainerID` set).
- Fail closed on strict/standard sandbox when env values reference `docker.sock` / `podman.sock`.
- Provide `Logs` as a package helper for higher layers that already know they hold a container manager.

## Non-responsibilities

- Selecting Podman vs Docker (see [`process/drivers`](../drivers/) and [`engine/drivers`](../../engine/drivers/)).
- Host OS process groups, bubblewrap, seatbelt, or Job Objects (that is [`process/local`](../local/)).
- Supervision, restart policy, groups, or project scope (daemon / supervise layers).
- Durable re-attach after daemon restart (labels / store are above this package).

## Usage

```go
eng, err:= enginedrivers.Open("auto") // or "podman" / "docker" / fake
if err != nil {
 return err
}
mgr:= container.New(eng)

h, err:= mgr.Start(ctx, process.StartSpec{
 ID: "proc-01ARZ3NDEKTSV4RRFFQ69G5FAV",
 Name: "db",
 Image: "postgres:16",
 Env: []string{"POSTGRES_PASSWORD=secret"},
 Ports: []string{"5432:5432"},
 // Command optional: empty uses image entrypoint
})
// h.PID == 0; h.ContainerID is engine id

_ = mgr.Stop(ctx, h.ID, 10*time.Second)
```

Product path usually goes through `process.Router` + `process/drivers.Open` rather than constructing this type in handlers.

## Lifecycle

| Method | Behavior |
|--------|----------|
| **Start** | Validate ID + image; optional command validation; refuse sock-like env under strict/standard; `eng.Run`; record entry as Running. |
| **Stop** | Mark Stopping; `eng.Stop` with timeout (default 10s); set exit code 0, Exited, unblock Waiters. |
| **Wait** | Block until Stop completed (or ctx cancel). Does not poll engine for natural exit. |
| **Inspect** | Snapshot handle (ID, ContainerID, status, exit code). |
| **Signal** | Returns a fixed “not supported” error while running. |
| **Logs** | Delegates to `eng.Logs` for the tracked container id. |

## Errors

Uses parent sentinels from `internal/process`:

- `ErrInvalidSpec` — empty ID/image, bad command
- `ErrAlreadyExists` — same process ID still registered
- `ErrNotFound` / `ErrNotRunning`
- `ErrSandboxFailed` — strict/standard sock heuristic

Engine failures are wrapped: `process/container: run|stop: …`.

## How it fits

```
CLI/MCP → daemon → process.Router
 ├─ local Manager (OS children)
 └─ drivers.Open("container…")
 → engine/drivers.Open
 → process/container.New(engine)
```

 (process drivers) and (container engines). Feature notes: the product design.

## Testing

- Unit: `go test./internal/process/container/` with `internal/engine/fake`.
- Integration: `go test -tags=integration./test/integration/` (Podman path when available).

## Related packages

| Package | Role |
|---------|------|
| [`internal/process`](../) | `Manager` interface, `StartSpec`, `Handle`, errors |
| [`internal/process/drivers`](../drivers/) | Name → Manager selector |
| [`internal/engine`](../../engine/) | Engine interface + RunSpec |
| [`internal/process/local`](../local/) | Sibling local OS driver |
