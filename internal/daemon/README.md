# daemon

Long-lived **pmmcpd control plane**: owns process state, private IPC, and the full method surface used by CLI/MCP clients.

## Overview

`Server` opens SQLite under `cfg.StateDir`, builds a process `Router` (local + container drivers), and serves **gRPC over a private endpoint** (Unix domain socket or Windows named pipe). Unary RPCs map to JSON method dispatch (`handle`); log/event follow use server-streaming RPCs. Background loops handle auto-restart and file watches. Optional boot relaunch restarts records with `desired=running`.

## When to use

- **Entry point only** from `cmd/pmmcpd` (or integration/e2e tests that boot a real daemon).
- Inject `Options.Manager` / `DBPath` in unit tests instead of a live host process tree.
- Do **not** call daemon from leaf libraries; clients use `internal/ipc` against a running daemon.

## Key API

| Symbol | Purpose |
|--------|---------|
| `New(ctx, Options)` | Open store, migrate, wire manager/registries/keyring |
| `ListenAndServe(ctx)` | Listen (peer-cred filtered), optional relaunch, start supervise/watch loops, gRPC serve until cancel |
| `RegisterGRPC` | Attach `Call` / `SubscribeLogs` / `SubscribeEvents` |
| `RelaunchEligible` | Start store rows eligible for boot relaunch |
| `Close` | Close listener + store |

**Method groups** (via `handle` / `dispatchExtra`, each gated by authz capabilities):

- **Core:** hello, whoami, daemon.info/reload, project.current/list, start/stop/restart/list/status/remove, logs/grep/errors, events, audit
- **Lifecycle extras:** run, wait, enable/disable, update, health.check
- **Groups / profiles / session / share**
- **Declare:** validate, diff, apply, show, import
- **Runtime/secrets/watch/webhooks/metrics/logs export & subscribe**

## Design notes

- Transport and auth: (gRPC IPC), (same-UID + role packs). Peer UID is enforced on accept; roles apply on top.
- Control plane is assembler-heavy: domain purity stays in `domain`; persistence in `store`; spawn in `process`; declare parsing in `declare`.
- `dispatchExtra` holds product-path methods so the core switch stays readable.
- Stream RPCs cap duration (default 30s, max 5m) for backpressure.
- Daemon reload re-reads a safe config subset (e.g. log level, sandbox default for new starts).

## Testing

- Package tests use temp DB paths and fake/injected managers where possible.
- Product and parity tests exercise large method surfaces without full e2e.
- Real IPC/lifecycle: `test/integration`, `test/e2e`.

## Related packages

- [`internal/domain`](../domain) — process values, status, error codes
- [`internal/process`](../process) — Manager / Router
- [`internal/store`](../store) — ProcessStore
- [`internal/declare`](../declare) — pmmcp.yaml parse/diff
- [`internal/ipc`](../ipc) — client dial / listener
- [`internal/api`](../api) — method names and payloads
- [`cmd/pmmcpd`](../../cmd/pmmcpd) — binary entry
