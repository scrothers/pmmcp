# agents: process/fake

## role
In-memory `process.Manager` for unit tests of higher layers (daemon, supervise, router consumers). No real OS children, no sandbox tools, no engines.

## surface
| Symbol / area | Notes |
|---------------|--------|
| `Manager`, `New()` | `var _ process.Manager` |
| Lifecycle | Start/Stop/Wait/Inspect/Signal with in-memory handles; terminal statuses short-circuit |
| Behavior | Duplicate ID / not found / cancel contexts mirror real drivers |

## deps
- Project: `internal/process`, `internal/domain`
- Third-party: none

## invariants
- Hermetic only — no network, no real exec.
- Does not implement sandbox isolation; tests that need fail-closed sandbox use `process/local` or inject errors at higher layers.
- Not a production driver; not registered in `process/drivers.Open`.

## tests
- `fake_test.go` — full Manager contract, cancel paths, signal/stop terminal no-ops.
- Unit tests hermetic (`t.Parallel()` when safe). **Coverage floor: ≥80% statements** for this package (CI). Do not add production seams only to hit residual lines. Meaningful assertions beat line hits.

## do-not
- Register in production selector.
- Soften security tests by always using fake where sandbox must be proven.
- Add real process spawning.

## related
- `internal/process`, consumers under `internal/daemon` tests
