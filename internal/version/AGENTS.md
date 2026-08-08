# agents: version

## role
Build-time version metadata for `pmmcp` / `pmmcpd` binaries. Package-level strings are overridden at link time with `-ldflags -X`; no runtime git probing.

## surface
| Symbol / area | Notes |
|---------------|--------|
| `Version` | Default `0.0.0-dev`; release semver/tag |
| `Commit` | Default `unknown`; git SHA when stamped |
| `BuildDate` | Default `unknown`; build timestamp when stamped |
| `String()` | Human line: `Version (commit=… date=…)` |

## deps
- Project: none
- Third-party: none (zero imports in production code)

## invariants
- Vars stay package-level so `-X github.com/scrothers/pmmcp/internal/version.*` works; no `init()`.
- Defaults are safe for plain `go test` / unstamped local builds.
- `String()` format is part of CLI/daemon version output contract.
- Does not parse semver or compare versions (IPC version lives in `internal/api`).

## tests
- `version_test.go` — default `String()`; ldflags injection via tiny build with exact `-X` paths.
- Unit tests hermetic (`t.Parallel()` when safe). **Coverage floor: ≥80% statements** for this package (CI). Do not add production seams only to hit residual lines. Meaningful assertions beat line hits.

## touch map
- Rename package/vars → update release/Taskfile `-ldflags` and the ldflags test.
- Version display for agents/API → prefer `Version` alone; humans use `String()`.

## do-not
- Detect git or network at runtime for version.
- Add project package imports or business logic here.
- Add `func init()`.
- Own packaging/release automation beyond stampable fields.

## related
Importers: `cmd/pmmcpd`, `internal/cli`, `internal/daemon` (`DaemonVersion` fields).
