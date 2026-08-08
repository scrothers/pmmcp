# engine/docker

Docker-backed implementation of [`engine.Engine`](../engine.go).

## Overview

Thin driver: `New` wraps `engine.CLIRunner{Binary: "docker"}` and delegates `Available` / `Run` / `Stop` / `Logs`. The MVP shells out to the `docker` CLI with discrete argv (no shell). When `docker` is not on `PATH`, operations fail with `engine.ErrUnavailable`.

## When to use

- Explicit Docker runtime: `engine/drivers.Open("docker")` or process specs that select docker
- Runtime probes (e.g. daemon `runtime.info` availability map)
- Prefer **podman** via auto when both exist (`drivers` prefers podman)

Not for: unit tests that should stay hermetic — use `engine/fake`.

## Key API

```go
e := docker.New()
e.Name() // "docker"
id, err := e.Run(ctx, engine.RunSpec{Image: "postgres:16", Ports: []string{"5432:5432"}})
_ = e.Stop(ctx, id, 10*time.Second)
logs, err := e.Logs(ctx, id, 100)
```

## Design notes

- Same CLI shape as podman driver so behavior stays aligned; only the binary name differs.
- `--rm` detached containers: lifecycle after stop is engine-defined cleanup.
- Parent package must not import this package; only `engine/drivers` (and rare probes) should.

## Testing

Unit: name + invalid spec without Docker installed. Real engine coverage is integration/e2e when Docker is present on the host.

## Related packages

- [`internal/engine`](../) — interface + CLIRunner
- [`internal/engine/podman`](../podman) — sibling driver
- [`internal/engine/drivers`](../drivers) — selector (`Open("docker")`)
- [`internal/process/container`](../../process/container) — Manager over Engine
