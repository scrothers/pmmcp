# Package audit

## Overview

`audit` appends and queries control-plane audit records. The daemon records lifecycle and configuration actions (start/stop, enable/disable, share, reload, and similar) so operators and agents can answer “who did what.”

The current implementation is an in-memory ring buffer. A durable backend can wrap the same shape later without changing call sites much.

## When to use

- Daemon handlers that should leave an audit trail after a mutating control-plane action
- `audit.query` / `pm_audit_query` implementations that list recent records

Do not use this package for managed-process stdout/stderr (that is log capture) or domain process events (that is `internal/event`).

## Key types and functions

| Symbol | Purpose |
|--------|---------|
| `Record` | One audit entry (`aud-` id, action, actor, session, target, detail, time) |
| `New(maxKeep)` | Create a log; default max keep is 10000 |
| `Append` | Store a record; fills ID and timestamp when omitted |
| `Query` | Return the last `limit` records, optionally filtered by `target` |

## Design notes

- **IDs**: generated via `internal/id` with the audit prefix (`aud-`).
- **Bounded memory**: oldest records drop when the log exceeds `maxKeep`.
- **Copy on read**: `Query` returns a new slice so callers cannot race the internal buffer.
- **Context**: signatures take `context.Context` for future I/O; the in-memory path ignores it today.

## Testing

```bash
go test ./internal/audit/...
```

Hermetic unit tests only (`t.TempDir` not required).

## Related packages

- [`internal/id`](../id/) — ULID generation (`aud-`)
- [`internal/daemon`](../daemon/) — primary consumer (append on mutations; query handler)
- [`internal/authz`](../authz/) — actor/role context for who may query audit
- [`internal/api`](../api/) — `MethodAudit` / IPC surface
