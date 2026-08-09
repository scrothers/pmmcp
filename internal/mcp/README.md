# mcp

MCP resources and prompts helpers used by the `pmmcp mcp` stdio adapter.

## Overview

The full MCP entrypoint lives in `internal/cli` (`pmmcp mcp`). This package supplies only:

- **Resources** — list/read URIs that mirror daemon state over private IPC
- **Prompts** — short agent templates (debug failing process, replace nohup)

Tools remain in the CLI catalog and always call the daemon via `ipc`.

## When to use

- Wiring MCP `resources/*` and `prompts/*` handlers in the CLI adapter.
- Reusing the same URI scheme if another thin MCP facade is added later.
- **Not** for embedding a second control plane or replacing gRPC IPC.

## Key API

```go
res, err:= mcp.ListResources(ctx, endpoint)
// always includes pmmcp://processes and pmmcp://daemon;
// appends pmmcp://process/<id> and …/log when list succeeds

text, err:= mcp.ReadResource(ctx, endpoint, "pmmcp://process/"+id+"/log")

prompts:= mcp.ListPrompts
body, err:= mcp.GetPrompt("debug_failing_process", processName)
```

| URI | Contents |
|-----|----------|
| `pmmcp://processes` | JSON list (`MethodList`) |
| `pmmcp://daemon` | Daemon info |
| `pmmcp://process/<id>` | Status view |
| `pmmcp://process/<id>/log` | Recent logs (200 lines) |

## Design notes

- **Degraded list.** If the daemon is down, `ListResources` still returns the two static entries so MCP clients can discover the surface.
- **Read is strict.** Unknown URIs and dial failures return errors.
- **Prompts are instructional strings**, not executable workflows; they name MCP tools the agent should call.
- covers MCP SDK choice at the CLI boundary; this package stays SDK-agnostic.

## Testing

```bash
go test./internal/mcp/...
```

Currently minimal/no unit tests; cover via CLI MCP integration or add table tests for URI routing with a test daemon socket.

## Related

- MCP SDK choice at the CLI boundary; this package stays a thin adapter
- See also: `https://github.com/scrothers/pmmcp/wiki/MCP`
- Docs: `https://github.com/scrothers/pmmcp/wiki/MCP`
- Consumers: `internal/cli`
- Transport: `internal/ipc`
