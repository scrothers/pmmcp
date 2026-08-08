# engine/drivers

Selector package for container engines. Assembles `engine.Engine` implementations by name without putting driver imports in the parent `engine` package.

## Overview

`internal/engine` defines the `Engine` interface and shared types. Concrete backends live in sibling packages (`podman`, `docker`). This package is the **only** place that imports both the parent interface and every driver, following the house driver-subpackage pattern.

Callers open an engine by string name:

| Name | Behavior |
|------|----------|
| `podman` | `podman.New` |
| `docker` | `docker.New` |
| `auto` or `""` | Prefer Podman if available, else Docker |

Auto detection records the choice via the returned engine’s `Name` so status and logs never hide which backend was selected.

## When to use

- Production wiring that needs an `engine.Engine` from config (`engine: podman|docker|auto`).
- Higher-level selectors such as `process/drivers` when opening `container`, `container:podman`, or `container:docker` managers.

Do **not** use this package from unit tests that need a hermetic engine — construct `engine/fake.New` directly.

## Key API

```go
eng, err:= drivers.Open("auto")
// or with a cancelable probe context:
eng, err:= drivers.OpenContext(ctx, "podman")
```

Sentinels:

- `ErrUnknown` — name not in the switch
- `ErrNoneAvailable` — auto mode and neither binary is on PATH

Named opens (`podman`, `docker`) always return a constructed engine; they do not fail just because the binary is missing. Callers that care should call `eng.Available(ctx)` or treat later `Run`/`Stop`/`Logs` errors (`engine.ErrUnavailable`).

## Design notes

- **Explicit wiring, no `init`.** Adding a backend means a new driver package plus one case in `Open`/`OpenContext` (and the auto branch if it participates in discovery).
- **Cycle safety.** Parent `engine` must never import drivers. Anything that needs “give me an engine” imports this selector instead.
- **Auto policy.** Prefer Podman when `Available`, else Docker. When both exist, Podman wins and `Name` is `"podman"`. There is no silent “guess” without a recorded name.
- **Probing.** Auto uses each driver’s `Available(ctx)` (PATH look for the CLI today). Prefer `OpenContext` when the caller already has a request or shutdown context.

## Testing

Unit tests only (`go test./internal/engine/drivers/`):

- Named open returns engines with matching `Name`
- Unknown name → `errors.Is(err, ErrUnknown)`
- Auto → either a podman/docker engine or `ErrNoneAvailable` on bare hosts

Real CLI/daemon coverage lives under integration tests for the podman/docker packages, not here.

## Related

- [`../`](../) — `engine.Engine`, `RunSpec`, sentinels, shared `CLIRunner`
- [`../podman/`](../podman/) — Podman CLI driver
- [`../docker/`](../docker/) — Docker CLI driver
- [`../fake/`](../fake/) — in-memory test double (not selected here)
- [`../../process/drivers/`](../../process/drivers/) — process-manager selector that calls this package
- (container engines), (process drivers)
