# agents: mcp

## role

SDK-agnostic MCP **resources** and **prompts** helpers for the `pmmcp mcp` stdio adapter in `internal/cli`. Dynamic resource content is fetched over private IPC (`ipc.Dial` + `api.Method*`); multi-line prompt/doc bodies live in `internal/prompts`. This package is **not** a full MCP server, not a second control plane, and does **not** register tools (tools stay in `cli.ToolMethod` → daemon).

## surface

| Symbol / area | Notes |
|---------------|--------|
| `Resource` / `ResourceTemplate` | MCP resource descriptors (URI, name, mime) |
| `StaticResources` | Fixed singletons (no daemon required to enumerate) |
| `ResourceTemplates` | Parameterized URIs: project, process, process log, group |
| `ListResources(ctx, endpoint)` | Static set + dynamic process/group rows; on dial/list error returns **static + error** (degraded discovery) |
| `ReadResource(ctx, endpoint, uri)` | Local docs/declare without dial; else daemon IPC |
| `Prompt` / `PromptArg` | Prompt descriptors |
| `ListPrompts` | Maps `prompts.List` → MCP shapes |
| `GetPrompt(name, args)` | `prompts.Render` only |
| Static URIs | `pmmcp: / processes`, `daemon`, `project/current`, `declare`, `ports`, `events/recent`, `docs/error-codes`, `docs/tool-index` |
| Templates | `pmmcp: / project/{id}`, `process/{name_or_id}`, `process/{name_or_id}/log`, `group/{name}` |

## deps

- Project: `api`, `domain`, `ipc`, `prompts`
- Third-party: none (stdio MCP server and go-sdk live in `cli`)

## invariants

- **Thin adapter** — marshal/format only; no process spawn, no store, no authz.
- **Tools are not here** — all `pm_*` dispatch is `cli` → private IPC.
- **Degraded list, strict read** — `ListResources` keeps static URIs discoverable when daemon is down; `ReadResource` errors on dial failure / unknown URI.
- **Prompt bodies** — embedded markdown under `internal/prompts/md`; this package only maps descriptors and render args.
- **Docs resources** — `docs/error-codes` and `docs/tool-index` are local (`prompts.Doc`), no IPC.
- **`pmmcp: / declare`** — reads `pmmcp.yaml` / `.yml` from **cwd**, not the daemon store.
- **ID vs name** — process URI segments with `proc-` prefix use `ID` payloads; otherwise name.
- **No public network** — only dials the configured private endpoint string passed by the caller.

## tests

- `mcp_test.go` — fake gRPC daemon: list/read URIs (up/down/errors), docs, declare, process by name, prompts render/unknown
- `pure_helpers_internal_test.go` — `idOrName` / `logsPayload` pure helpers
- Consumed from `cli` SDK registration tests (`registerResources` / `registerPrompts`)
- Unit tests hermetic (`t.Parallel` when safe). **Coverage floor: ≥80% statements** for this package (CI). Do not add production seams only to hit residual lines. Meaningful assertions beat line hits.

## touch map

### Add a static resource

1. Add URI to `StaticResources` with description from `prompts.ResourceDescription` (and `lines.toml` / prompts package as needed).
2. Handle `ReadResource` switch (local or `callJSON` / helper).
3. Register is automatic for statics via `cli.registerResources` iterating `StaticResources`.
4. Tests: list includes URI; read returns expected shape; daemon-down still lists statics.

### Add a resource template / dynamic URI

1. `ResourceTemplates` entry + description key in prompts.
2. `ListResources` append logic if instances should appear when daemon is up.
3. `readDaemonResource` branch for the URI pattern.
4. CLI template registration already loops `ResourceTemplates`.

### Add a prompt

1. Spec + markdown body in **`internal/prompts`** (not here).
2. `ListPrompts` / `GetPrompt` pick it up automatically.
3. Ensure prompt text does **not** instruct dumping secrets or disabling sandbox casually (root security law).

## do-not

- Do not implement MCP tools or catalog maps here — that is `cli.ToolMethod`.
- Do not import `daemon`, process drivers, or store.
- Do not auto-start the daemon or hide dial failures on read.
- Do not return raw secret values from resources (status/list paths must stay redacted at the daemon).
- Do not hardcode multi-line prompt/doc bodies — use `internal/prompts`.
- Do not bind TCP or open a second control plane.
- Do not treat this package as the MCP server entrypoint (`pmmcp mcp` lives in `cli`).

## related

- Root law: [`..../AGENTS.md`](..../AGENTS.md)
- Human overview: [`README.md`](README.md)
- Consumer: [`../cli/AGENTS.md`](../cli/AGENTS.md) (`runMCPSDK`)
- Bodies: [`../prompts/`](../prompts/)
- Docs: `docs/mcp.md`
