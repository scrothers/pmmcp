# agents: api

## role
Shared private IPC contract: `APIVersion`, framed `Request`/`Response`, `Method*` names, `AllMethods`, and method-specific JSON DTOs. Source of truth for catalog parity with MCP tools (via `cli.ToolMethod`). Hand-written JSON surface only — gRPC stubs live under `api/gen/pmmcp/v1`.

## surface
| Symbol / area | Notes |
|---------------|--------|
| `APIVersion` | `"1.0"` major.minor negotiated at hello |
| `Request`, `Response` | Framed call: session, role, payload bytes |
| `Method*` + `AllMethods` | Full IPC method surface (process, logs, events, …) |
| Payload/result DTOs | e.g. `StartPayload`, `ProcessView`, `SecretPayload`, group/profile/session/declare/watch/webhook views |
| `ParseVersion`, `Compatible` | numeric version compare → `domain.CodeIPCVersionMismatch` |

## deps
- Project: `internal/domain` (version mismatch errors only)
- Third-party: none

## invariants
- Method strings map 1:1 to MCP `pm_*` tools via `cli.ToolMethod`; every method is listed in `AllMethods`.
- Payload is method-specific JSON; list/status views use `EnvKeys` / secret names-paths — not secret values (`SecretListResult` never returns values).
- `Compatible`: fail closed on major mismatch or client minor newer than daemon.
- Keep Request/Response field names stable; protobuf envelopes are separate in `gen/`.
- Adding a method is deliberate product work: api + daemon handler + cli map + parity tests + docs when user-visible.

## tests
- `methods_test.go` — `AllMethods` non-empty, unique, size floor.
- `version_test.go` — parse and compatibility matrix.
- Catalog parity also enforced from `cli` tests against `AllMethods`.
- Unit tests hermetic (`t.Parallel` when safe). **Coverage floor: ≥80% statements** for this package (CI). Do not add production seams only to hit residual lines. Meaningful assertions beat line hits.

## touch map
- New IPC method → `Method*` + `AllMethods` + DTOs if needed → daemon + `cli.ToolMethod` + parity tests.
- Handshake version bump → `APIVersion` + `Compatible` behavior + clients.
- Secret-related DTOs → never expand list/read shapes to return raw values.

## do-not
- Hand-edit or reimplement gRPC types here — change `api/proto` and `buf generate`.
- Invent methods without full catalog parity (api + daemon + cli + tests).
- Import daemon/cli (this package stays a thin contract leaf above domain).
- Put supervision or OS logic in DTO helpers.
- Log or document secret **values** in API shapes meant for agents.

## related
`internal/api/gen/pmmcp/v1` (wire), `internal/daemon`, `internal/cli`, `internal/ipc`.
