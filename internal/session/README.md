# session

In-memory **session registry** for control-plane connections (MCP / CLI talking to `pmmcpd`). A session records *who* is acting for audit and optional cleanup when a connection ends.

## Model

| Field | Meaning |
|-------|---------|
| `ID` | Internal prefixed ULID: `sess-…` (always allocated) |
| `HarnessID` | Optional external conversation/session id from the agent harness |
| `Role` | Authz role string at open time (e.g. agent/operator) |
| `CreatedAt` / `EndedAt` | Lifetime markers |

**Primary identity for display:** `PrimaryID()` returns the harness id when set, otherwise the internal `sess-` id. Processes still store whichever session key the daemon attached at start.

## Registry API

```go
r := session.NewRegistry()

s, err := r.Open("claude-conv-123", "agent") // harness preferred
s, err = r.Open("", "operator") // internal id only

got, ok := r.Get(s.ID)
ok = r.End(s.ID) // stamps EndedAt
```

The registry is process-local (daemon memory). It is **not** written to SQLite. After a daemon restart, sessions are empty; durable process rows may still carry a historical `session_id`.

## How the daemon uses it

1. On each request, `ensureSession` reuses `req.Session` if it is a known internal id; otherwise treats it as a harness id and `Open`s a new session.
2. `pm_session_info` / session CLI surface expose id, harness id, role, timestamps.
3. `pm_session_end` marks the session ended and may stop processes flagged `stop_on_disconnect` for that session (logic lives in `internal/daemon`, not here).

Default product rule (glossary): **processes survive disconnect**. Session is attribution, not a hard lock on the process.

## Design constraints

- 1:1 with client connection conceptually; many sessions per daemon uptime.
- Prefer harness-provided ids so multi-turn agent work correlates across reconnects when the host reuses the same conversation id.
- No import of store/drivers — keep this package a leaf helper.

## Tests

```bash
go test ./internal/session/...
```
