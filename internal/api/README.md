# Package api

## Overview

`api` is the shared private IPC contract between the `pmmcp` client and the `pmmcpd` daemon. It defines the framed request/response envelope, the complete set of method name constants, and the JSON payload and result types used on those methods.

This package is **not** generated gRPC code. Wire-level protobuf/gRPC types live in [`gen/pmmcp/v1`](gen/pmmcp/v1/).

## When to use

- Implementing or calling an IPC method on either side of the private control plane
- Adding a new method constant, payload, or result shape
- Asserting catalog parity between IPC methods and MCP tools (`cli.ToolMethod` / `api.AllMethods`)

Prefer importing this package for method names and DTO types rather than duplicating string literals.

## Key types and functions

| Symbol | Purpose |
|--------|---------|
| `APIVersion` | IPC major.minor string (`"1.0"`) |
| `Request` / `Response` | Framed call: method, session, role, payload; OK/error/retryable/payload |
| `Method*` constants | Method names (e.g. `process.start`, `logs.tail`, `declare.apply`) |
| `AllMethods` | Slice of every method name (parity tests) |
| `StartPayload` / `StartResult` | Process start |
| `IDPayload` | Identify process by id/name (stop, status, remove, …) |
| `ListPayload` / `ProcessView` | List/status filtering and views |
| `LogsPayload` / `LogsResult` | Log tail/grep/errors |
| `HelloResult`, `DaemonInfoResult`, `WhoamiResult` | Meta/daemon identity |
| Group, profile, session, declare, secret, watch, webhook payloads/views | Extended control-plane surface |

## Design notes

- **Leaf package**: no project imports; safe for both client and daemon.
- **JSON over methods**: gRPC `Call` carries method name + JSON payload; these structs are that JSON schema.
- **Catalog alignment**: MCP tools (`pm_start`, …) map onto `Method*` via `internal/cli`.
- **Redaction-friendly DTOs**: secret list types expose names/paths only; values are never returned on list.

## Testing

Unit tests live next to the package (`methods_test.go`). Catalog and parity tests in `internal/cli` and `internal/daemon` also depend on `AllMethods` and the method constants.

```bash
go test ./internal/api/...
```

## Related packages

- [`internal/api/gen/pmmcp/v1`](gen/pmmcp/v1/) — generated gRPC/protobuf messages and `Daemon` service
- [`internal/ipc`](../ipc/) — client dial/call using these types
- [`internal/daemon`](../daemon/) — server dispatch and handlers
- [`internal/cli`](../cli/) — CLI and MCP catalog mapping onto methods
- `api/proto/pmmcp/v1/daemon.proto` — source of truth for the gRPC surface
