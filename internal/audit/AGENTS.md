# agents: audit

## role
Append-only control-plane audit trail: who did what to which target, with outcome and capability. Two backends: volatile in-memory ring (`New`) and durable SQLite table (`NewSQLiteLog`) on the daemon's shared DB. Forensic retention is longer than the domain event log.

## surface
| Symbol / area | Notes |
|---------------|--------|
| `Outcome*` | `allowed`, `denied`, `error` |
| `Record` | `Seq`, `ID` (`aud-`), `Action`, `Actor`, `Role`, `SessionID`, `Target`, `Outcome`, `Capability`, `Client`, `Reason`, `RequestID`, `Detail`, `At` |
| `Filter` | Actor, SessionID, Action, Target, Outcome, Since, Until — zero = unrestricted |
| `Log` | mutex + optional `*sql.DB` |
| `New(maxKeep)` | in-memory; `maxKeep<=0` → `DefaultMaxRecords` (100k) |
| `NewSQLiteLog(db, opts...)` | migrate `audit` table; nil db → error |
| `WithMaxAge` | default ~90 days (`DefaultMaxAge`) |
| `Append` | assigns `aud-` + UTC `At` + `Seq` |
| `Query` / `QueryFilter` | newest-first then chronological return; limit 0 → 100 |

## deps
- Project: `internal/id` (`id.New(id.Audit)`)
- Third-party: none (`database/sql` only)

## invariants
- IDs are `aud-` ULIDs when auto-assigned
- Durable backend retains by **age only** — never truncates by count
- In-memory backend also rings by `maxKeep` for tests/interim use
- Query returns a defensive copy on the memory path; SQL errors log and return nil
- No secret values in `Detail` / `Reason` — callers redact
- Thread-safe; no stored `context.Context`; no `func init`

## tests
- `audit_test.go`, `audit_mem_test.go`, `audit_sqlite_test.go`, `audit_error_test.go`, `flaky_driver_test.go` — Append prefixes, filter match, retention, SQL path
- Unit tests hermetic (`t.Parallel` when safe). **Coverage floor: ≥80% statements** for this package (CI). Do not add production seams only to hit residual lines. Meaningful assertions beat line hits.

## touch map
- Daemon appends on sensitive methods (authz allow/deny/error) and serves query tools
- Schema created here on the shared `sqlite.Store.DB` handle

## do-not
- Do not log credentials, tokens, or resolved secrets into records
- Do not hand-roll non-`aud-` IDs
- Do not silently drop durable rows by count
- Do not open a private DB; share the daemon pool
- Do not confuse with `event` (lifecycle bus) or process logs

## related
- `internal/event`, `internal/authz`, `internal/store/sqlite`, `internal/id`, 
