# agents: process/drivers

## role
**Selector** for process backends: the only package that imports parent `process` plus concrete drivers (`local`, `container`) and engine drivers. Explicit `Open` wiring — no `func init()`.

## surface
| Symbol / area | Notes |
|---------------|--------|
| `Open(name)` | `"local"` / `""` → local; `"container"` / `"container:auto"` → engine auto; `"container:podman"` / `"container:docker"` |
| `ErrUnknown` | Unregistered driver name |

## deps
- Project: `internal/process`, `internal/process/local`, `internal/process/container`, `internal/engine/drivers`
- Third-party: none

## invariants
- No `func init()` registration.
- Adding a backend = new driver package + one case here; never bend parent `Manager` for one driver.
- Container paths open engines via `engine/drivers` only.

## tests
- `drivers_test.go` — Open local/container names, unknown, auto error propagation (hermetic with engine availability).
- Unit tests hermetic (`t.Parallel()` when safe). **Coverage floor: ≥80% statements** for this package (CI). Do not add production seams only to hit residual lines. Meaningful assertions beat line hits.

## do-not
- Import sibling engines directly (use `engine/drivers`).
- Auto-start daemons or engines.
- Put lifecycle logic here — only selection.

## related
- `internal/process`, `internal/engine/drivers`
