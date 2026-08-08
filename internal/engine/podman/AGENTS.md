# agents: engine/podman

## role
`engine.Engine` implementation via the **Podman CLI**. Available checks binary on PATH (rootless socket discovered by CLI itself). Preferred first candidate for engine auto-select.

## surface
| Symbol / area | Notes |
|---------------|--------|
| `Engine`, `New()` | CLIRunner with binary `podman` |
| Full cap set | Same optional interfaces as docker (shared CLIRunner methods) |
| Compile-time anchors | Core Engine + capabilities |

## deps
- Project: `internal/engine` only
- Third-party: none

## invariants
- Available = `exec.LookPath("podman")` (+ runner Available behavior); package does not open the socket directly.
- Argv-only CLI; missing binary → `engine.ErrUnavailable`.
- No sibling docker import; selector owns auto preference order.

## tests
- `podman_test.go`, `podman_ops_test.go` — name/available/ops via fake CLI helper; `export_test.go`.
- Unit tests hermetic (`t.Parallel()` when safe). **Coverage floor: ≥80% statements** for this package (CI). Do not add production seams only to hit residual lines. Meaningful assertions beat line hits.

## do-not
- Import docker driver or process packages.
- Hard-code privileged/host network for default runs.
- Use shell wrappers.

## related
- `internal/engine`, `internal/engine/drivers` (auto prefers podman)
