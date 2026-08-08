# engine

Container **engine interface** and shared CLI runner. Concrete backends live in subpackages; selection is in `engine/drivers`.

## Overview

`Engine` abstracts create/start, stop, and logs for a container runtime. The parent package defines the contract and `CLIRunner` (used by both Docker and Podman drivers to shell out to their CLIs safely). Unit tests use `engine/fake`. Production code should obtain an implementation via `engine/drivers.Open` (or a process-layer driver factory), not by importing docker/podman from mid-level packages unless probing availability.

## When to use

- Defining or consuming the `Engine` interface (e.g. `process/container`)
- Sharing CLI spawn logic (`CLIRunner`) inside a new driver
- Fakes in unit tests (`engine/fake`)

Use `engine/drivers` when you need “podman | docker | auto” by name.

## Key API

```go
type Engine interface {
 Name string
 Available(ctx context.Context) bool
 Run(ctx context.Context, spec RunSpec) (containerID string, err error)
 Stop(ctx context.Context, containerID string, timeout time.Duration) error
 Logs(ctx context.Context, containerID string, tail int) (string, error)
}

type RunSpec struct {
 Name, Image string
 Command []string
 Env map[string]string
 Ports []string // "host:container"
}
```

`CLIRunner` implements the same Run/Stop/Logs semantics against `Binary` (`docker` or `podman`), with injectable `LookPath` / `Command` for tests.

## Design notes

- Driver-subpackage pattern: parent = interface; `docker`/`podman`/`fake` = impls; `drivers` = only package that imports all.
- Auto selection prefers podman when available, else docker (`ErrNoneAvailable` if neither).
- Detached run uses `run -d --rm` plus optional `--name`, `-e`, `-p`.

## Testing

Parent: CLI runner edge cases without a real engine. Driver packages and `fake` own behavioral tests. Integration with real Podman lives under `test/integration`.

## Related packages

- [`engine/docker`](./docker) — Docker CLI driver
- [`engine/podman`](./podman) — Podman CLI driver
- [`engine/drivers`](./drivers) — `Open` / `OpenContext`
- [`engine/fake`](./fake) — in-memory engine for tests
- [`internal/process/container`](../process/container) — process Manager over Engine
