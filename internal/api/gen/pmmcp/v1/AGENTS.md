# agents: api/gen/pmmcp/v1

## role
**Generated** protobuf and gRPC stubs for the private `pmmcp.v1.Daemon` control plane (package name `pmmcpv1`). Unary `Call` plus server-streaming log/event subscribe. **Do not hand-edit.**

## surface
| Symbol / area | Notes |
|---------------|--------|
| Messages | `CallRequest`, `CallResponse`, `SubscribeLogsRequest`, `LogChunk`, `SubscribeEventsRequest`, `EventChunk` |
| Client | `DaemonClient`, `NewDaemonClient` |
| Server | `DaemonServer`, `UnimplementedDaemonServer`, `RegisterDaemonServer` |
| RPCs | `Call` (unary); `SubscribeLogs`, `SubscribeEvents` (server-streaming) |
| Full method names | `Daemon_*_FullMethodName` constants |

## deps
- Project: none (generated)
- Third-party: `google.golang.org/grpc`, `google.golang.org/protobuf` (load-bearing)

## invariants
- Source of truth: `api/proto/pmmcp/v1/daemon.proto`.
- Regenerate only via `buf generate` (`buf.gen.yaml` → `internal/api/gen`, `paths=source_relative`).
- `Call` payload/response bytes are JSON matching hand-written `internal/api` types and method names.
- Private control plane over UDS/named pipe — not a public HTTP/TCP admin API.
- go_package path stays `github.com/scrothers/pmmcp/internal/api/gen/pmmcp/v1;pmmcpv1`.

## tests
- No tests in this package (generated). Exercise via `internal/ipc` and `internal/daemon` gRPC tests.
- Coverage excluded — generated.

## touch map
- Wire/RPC change → edit `.proto` → `buf generate` → fix daemon/ipc implementors.
- JSON method semantics → change `internal/api` DTOs/handlers, not generated Go.

## do-not
- **DO NOT EDIT** `*.pb.go` / `*_grpc.pb.go` by hand.
- Do not add hand-written business logic or tests in this directory.
- Do not open a public network listener based on these stubs.
- Do not invent alternate `go_package` paths.

## related
`api/proto/`, `internal/api` (JSON contract), `internal/ipc`, `internal/daemon`.
