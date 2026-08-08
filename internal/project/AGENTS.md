# agents: project

## role
Detect project root and stable filesystem identity key from a cwd. Pure path helpers — no store, no ULID project records. Flag/env (`--project`, `PMMCP_PROJECT`) precedence is the caller's job; this package implements the config/VCS parent walks.

## surface
| Symbol / area | Notes |
|---------------|--------|
| `Detect(ctx, cwd)` | abs+clean; two-pass walk; returns `(root, key, err)` |
| `Key(root)` | abs+clean; `EvalSymlinks` when possible → canonical key |
| config markers | `pmmcp.yaml`, `pmmcp.yml` (pass 1, outranks VCS) |
| vcs markers | `.git` file or dir via `Lstat` (pass 2) |

## deps
- Project: none (leaf)
- Third-party: none

## invariants
- Precedence of walks: nearest `pmmcp.yaml|yml` wins over any `.git`; then nearest VCS; else cwd-as-root
- Always returns a root — no “not found”; caller applies global-scope / required-project policy
- Key is path identity (resolved real path), not a `proj-` ULID
- Marker check is `Lstat` success only — does not parse YAML or open git
- Symlink roots collide after `Key` (link and real path share identity)
- Honors `ctx` cancel during walks; errors wrap `project: detect: …`

## tests
- `project_test.go` — nested `.git`, `.git` file (worktree), yaml/yml, no-marker fallback, cancel
- `symlink_test.go` — Key(real)==Key(link); `coverage_test.go` as needed
- Unit tests hermetic (`t.Parallel` when safe). **Coverage floor: ≥80% statements** for this package (CI). Do not add production seams only to hit residual lines. Meaningful assertions beat line hits.

## touch map
- Daemon `MethodProjectCurrent` and start/list scoping via client cwd
- Declare/import tooling may share markers but does not own detection

## do-not
- Do not mint `proj-` ULIDs or open a project database here
- Do not read git remotes or parse `pmmcp.yaml` contents
- Do not reverse walk order (config must beat VCS)
- Do not treat unresolved symlink path and real path as distinct keys when EvalSymlinks succeeds

## related
- `internal/daemon`, `internal/declare`, / scope-project
