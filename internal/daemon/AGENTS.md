# agents: daemon

## role

`pmmcpd` control plane: wires store, process router, authz, audit/events, groups/profiles/sessions, secrets keyring, watch, webhooks, and the full IPC method surface. Sole long-lived parent of managed processes. Listens only on a **private** endpoint (UDS / named pipe) with peer-UID filtering; serves gRPC unary `Call` plus log/event streams. Thin binary entry is `cmd/pmmcpd` → `daemoncmd` → `New` + `ListenAndServe`.

## surface

| Symbol / area | Notes |
|---------------|--------|
| `Server` | Assembler state: store, manager/router, events, audit, sessions, groups, profiles, hooks, shares, watches, keyring, in-memory indexes |
| `Options` | `Config`, test seams: `DBPath`, `Manager`, `Store`, webhook deliver/poll, auto-restart max/backoff |
| `New(ctx, Options)` | Mkdir state `0700`, open/migrate SQLite, local+router manager, keyring, SQLite audit/event logs |
| `ListenAndServe(ctx)` | `ipc.Listen` (peer-cred), optional boot relaunch, background loops, gRPC serve until cancel |
| `Close` | Cancel run ctx, close listener + store (and override `dbStore` if set) |
| `RegisterGRPC` / `grpcAdapter` | `Call` → `handle`; `SubscribeLogs` / `SubscribeEvents` (capped duration, redaction, authz) |
| `handle` / `dispatchExtra` | API version gate + principal + method switch / product methods |
| `require` / `authorizeTarget` / `deny` / `auditDeny` | Capability packs + cross-session share book |
| `doStart` / `doStop` / `doRestart` / … | Core lifecycle + logs/events/audit in `server.go` |
| `handlers_ext.go` | Product path: update/run/wait/enable, groups, profiles, session/share, declare, secrets, watch, webhooks, metrics, log export/ship/subscribe |
| `RelaunchEligible` | Boot restart of `desired=running` (adopt live PID when possible) |
| `runAutoRestartLoop` / `runWatchDispatchers` / `runWebhookDispatch` | Supervision + file watch + SSRF-aware webhook delivery |
| `pidAlive` | Platform PID liveness (unix / other) |
| Secrets | Resolve `secret://` at launch only; register for log redaction; keyring paths not values in list views |
| Sandbox | Default from config; relaxation requires `CapSandboxRelax` and is auditable |

## deps

- Project: `api` (+ `api/gen/pmmcp/v1`), `audit`, `authz`, `config`, `declare`, `domain`, `engine/docker|podman` (runtime info), `event`, `group`, `id`, `ipc`, `logcap`, `observability`, `ports`, `process` (+ `drivers`, `local`), `profile`, `project`, `sandbox` (+ platform), `secret`, `session`, `store` (+ `sqlite`), `supervise`, `version`, `watch`, `webhook`
- Third-party: `google.golang.org/grpc`

## invariants

- **Daemon authority** — clients never own child PIDs; supervision, relaunch, health restart live here.
- **Private control plane only** — gRPC over UDS/named pipe from `ipc.Listen`; no default TCP/HTTP admin listener.
- **Peer UID** — same-OS-user filter on accept (`ipc`); roles/caps apply on top of cooperative same-user identity.
- **Authz on sensitive methods** — `require` + audited deny; cross-session targets need full/operator or `ShareBook` grant (`authorizeTarget`).
- **API version fail-closed** — empty/mismatched client version rejected; no silent backfill on gRPC `Call`.
- **Argv, never implicit shell** — `domain.ValidateCommand` + process manager exec of argv lists.
- **Secrets** — never log or return resolved secret values in status/list/audit detail; resolve into child env at start; redact on log paths; keyring dir under state.
- **Sandbox default strict** — missing isolation fails closed at process layer; relaxation is capability-gated.
- **Webhooks SSRF-aware** — delivery via `webhook` package allowlist / blocked ranges.
- **State dir permissions** — `StateDir` created `0700`; SQLite + keyring under it.
- **No `func init()`** — explicit wiring in `New` / `main`.
- **Context** — no `context.Context` stored on `Server`; hold `runCancel` / `runDoneCh` only.
- **Method coverage** — every `api.AllMethods` entry must dispatch (parity tests); `hello` is handshake-only (not a `pm_*` tool).

## tests

- `parity_test.go` — every `AllMethods` handled (not unimplemented); core methods succeed
- `product_test.go` / `m2_product_test.go` / `m3_product_test.go` — lifecycle, logs, secrets redaction, groups/profiles
- `handlers_*_test.go` / `handlers_dispatch_deny_test.go` — authz deny, declare/secret/webhook, group/session
- `server_test.go` / `server_uncovered_test.go` / `*_closures_test.go` — New failures, relaunch, list filters, seams
- `grpc_stream_test.go` / `grpc_internal_test.go` — stream RPCs
- `secret_redact_test.go` / `supervise_internal_test.go` / `state_mode_test.go` / `unavailable_test.go`
- Integration/e2e: `test/integration`, `test/e2e` boot real daemon socket
- Unit tests hermetic where possible (`t.TempDir`, injected `Manager`/`Store`); `t.Parallel()` when safe. **Coverage floor: ≥80% statements** for this package (CI). Do not add production seams only to hit residual lines. Meaningful assertions beat line hits.

## touch map

### Add or change an IPC method / handler

1. `internal/api` — `Method*` + `AllMethods` + DTOs.
2. **This package** — case in `handle` or `dispatchExtra`; call `require` (and `authorizeTarget` when acting on a process owned by another session).
3. Implement `doXxx`; audit sensitive mutations; never put secret values in audit detail.
4. `internal/cli` — `ToolMethod` + description + CLI verb or intentional omission.
5. Parity: `parity_test.go`, `cli` catalog tests, docs if user-visible.
6. Catalog pin remains **65** `pm_*` tools unless product deliberately changes it (`hello` stays out of the catalog).

### Wire a new subsystem dependency

1. Construct in `New` (or `newProductState`); inject via `Options` for tests.
2. Background work: start under `ListenAndServe` run ctx; stop on cancel / `Close`.
3. Do not store a live `context.Context` on `Server`.

### Streaming (logs/events)

- Prefer gRPC stream RPCs (`SubscribeLogs` / `SubscribeEvents`) with version + capability + redaction; unary subscribe bookkeeping remains in handlers for catalog methods.

## do-not

- Do not bind a public TCP/HTTP control API “for debugging.”
- Do not skip peer-cred, API version, or authz checks to green a test in production paths.
- Do not log or return resolved secrets, full env maps, or token file contents (daemon.info redacts token path).
- Do not put supervision or store ownership in `cli` / `mcp`.
- Do not auto-start from clients — this package **is** the daemon; clients must fail with `daemon_unavailable`.
- Do not add `func init()` registration for drivers — use `process/drivers` / constructor injection.
- Do not hand-edit `internal/api/gen/**`.
- Do not weaken sandbox fail-closed or secret redaction to silence tests.
- Do not use UUIDs for new record IDs — prefixed ULIDs via `internal/id`.

## related

- Root law: [`../../AGENTS.md`](../../AGENTS.md) (security, parity, architecture)
- Human overview: [`README.md`](README.md)
- Entry: `cmd/pmmcpd`, `internal/daemoncmd` (if present)
- Clients: [`../cli/AGENTS.md`](../cli/AGENTS.md), [`../mcp/AGENTS.md`](../mcp/AGENTS.md)
- Contract/transport: [`../api/AGENTS.md`](../api/AGENTS.md), [`../ipc/AGENTS.md`](../ipc/AGENTS.md), [`../authz/AGENTS.md`](../authz/AGENTS.md)
