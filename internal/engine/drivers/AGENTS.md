# agents: engine/drivers

## role
**Selector** for container engines: only package importing parent `engine` + all concrete engines. Explicit `Open` / `OpenContext` — no `func init`.

## surface
| Symbol / area | Notes |
|---------------|--------|
| `Open(name)` | `"podman"`, `"docker"`, `"auto"` / empty |
| `OpenContext(ctx, name)` | Same with ctx for availability probes on auto |
| Auto policy | Prefer **podman** if Available, else docker; `Name` records choice |
| `ErrUnknown`, `ErrNoneAvailable` | Unknown name / no candidate available |
| `chooseEngine` (internal; test-exported) | First Available candidate |

## deps
- Project: `internal/engine`, `internal/engine/docker`, `internal/engine/podman`
- Third-party: none

## invariants
- No silent guess when both exist — return whichever wins preference; caller reads `Name`.
- No init-time registration.
- Fake engine is **not** production-registered.

## tests
- `drivers_test.go` — named open, auto/choose branches with fakes, cancel, none available.
- Unit tests hermetic (`t.Parallel` when safe). **Coverage floor: ≥80% statements** for this package (CI). Do not add production seams only to hit residual lines. Meaningful assertions beat line hits.

## do-not
- Register `engine/fake` for production auto.
- Put run argv logic here.
- Add `func init`.

## related
- `internal/engine/{docker,podman}`, `internal/process/drivers`
