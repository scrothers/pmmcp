# agents: cli

## role

Thin `pmmcp` client: cobra command tree, MCP tool catalog (`ToolMethod` / `ToolDescription`), and stdio MCP adapter (`pmmcp mcp`). Every mutating or status path dials the private IPC endpoint and calls one `api.Method*`; this package never owns child PIDs, never auto-starts `pmmcpd`, and never implements supervision. Catalog size is frozen at **65** `pm_*` tools (parity-tested against `api.AllMethods`).

## surface

| Symbol / area | Notes |
|---------------|--------|
| `Execute` / `Run` / `NewRootCmd` | Binary entry; builds cobra tree, silences usage/errors for main exit mapping |
| `rootState` | Per-invocation `--json` / `--config` (or `PMMCP_CONFIG`); `dial` → `ipc.Dial` |
| `ToolMethod` | **65** MCP tool names → `api.Method*` (frozen; `TestToolMethodCount`) |
| `ToolDescription` | From `prompts.ToolDescriptions()` for `tools/list` |
| `ToolNames` / `MethodSet` / `AllMethodsSet` / `ReverseToolMethod` | Catalog / parity helpers |
| `IntentionalCLIOmissions` + `cliVerbs` / `CommandForTool` / `Dispatchable` | Streaming tools without CLI verbs; catalog↔CLI coverage |
| `callJSON` / `jsonCmd` / `idCmd` / `dslCmd` / `payloadFromArgs` | IPC call + strict payload DSL (`key=value`, `key:=json`, `--flag`, `--json`) |
| Process / admin / declarative cmds | `commands_process.go`, `commands_admin.go`, `commands_declarative.go` |
| `runMCPSDK` / `registerTools` / `registerResources` / `registerPrompts` | Official go-sdk stdio MCP server |
| `mcpCall` / `schemaForTool` | Tool → IPC bridge; specialized JSON Schema for well-known tools |
| `installService` / `uninstallService` / `resolveDaemonPath` | User-service install via `internal/service` (not process ownership) |
| `doctor` / `version` / `mcp` | Local health check, version print, MCP stdio — not catalog tools |

## deps

- Project: `api`, `config`, `domain`, `doctor`, `ipc`, `mcp` (resources/prompts), `prompts`, `service`, `version`
- Third-party: `github.com/spf13/cobra`, `github.com/modelcontextprotocol/go-sdk/mcp`

## invariants

- **Thin client only** — no process spawn, no store, no authz engine; authority lives in `pmmcpd`.
- **No surprise daemon spawn** — dial failure surfaces `daemon_unavailable`; CLI/MCP never fork `pmmcpd`.
- **Catalog parity** — every `ToolMethod` value ∈ `api.AllMethods`; every non-omitted tool has a dispatchable `cliVerbs` path; changing the **65** count is deliberate product work.
- **Intentional omissions (5)** — `pm_logs_subscribe`/`unsubscribe`, `pm_events_subscribe`/`unsubscribe`/`subscriptions` (streaming; MCP/gRPC only).
- **`hello` is not a tool** — IPC handshake only (`api.MethodHello`); not in `ToolMethod`.
- **Secrets on stdin** — `secret set` rejects `value=` on argv; value piped only.
- **Strict payloads** — malformed DSL / JSON → clear `invalid_argument`, never silent invent.
- **MCP tools fail as `IsError` content** — dial/daemon errors do not crash the stdio server process.

## tests

- `catalog_test.go` — count=65, methods ⊂ AllMethods, omissions, `Dispatchable`, descriptions, reverse map
- `cli_internal_test.go` / `dispatch_internal_test.go` / `run_test.go` — DSL, command paths, daemon-down
- `mcp*.go` / `sdk_*_test.go` / `scripted_daemon_internal_test.go` — tool call, resources/prompts registration (fake daemon)
- `service_internal_test.go` — path resolve / install error paths
- E2E: `test/e2e` for full binary + MCP surface
- Unit tests hermetic (`t.Parallel()` when safe). **Coverage floor: ≥80% statements** for this package (CI). Do not add production seams only to hit residual lines. Meaningful assertions beat line hits.

## touch map

### Add or change a `pm_*` tool

1. `internal/api` — `Method*` + `AllMethods` (+ DTO if needed).
2. `internal/daemon` — handler + `require`/authz (and audit where required).
3. **`ToolMethod` + `ToolDescription` source** (`prompts` lines for description).
4. Either **`cliVerbs`** + cobra command, or **`IntentionalCLIOmissions`** with reason.
5. Specialized schema in `specializedSchemas()` when the tool needs non-empty input shape.
6. Green: `catalog_test.go`, daemon parity, user-facing `docs/` if behavior changes.
7. Update frozen count only deliberately (`TestToolMethodCount` want=65).

### Add a CLI-only verb (no MCP tool)

- Prefer not. Client surface is catalog-driven; doctor/version/install-service/mcp are the intentional non-catalog commands.

### Change payload DSL

- Edit `payloadFromArgs` + tests; keep fail-closed parse errors.

## do-not

- Do not put process supervision, sandbox apply, or store writes in this package.
- Do not auto-start the daemon on dial failure.
- Do not accept secret values on argv / MCP args that echo into shell history.
- Do not add a `pm_*` tool without full parity (api + daemon + catalog + CLI verb or omission + tests).
- Do not grow or shrink `ToolMethod` without product intent and test pin update.
- Do not reimplement resources/prompts here — use `internal/mcp` + `internal/prompts`.
- Do not open a network control listener or second IPC protocol from the client.

## related

- Root law: [`../../AGENTS.md`](../../AGENTS.md) (MCP/CLI/API parity, security)
- Human overview: [`README.md`](README.md)
- Consumers: `cmd/pmmcp`
- Peers: [`../daemon/AGENTS.md`](../daemon/AGENTS.md), [`../mcp/AGENTS.md`](../mcp/AGENTS.md), [`../api/AGENTS.md`](../api/AGENTS.md), [`../ipc/AGENTS.md`](../ipc/AGENTS.md)
