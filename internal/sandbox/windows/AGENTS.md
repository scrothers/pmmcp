# agents: sandbox/windows

## role
Windows sandbox application: same `Apply` surface as other platforms, but **strict fails closed** in MVP because Job Objects do not provide filesystem confinement. Standard may report best-effort Job Object mode; path policy still denies sensitive reads.

## surface
| Symbol / area | Notes |
|---------------|--------|
| `Applied` | `Profile` + `Mode` (`policy`, `off`, `job-object` for standard when applicable) |
| `Apply(ctx, pol)` | Unknown profile error; strict → `sandbox.ErrStrictUnsupported` (fail closed); strict/standard empty project root still rejected where required |

## deps
- Project: `internal/sandbox`
- Third-party: none

## invariants
- **Strict never runs unconfined** on Windows local processes — `ErrStrictUnsupported` / local `wrapSandbox` → `process.ErrSandboxFailed`. Escape hatch: container runtime or looser profile.
- Job Objects (tree-kill / limits) are applied in `process/local`, not as FS isolation claims here.
- Path policy still denies `.ssh`-style paths for non-off profiles.
- Do not report Mode that implies FS isolation that does not exist.

## tests
- `apply_test.go` — strict fail-closed, standard/permissive/off, cancel, path allows.
- Unit tests hermetic (`t.Parallel()` when safe). **Coverage floor: ≥80% statements** for this package (CI). Do not add production seams only to hit residual lines. Meaningful assertions beat line hits.
- Spawn path needs Windows CI / `process/local` sandbox_windows tests; optional `PMMCP_REQUIRE_SANDBOX`.

## do-not
- “Succeed” strict local starts without container isolation.
- Claim AppContainer/restricted-token FS confinement until implemented.
- Import process packages.

## related
- `internal/process/local` — `wrap_windows.go`, Job Objects, low-integrity token (non-strict)
- `internal/process/container` — escape hatch for strict workloads
