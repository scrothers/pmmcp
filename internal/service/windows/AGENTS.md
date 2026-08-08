# agents: service/windows

## role
Writes Windows **logon-task artifacts** (bat + Task Scheduler XML + README) under a user install dir. Does not register the scheduled task itself.

## surface
| Symbol / area | Notes |
|---------------|--------|
| `DirName`, `BatName`, `TaskXMLName`, `ReadmeName` | Artifact names under install dir |
| `Install(ctx, pmmcpdPath)` | Write start bat, task XML, install notes |
| `Uninstall(ctx)` | Remove artifacts |
| `InstallDir()` | Resolved user install directory |

## deps
- Project: none
- Third-party: none

## invariants
- XML/path escaping; reject control characters in path.
- Artifacts are operator-registered (schtasks / Task Scheduler UI) — Install only writes files.
- User-scoped paths only.

## tests
- `windows_test.go` — install/uninstall, escape, percent paths, write failures, cancel.
- Unit tests hermetic (`t.Parallel()` when safe). **Coverage floor: ≥80% statements** for this package (CI). Do not add production seams only to hit residual lines. Meaningful assertions beat line hits.

## do-not
- Invoke schtasks from Install without an explicit product decision.
- Write machine-wide system services as the default.
- Leave secrets in bat/XML.

## related
- `internal/service` facade
