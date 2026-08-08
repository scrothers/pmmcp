# agents: engine/fake

## role
In-memory `engine.Engine` (plus all optional capabilities) for unit tests. Supports forced errors, natural exit, health, list-by-labels — no real containers.

## surface
| Symbol / area | Notes |
|---------------|--------|
| `Engine`, `New()` | Name `"fake"`; Available/Run/Stop/Logs/Inspect/Wait/Remove/Pull/List/Version |
| Test hooks | `Exit`, `SetHealth`, injectable per-op errors, Available toggle |
| Compile-time anchors | Core + all capability interfaces |

## deps
- Project: `internal/engine`
- Third-party: none

## invariants
- Hermetic memory only.
- Not registered in `engine/drivers` production Open.
- Safe for parallel tests when each test owns its Engine instance.

## tests
- `fake_test.go` — contract + hooks + cancel/error overrides.
- Unit tests hermetic (`t.Parallel()` when safe). **Coverage floor: ≥80% statements** for this package (CI). Do not add production seams only to hit residual lines. Meaningful assertions beat line hits.

## do-not
- Ship as a real backend.
- Call host docker/podman.

## related
- `internal/process/container` tests, `engine/drivers` chooseEngine tests
