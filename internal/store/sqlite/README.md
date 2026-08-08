# sqlite

SQLite-backed implementation of [`store.ProcessStore`](../) using **[modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite)** (pure Go). This is the production durable store for process inventory and desired state.

## Access rule

**Only `pmmcpd` opens this database.** CLI and MCP clients must not import this package or open the file. They talk to the daemon over IPC.

Default path: `$state_dir/pmmcp.db` (see daemon config / `StateDir`).

## Usage

```go
s, err:= sqlite.Open(path)
if err != nil {... }
defer s.Close

if err:= s.Migrate(ctx); err != nil {... }

err = s.Create(ctx, proc)
proc, err = s.Get(ctx, id)
list, err = s.List(ctx, store.ProcessFilter{ProjectID: "proj-…"})
```

`Open` does **not** migrate. Call `Migrate` once at daemon startup (safe to call again; already-applied versions are skipped).

## Connection settings

- Driver name: `"sqlite"` (modernc registration; mattn uses `"sqlite3"` and is **not** a dependency)
- `MaxOpenConns(1)` — single local writer
- `PRAGMA foreign_keys = ON`
- `PRAGMA journal_mode = WAL`

## Migrations

Versions live in an ordered slice in `sqlite.go` and are recorded in `schema_migrations(version, applied_at)`.

| Version | Change |
|--------:|--------|
| 1 | `processes` table + indexes on `(project_id, name)` and `status` |
| 2 | `env_keys_json`, `predecessor_id`, `successor_id` (restart generations) |

**Rule:** never rewrite an applied migration’s SQL; append a new version.

### Process columns (summary)

Identity and scope: `id`, `name`, `project_id`, `profile`, `session_id` 
Lifecycle: `status`, `desired`, `pid`, `exit_code`, `last_error`, timestamps 
Spec: `command_json` (argv), `cwd`, `sandbox`, `runtime`, `log_dir` 
Secrets hygiene: `env_keys_json` — **key names only**, never values 
Restart chain: `predecessor_id`, `successor_id`

## Errors

| Condition | Return |
|-----------|--------|
| No row | `store.ErrNotFound` |
| Unique violation on insert | `store.ErrConflict` (wrapped) |
| Nil/invalid process | validation / `fmt.Errorf` with `sqlite: …` prefix |

## Tests

Hermetic unit tests use `t.TempDir` databases:

```bash
go test./internal/store/sqlite/...
```

`TestGoModUsesModerncOnly` fails the build if `mattn/go-sqlite3` appears in `go.mod`.

## Packaging / backup

Copy the DB only when the daemon is stopped, or use the SQLite backup API. Log **bodies** are not stored here (segment files on disk; paths may be referenced via process fields).
