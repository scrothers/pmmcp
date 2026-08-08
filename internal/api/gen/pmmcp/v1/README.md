# Package pmmcpv1 (generated)

## Overview

Generated Go bindings for the private `pmmcp.v1.Daemon` gRPC service. Files:

- `daemon.pb.go` — messages
- `daemon_grpc.pb.go` — client/server stubs

**Do not edit these files.** Change the proto and regenerate.

## When to use

Import this package when implementing or dialing the gRPC control plane:

- Daemon: implement `DaemonServer` (see `internal/daemon`)
- Client: `NewDaemonClient` (see `internal/ipc`)

For method names and JSON DTO shapes, use [`internal/api`](../../) instead.

## Key types and functions

| Symbol | Purpose |
|--------|---------|
| `CallRequest` / `CallResponse` | Unary method + JSON payload/result |
| `SubscribeLogsRequest` / `LogChunk` | Streaming log follow |
| `SubscribeEventsRequest` / `EventChunk` | Streaming domain events |
| `DaemonClient` / `NewDaemonClient` | Client API |
| `DaemonServer` / `RegisterDaemonServer` | Server API (embed `UnimplementedDaemonServer`) |

## Design notes

- Proto source: [`api/proto/pmmcp/v1/daemon.proto`](../../../../../api/proto/pmmcp/v1/daemon.proto)
- Generate: from repo root, `buf generate` (`buf.gen.yaml` writes under `internal/api/gen`)
- `Call` is a thin envelope; domain semantics live in `internal/api` method strings and JSON
- Streaming RPCs carry backpressure via gRPC flow control and optional `max_duration_sec`

## Testing

No unit tests in this directory. Exercise via IPC client and daemon gRPC server tests.

## Related packages

- [`api/proto`](../../../../../api/proto/) — proto sources and notes
- [`internal/api`](../../) — JSON IPC method/DTO contract
- [`internal/ipc`](../../../ipc/) — dial + `Call` client
- [`internal/daemon`](../../../daemon/) — `DaemonServer` implementation
