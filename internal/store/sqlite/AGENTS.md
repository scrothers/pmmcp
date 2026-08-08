# agents: store/sqlite

## role
Sole `store.ProcessStore` driver using pure-Go `modernc.org/sqlite` (driver name `"sqlite"`, not mattn). Daemon-only open of the state database. Also exposes the shared `*sql.DB` for event/audit logs.

## surface
| Symbol / area | Notes |
|---------------|--------|
| `Open` / `OpenContext` | DSN pragmas: busy_timeout, WAL, foreign_keys; `MaxOpenConns(1)`; no auto-migrate |
| `(*Store).DB` | shared handle for event/audit; callers must not Close |
| `Migrate` | ordered `migrations` + `schema_migrations`; idempotent |
| CRUD + CAS | full ProcessStore: Create/Get/Update/UpdateWithCAS/Delete/List/Close |
| `var _ store.ProcessStore` | compile-time anchor |

## deps
- Project: `internal/domain`, `internal/store`
- Third-party: `modernc.org/sqlite` (+ `modernc.org/sqlite/lib` for constraint codes)

## invariants
- No CGo / no `mattn/go-sqlite3` (enforced by driver tests on go.mod)
- Never edit applied migration SQL — append new versions only
- Schema: v1 processes; v2 env_keys/predecessor/successor; v3 partial unique live name (`successor_id = ''`)
- Store env **key names** only (`env_keys_json`), never values
- Times as fixed-width UTC layout so string ORDER BY is chronological
- Unique violations → `store.ErrConflict`; missing row → `store.ErrNotFound`
- Create/Update/CAS call `p.Validate`; empty Runtime stored as `"local"`
- One writer connection; blank import registers modernc — no `func init` in this package

## tests
- `sqlite_test.go`, `sqlite_ext_test.go`, `scan_test.go` — CRUD, filters, CAS, conflict
- `driver_test.go` — modernc present, mattn absent; migrate/flaky/error tests for failure paths
- Unit tests hermetic (`t.Parallel` when safe; `t.TempDir` DB files). **Coverage floor: ≥80% statements** for this package (CI). Do not add production seams only to hit residual lines. Meaningful assertions beat line hits.

## touch map
- `internal/daemon.New` — Open state_dir/pmmcp.db, Migrate, pass Store + DB to event/audit
- Forbidden importers: `cmd/pmmcp`, CLI, MCP adapter

## do-not
- Do not open from non-daemon product paths
- Do not hand-edit applied migration SQL
- Do not store secret or env values in process rows
- Do not bump modernc casually inside feature work (isolate dependency commits)
- Do not Close the handle returned by `DB` from event/audit callers

## related
- parent `internal/store`, `internal/domain`, `internal/event`, `internal/audit`, 
