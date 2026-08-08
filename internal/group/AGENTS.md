# agents: group

## role
In-memory app-group registry with `depends_on` DAG validation and deterministic start/stop order. Members reference process names/IDs; process lifecycle stays in the daemon.

## surface
| Symbol / area | Notes |
|---------------|--------|
| `Member` | `Name`, `ProcessID`, `Order`, `DependsOn []string` (member names) |
| `Group` | `ID` (`grp-`), `Name`, `ProjectID`, `Members` |
| `Registry` | `NewRegistry`, `Create`, `Get`, `List(projectID)`, `Remove` |
| `StartOrder` / `StopOrder` | topo start; stop = reverse start |
| Sentinels | `ErrNotFound`, `ErrCycle`, `ErrExists` |

## deps
- Project: `internal/id` (`id.New(id.Group)`, `id.HasPrefix`)
- Third-party: none

## invariants
- Empty name rejected; empty ID → `grp-` ULID; non-empty ID must have `grp-` prefix
- Create validates the DAG up front (self-deps, unknown targets, cycles → `ErrCycle`)
- Edge A depends_on B ⇒ B starts before A; duplicate DependsOn edges are deduped
- Kahn tie-break: `Member.Order` first, then input slice position (stable)
- Create/Get return deep clones (no shared member/DependsOn slices)
- List filters by ProjectID when non-empty; sorted by ID for stability
- Thread-safe (`sync.Mutex`); no `func init()`

## tests
- `group_test.go`, `group_more_test.go`, `internal_test.go`, `coverage_test.go` — CRUD, chains, cycles, unknown dep, Order tie-break, clones
- Unit tests hermetic (`t.Parallel()` when safe). **Coverage floor: ≥80% statements** for this package (CI). Do not add production seams only to hit residual lines. Meaningful assertions beat line hits.

## touch map
- Daemon group create/remove/start/stop/status handlers own process exec using StartOrder/StopOrder
- Product/MCP tools map to those handlers, not this package directly

## do-not
- Do not skip cycle validation on write
- Do not silently reorder beyond topo + documented tie-break
- Do not store argv/lifecycle state here — only group membership graph
- Do not return internal map/slice pointers without cloning
- Do not invent non-`grp-` IDs

## related
- `internal/daemon`, `internal/id`, domain group tools in API/CLI catalog
