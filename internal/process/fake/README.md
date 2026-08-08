# process/fake

In-memory [`process.Manager`](../manager.go) for unit tests. It validates start specs and tracks lifecycle state without spawning OS processes or containers.

## When to use

- Testing code that depends on `process.Manager` (handlers, routers, supervision loops) and must stay hermetic.
- Asserting which `StartSpec` values were passed (`Manager.Starts`).
- Avoiding sandbox binaries (bwrap / sandbox-exec) and engine daemons in the default `go test ./...` gate.

Prefer the real [`local`](../local/) or [`container`](../container/) drivers for integration and e2e tests.

## Usage

```go
mgr := fake.New()
h, err := mgr.Start(ctx, process.StartSpec{
    ID:      "proc-01ARZ3NDEKTSV4RRFFQ69G5FAV",
    Command: []string{"sleep", "30"}, // validated, not executed
})
// h.PID is synthetic (>= 1001), Status Running

_ = mgr.Stop(ctx, h.ID, time.Second)
// mgr.Starts holds every StartSpec seen
```

Daemon override pattern:

```go
srv, err := daemon.New(ctx, daemon.Options{
    Config:  cfg,
    Manager: fake.New(), // replaces Router product path when set
})
```

## Behavior summary

| Method | Behavior |
|--------|----------|
| **Start** | Require non-empty ID and valid argv; reject duplicates; record spec; return Running handle with synthetic PID. |
| **Stop** | Mark Exited with exit code 0; unblock Wait. Idempotent if already terminal. |
| **Wait** | Return immediately if terminal; else block until Stop or context cancel. |
| **Inspect** | Snapshot handle (copies exit code pointer). |
| **Signal** | No-op success while running; errors if missing or terminal. |

## Differences from local

| Concern | fake | local |
|---------|------|-------|
| Exec | never | `exec.Command` argv |
| PID | counter | OS PID |
| Same ID after exit | still “exists” | may restart after terminal |
| Sandbox / logs / rlimit | none | full |
| `Starts` history | yes | no |

## Wiring

Not part of [`process/drivers.Open`](../drivers/). Construct with `fake.New()` in tests only. Production always uses `local` / `container` through the router.

## Testing

```bash
go test ./internal/process/fake/
```

Package tests cover start/inspect/stop/wait, validation errors, and signal edge cases.

## Related

- Parent interface: [`internal/process`](../)
- Real drivers: [`local`](../local/), [`container`](../container/)
- Engine test double: [`internal/engine/fake`](../../engine/fake/) (different interface)
