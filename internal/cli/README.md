# Package cli

## Overview

`cli` implements the `pmmcp` client: a hand-dispatched command tree (cobra is deferred), the MCP tool catalog, and an MCP stdio server that forwards tool calls to the daemon over private IPC.

The package is intentionally thin. Lifecycle, supervision, and authz live in `pmmcpd`.

## When to use

- `cmd/pmmcp` entry: call `cli.Run(ctx, os.Args[1:])`
- Extending user-facing commands or the MCP tool surface
- Catalog parity work between CLI verbs, MCP `pm_*` tools, and `api.Method*` names

## Key types and functions

| Symbol | Purpose |
|--------|---------|
| `Run` | Top-level command dispatch |
| `ToolMethod` | MCP tool name → IPC method |
| `ToolDescription` | Human descriptions for `tools/list` |
| `ToolNames` | Sorted tool names |
| `MethodSet` / `AllMethodsSet` / `ReverseToolMethod` | Parity helpers |
| `IntentionalCLIOmissions` | Tools without a dedicated CLI verb |
| `CLICommandForTool` | CLI entrypoint hint or empty if omitted |

### Command surface (high level)

`version`, `doctor`, process lifecycle (`start`/`stop`/`restart`/`list`/`status`/`remove`/`run`/`wait`/`enable`/`disable`/`health`), `logs`/`grep`/`errors`/`events`, `group`, `profile`, declare (`validate`/`apply`/`diff`/`import`), `webhook`, `metrics`, `sandbox-profiles`, `ports`, `whoami`, `reload`, `session`, `secret`, `watch`, `share`/`unshare`, `project`, `mcp`, `install-service`/`uninstall-service`.

Global flag: `--json`.

## Design notes

- **Dial path**: loads config (`PMMCP_CONFIG` optional), dials `cfg.IPC.Endpoint` via `internal/ipc`.
- **Catalog is frozen at 65 tools**: changing the count fails `TestToolMethodCount` until catalog tests and docs update.
- **MCP**: `runMCPSDK` registers every catalog tool and selected resources/prompts; tools call the daemon with JSON args.
- **Omissions**: streaming subscribe tools are intentionally MCP/gRPC-primary; see `omission.go`.
- **Service install**: wraps `internal/service` using `pmmcpd` next to the client binary when present.

## Testing

```bash
go test ./internal/cli/...
```

Catalog tests enforce tool-name parity against the in-repo catalog. Broader surface coverage is in `test/e2e`.

## Related packages

- [`cmd/pmmcp`](../../cmd/pmmcp/) — binary entry
- [`internal/api`](../api/) — IPC methods and DTOs
- [`internal/config`](../config/) — endpoint and defaults
- [`internal/ipc`](../ipc/) — transport
- [`internal/mcp`](../mcp/) — resources and prompts
- [`internal/service`](../service/) — user service unit install
- [`internal/daemon`](../daemon/) — server that executes methods
