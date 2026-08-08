# agents: store

## role
Leaf interface package for the durable process / desired-state repository. No driver imports. Concrete implementation today is `store/sqlite` only. Daemon opens the database; CLI/MCP never do.

## surface
| Symbol / area | Notes |
|---------------|--------|
| `ProcessStore` | `Migrate`, `Create`, `Get`, `Update`, `UpdateWithCAS`, `Delete`, `List`, `Close` |
| `ProcessFilter` | `ProjectID`, `Status` (`domain.Status`), `Name` — zero = unrestricted |
| `ErrNotFound` | missing id on Get/Update/Delete/CAS miss as gone |
| `ErrConflict` | duplicate PK, live `(project_id,name)` scope, or CAS token mismatch |

## deps
- Project: `internal/domain` (`Process`, `Status`)
- Third-party: none

## invariants
- Interface only — no Open, SQL, or file paths in this package
- Drivers live under `store/<driver>/`; parent never imports them
- Values are `*domain.Process`; validation before write belongs in the driver
- `Update` is last-writer-wins; `UpdateWithCAS` uses `UpdatedAt` as optimistic token
- Errors: sentinels via `errors.Is`; drivers wrap with `%w`
- No `func init`, no package globals, no stored contexts

## tests
- No unit tests in this package (interfaces + sentinels only); contract exercised by `store/sqlite` tests
- Unit tests hermetic (`t.Parallel` when safe). **Coverage floor: ≥80% statements** for this package (CI). Do not add production seams only to hit residual lines. Meaningful assertions beat line hits.

## touch map
- `internal/daemon` holds `store.ProcessStore`; opens sqlite at state_dir
- `internal/store/sqlite` implements + compile-time `var _ store.ProcessStore`
- Event/audit use the shared `*sql.DB` from the sqlite driver, not this interface

## do-not
- Do not import `modernc.org/sqlite` or `database/sql` here
- Do not open the DB from CLI, MCP, or non-daemon product paths
- Do not bend the interface for one driver — add a driver package instead
- Do not put event/audit table APIs on ProcessStore

## related
- `internal/store/sqlite`, `internal/domain`, `internal/daemon`, 
