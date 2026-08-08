# agents: event

## role
Append-only domain event bus and durable event log for lifecycle/control facts (`process.started`, etc.). Two backends: volatile in-memory ring (`NewBus`) and SQLite table on the daemon's shared DB (`NewSQLiteLog`). Not process stdout and not audit.

## surface
| Symbol / area | Notes |
|---------------|--------|
| `Event` | `Seq`, `ID` (`evt-`), `Type`, `ProcessID`, `GroupID`, `SessionID`, `Severity`, `ProjectID`, `Message`, `At` |
| `Bus` | mutex + optional `*sql.DB`; retention by count and age |
| `NewBus(maxKeep)` | in-memory; `maxKeep<=0` → `DefaultMaxEvents` (100k) |
| `NewSQLiteLog(db, opts...)` | migrate `events` table; nil db → error |
| `WithMaxCount` / `WithMaxAge` / `WithMaxPayload` | functional options (SQL path primarily) |
| `Append` | assigns `evt-` + UTC `At` + monotonic `Seq`; truncates Message |
| `Query` / `QuerySince` | chronological; process filter; limit 0 → 100; SQL query errors log and return nil |

## deps
- Project: `internal/id` (`id.New(id.Event)`)
- Third-party: none (`database/sql` only)

## invariants
- Domain events ≠ process logs (`logcap`) ≠ audit (`audit`) — 
- Auto IDs are `evt-` prefixed ULIDs; `Seq` is the resumable stream cursor
- Defaults: 100k events, 7-day max age, 16 KiB max payload (UTF-8-safe truncate + marker)
- In-memory Query returns a defensive copy; count trim copies into a fresh slice
- SQLite shares the daemon store handle (`sqlite.Store.DB`); this package does not own Close
- Thread-safe concurrent Append/Query; no `func init`

## tests
- `event_test.go`, `event_mem_test.go`, `event_sqlite_test.go`, `event_error_test.go`, `flaky_driver_test.go` — assign ID/Seq, filter, retention, truncate, SQL migrate/append/query
- Unit tests hermetic (`t.Parallel` when safe). **Coverage floor: ≥80% statements** for this package (CI). Do not add production seams only to hit residual lines. Meaningful assertions beat line hits.

## touch map
- Daemon wires bus + lifecycle appends; MCP/CLI event list/stream methods read via daemon handlers
- Schema lives here (CREATE TABLE IF NOT EXISTS), not in `store/sqlite` migrations

## do-not
- Do not store child stdout/stderr or authz “who did what” (use `logcap` / `audit`)
- Do not put secret values in `Message`
- Do not open a separate DB file; share the daemon pool via `DB`
- Do not return internal slice aliases from in-memory Query
- Do not invent non-`evt-` IDs for bus events

## related
- `internal/audit`, `internal/store/sqlite` (`DB`), `internal/id`, 
