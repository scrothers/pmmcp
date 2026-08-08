# ipc

Private gRPC transport for pmmcp control-plane traffic.

## Overview

`pmmcp` (CLI/MCP) never owns children; it talks to `pmmcpd` over a local-only endpoint:

| OS | Endpoint |
|----|----------|
| Linux / macOS | Unix domain socket (file mode `0600`, directory `0700`) |
| Windows | Named pipe with owner-only SDDL |

The server side (`Listen`) wraps accept with same-UID peer filtering. The client (`Dial` / `Client`) opens a gRPC connection, probes with hello, and exposes unary `Call` plus log subscribe streaming.

## When to use

- **Daemon**: call `ipc.Listen(endpoint)` and serve gRPC on the returned `net.Listener`.
- **CLI / MCP / doctor / tools**: `ipc.Dial(ctx, endpoint)` then `Call` / `Hello` / `SubscribeLogs`.
- **Do not** use this package for public HTTP or multi-tenant remote APIs.

## Key API

```go
ln, err:= ipc.Listen(cfg.IPC.Endpoint)
// … register pmmcpv1.DaemonServer on grpc.NewServer; Serve(ln)

c, err:= ipc.Dial(ctx, endpoint)
defer c.Close
c.SetSession(sessionID, role)
err = c.Call(ctx, api.MethodList, api.ListPayload{}, &out)
stream, err:= c.SubscribeLogs(ctx, processID, "both", 30)
```

Helpers:

- `PeerUID` / `AllowedUID` — peer credential checks (Linux `SO_PEERCRED`).
- `WriteFrame` / `ReadFrame` — length-prefixed JSON; historical/tests only (not the shipped wire protocol; see supersession).

## Design notes

- **Transport only.** AuthZ matrices and method dispatch live in `daemon` / `authz`. Peer UID is the OS gate; role/session ride on Call metadata.
- **JSON payloads inside gRPC.** `CallRequest.Payload` is marshaled method args; results unmarshal into the caller’s `out` value.
- **Default deadlines.** `Call` adds a 60s timeout when the context has none; Dial’s hello probe uses 2s.
- **Platform files.** Dial, named-pipe listen, and peercred are build-tagged so non-target GOOS builds stay clean.
- **Fail closed on dial.** Connectivity/RPC errors map to `domain.CodeDaemonUnavailable` (retryable).

## Testing

```bash
go test./internal/ipc/...
```

Codec tests are hermetic. Peercred tests need a real UDS accept path. Full client/server coverage lives under `internal/daemon` product tests and `test/integration` / `test/e2e`.

## Related

- ADRs: 031-ipc-grpc, 019-ipc-and-authz, 034-ipc-json-frames (superseded)
- Proto / generated types: `api/proto/`, `internal/api`
- Consumers: `internal/daemon`, `internal/cli`, `internal/mcp`, `internal/doctor`
