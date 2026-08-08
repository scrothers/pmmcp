# agents: sandbox/darwin

## role
macOS sandbox application: validates `sandbox.Policy`, reports effective mode, and (on Darwin) rewrites argv via **sandbox-exec / seatbelt** profiles for strict/standard FS isolation.

## surface
| Symbol / area | Notes |
|---------------|--------|
| `Applied` | `Profile` + `Mode` (`policy` / `off` / `seatbelt` when isolation available) |
| `Apply(ctx, pol)` | Fail-closed: unknown profile, strict/standard without project root |
| `TrySandboxExec(cmd, projectRoot, pol)` | Builds sandbox-exec argv + temp profile; `ok=false` if missing |
| `SandboxExecAvailable`, `IsolationAvailable` | PATH probe for `sandbox-exec` |
| `SeatbeltProfile(projectRoot, pol)` | Deny-default seatbelt text (strict/standard) |
| Stubs (`//go:build !darwin`) | `TrySandboxExec` no-op; `IsolationAvailable` false |

## deps
- Project: `internal/sandbox`
- Third-party: none (stdlib `os/exec` on Darwin)

## invariants
- Strict/standard without project root → error (never silent open host).
- Local driver must fail closed when isolation required and `sandbox-exec` is absent (`process.ErrSandboxFailed`).
- Seatbelt profiles are deny-default; strict limits egress (loopback allowed); no home-wide read of unrelated projects.
- Temp seatbelt files hold path policy only — not secrets.
- Off → Mode `off`; permissive → path policy mode without requiring seatbelt.

## tests
- `apply_test.go` — profile/mode matrix, cancel, empty project root.
- `seatbelt_profile_test.go` — deny-default, SSH/home edges, standard egress.
- `seatbelt_stub_test.go` — non-Darwin stub behavior.
- Unit tests hermetic (`t.Parallel()` when safe). **Coverage floor: ≥80% statements** for this package (CI). Do not add production seams only to hit residual lines. Meaningful assertions beat line hits.
- Real isolation: exercised via `process/local` with `PMMCP_REQUIRE_SANDBOX=1` on macOS when `sandbox-exec` is present.

## do-not
- Report Mode that overstates enforcement when seatbelt is unavailable.
- Call seatbelt restriction on the daemon process itself.
- Import `process` or other drivers.

## related
- `internal/sandbox` — policy types
- `internal/process/local` — `wrap_darwin.go` calls `TrySandboxExec`
