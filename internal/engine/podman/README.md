# engine/podman

Podman-backed `engine.Engine` implementation.

## Overview

This package runs containers by shelling out to the `podman` CLI with fixed argv (no shell). Shared run/stop/logs argument construction lives in the parent package’s `engine.CLIRunner`; this driver only sets `Binary: "podman"` and exposes the standard `Engine` methods.

Rootless Podman typically uses `$XDG_RUNTIME_DIR/podman/podman.sock`. The CLI discovers that socket; this package does **not** open the REST API socket directly.

## When to use

- Production container workloads preferring Podman (default in auto mode when available).
- Direct construction when code must pin Podman: `podman.New`, or via `engine/drivers.Open("podman")`.
- Daemon doctor / capability probes (`Available`).

For unit tests, use `engine/fake` instead of this package.

## Key API

```go
eng:= podman.New
if !eng.Available(ctx) {
 // podman not on PATH
}
id, err:= eng.Run(ctx, engine.RunSpec{
 Name: "db",
 Image: "postgres:16",
 Env: map[string]string{"POSTGRES_PASSWORD": "x"},
 Ports: []string{"5432:5432"},
})
_ = eng.Stop(ctx, id, 10*time.Second)
out, _:= eng.Logs(ctx, id, 100)
```

| Method | Notes |
|--------|--------|
| `Name` | `"podman"` |
| `Available(ctx)` | `exec.LookPath("podman")` (via CLIRunner) |
| `Run` | detached `podman run -d --rm …`; returns container ID |
| `Stop` | stop with timeout; force path handled by CLIRunner |
| `Logs` | recent log tail |

## Design notes

- **Driver-subpackage pattern.** Parent `engine` holds the interface; this package is one concrete backend. Type is named `Engine` (not `PodmanEngine`) so it reads as `podman.Engine`.
- **CLI over socket (MVP).** Prefer API later if needed; keep argv-safe if CLI remains.
- **Sentinels.** Missing binary → `engine.ErrUnavailable`; empty image → `engine.ErrInvalidSpec`. Callers should use `errors.Is`.
- **Parity with Docker driver.** Behavior should stay aligned via shared `CLIRunner` so process/container code does not branch on engine name for basic lifecycle.
- **Auto selection.** `engine/drivers` prefers this engine when `Available` returns true.

## Testing

```bash
go test./internal/engine/podman/
```

Unit tests assert naming, invalid-spec errors, and non-panic error returns when the CLI or daemon is missing. Real run/stop against a live Podman belongs under integration tests (`test/integration`, build tag `integration`).

## Related

- [`../`](../) — interface, `RunSpec`, `CLIRunner`, sentinels
- [`../docker/`](../docker/) — Docker CLI sibling
- [`../drivers/`](../drivers/) — name selector + auto preference for Podman
- [`../fake/`](../fake/) — unit-test double
- [`../../../test/integration/`](../../../test/integration/) — live Podman coverage
- (container engines), (argv not shell)
