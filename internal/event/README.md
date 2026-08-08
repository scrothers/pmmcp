# event

In-memory domain event bus and bounded append-only event log.

## Overview

Domain events are lifecycle and control-plane facts emitted by the daemon: process started/stopped/exited, relaunch, declare applied, and similar. Each event can carry process, group, and session references plus a free-form message.

This package is intentionally separate from:

| Stream | Package / concern |
|--------|-------------------|
| Process stdout/stderr | `logcap` |
| Domain lifecycle events | **this package** |
| Who did what (authz) | `audit` |
| Metrics/traces | `observability` |

See for the product model.

## When to use

- Daemon code that must record or query lifecycle events for CLI `events`, MCP tools, webhooks, or supervision feedback.
- Product tests that assert event emission after start/stop/restart.

Not for capturing child process logs or durable multi-host event streaming. Persistence to SQLite may wrap or mirror this bus later; the current implementation is process-local memory with a max-keep bound.

## Key API

```go
bus:= event.NewBus(100_000) // daemon uses a large bound

e, err:= bus.Append(ctx, event.Event{
 Type: "process.started",
 ProcessID: pid,
 SessionID: session,
 Message: name,
})
// e.ID is evt-…, e.At is UTC now when left zero

recent:= bus.Query(ctx, pid, 50) // filter by process
all:= bus.Query(ctx, "", 100) // no process filter; default limit 100 if limit<=0
```

### `Event` fields

| Field | Meaning |
|-------|---------|
| `ID` | `evt-` ULID (auto if empty) |
| `Type` | e.g. `process.started`, `process.stopped`, `process.exited` |
| `ProcessID` / `GroupID` / `SessionID` | optional resource refs |
| `Message` | short human/agent detail |
| `At` | event time (UTC now if zero on Append) |

## Design notes

- **Retention.** `NewBus(maxKeep)` drops the oldest events once the slice exceeds `maxKeep`. Zero or negative maxKeep defaults to 10_000.
- **IDs.** Generated via `id.New(id.Event)` so every bus event matches the house ULID prefix table.
- **Query semantics.** Chronological order (oldest→newest among retained). Optional process filter. Always returns a copy so callers cannot mutate internal storage under the mutex.
- **Context.** Methods take `context.Context` for consistency with I/O-shaped APIs; the in-memory bus does not currently block on ctx beyond what callers do around the call.
- **Daemon usage.** `Server` holds one bus; supervise loops and handlers append on lifecycle transitions.

## Testing

```bash
go test./internal/event/
```

Unit coverage: ID/time assignment, process filter, empty-filter miss. No external services.

## Related

- [`../id/`](../id/) — `evt-` generation
- [`../audit/`](../audit/) — separate authz audit trail (`aud-`)
- [`../logcap/`](../logcap/) — process log capture
- [`../daemon/`](../daemon/) — primary producer/consumer
- (logs and events), (identity)
