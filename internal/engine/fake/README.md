# engine/fake

In-memory implementation of `engine.Engine` for unit tests.

## Overview

The fake engine records successful `Run` specs, assigns synthetic container IDs (`fake-1`, `fake-2`, …), stores stub log text, and supports injected failures. It never looks for Podman or Docker and never touches the host network or filesystem beyond process memory.

## When to use

- Unit tests for `process/container`, routers, or any code that depends on `engine.Engine`
- Failure-injection scenarios (`RunErr`, `StopErr`, custom `AvailableFunc`)

Prefer real engines only in integration/e2e tests (for example `test/integration` with Podman). Do not wire this package into production selector or daemon startup.

## Key API

```go
e := fake.New()
id, err := e.Run(ctx, engine.RunSpec{Image: "postgres:16", Ports: []string{"5432:5432"}})
logs, err := e.Logs(ctx, id, 10)
err = e.Stop(ctx, id, time.Second)

// Assertions / injection
_ = e.Runs              // copy of specs that passed validation
e.RunErr = errors.New("boom")
e.AvailableFunc = func(context.Context) bool { return false }
```

| Method | Behavior |
|--------|----------|
| `Name()` | always `"fake"` |
| `Available` | `true` if ctx OK, or `AvailableFunc` when set |
| `Run` | requires non-empty `Image`; records spec; returns `fake-<n>` |
| `Stop` | marks container stopped; unknown ID → `engine.ErrNotFound` |
| `Logs` | returns stored stub string; unknown ID → `engine.ErrNotFound` |

## Design notes

- Implements the full `engine.Engine` surface so production container managers can be unit-tested without a daemon.
- Compile-time check: `var _ engine.Engine = (*Engine)(nil)`.
- Mutex-protected map of containers; sequence counter is atomic so concurrent tests on one instance stay consistent.
- Intentionally **not** part of `engine/drivers.Open` — tests import and construct it explicitly, keeping production wiring free of test types.

## Testing

```bash
go test ./internal/engine/fake/
```

Covers happy-path Run/Stop/Logs, empty-image `ErrInvalidSpec`, and missing-ID `ErrNotFound`.

## Related

- [`../`](../) — `engine.Engine` interface and sentinels
- [`../drivers/`](../drivers/) — production selector (does not include fake)
- [`../podman/`](../podman/), [`../docker/`](../docker/) — real CLI engines
- [`../../process/container/`](../../process/container/) — primary consumer of engines in process layer
