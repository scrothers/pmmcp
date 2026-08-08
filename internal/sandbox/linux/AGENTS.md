# agents: sandbox/linux

## role
Linux sandbox application: validates `sandbox.Policy`, reports effective mode, provides **bubblewrap** argv rewriting for child isolation, and optional **Landlock** helpers (probe / restrict current thread for tests — not daemon self-lock).

## surface
| Symbol / area | Notes |
|---------------|--------|
| `Applied` | `Profile` + `Mode` |
| `Apply(ctx, pol)` | Strict/standard need project root; Mode `bwrap` if `bwrap` on PATH else `policy` (never claims `landlock` for children in MVP) |
| `TryBwrap` / `TryBwrapPolicy` | Rewrite argv under bubblewrap; masks secret-parent paths; project RO/RW binds |
| `BwrapAvailable`, `IsolationAvailable` | PATH probe |
| `LandlockAvailable`, `LandlockRestrictPaths` | Kernel ABI probe + ruleset; real on `//go:build linux`, stub elsewhere |
| `//go:build !linux` stubs | Landlock unavailable |

## deps
- Project: `internal/sandbox`
- Third-party: `golang.org/x/sys/unix` (Landlock syscalls on Linux)

## invariants
- Fail closed: unknown profile; strict/standard without project root.
- Child FS isolation for local strict/standard is **bwrap** via the local driver — not Landlock on the daemon.
- Mode does not report `landlock` for Apply results in MVP (would overstate child confinement).
- Strict bwrap is allowlist-oriented; standard binds root RO with deny mounts for policy denials.
- Do not Landlock-restrict the long-lived daemon process.

## tests
- `apply_test.go`, `bwrap_test.go` — modes, argv shape, secret-parent masking, policy rejects.
- `landlock_linux_test.go` — ABI/helpers; irreversible restrict uses re-exec helper process.
- Unit tests hermetic (`t.Parallel()` when safe). **Coverage floor: ≥80% statements** for this package (CI). Do not add production seams only to hit residual lines. Meaningful assertions beat line hits.
- Host tool gate: `PMMCP_REQUIRE_SANDBOX` forces hard fail when `bwrap` missing in related process/local tests.

## do-not
- Soften strict when bwrap is missing at the local driver (fail closed there).
- Open host mounts that re-expose denied paths (e.g. `.ssh`).
- Import process/engine packages.

## related
- `internal/sandbox` — policy
- `internal/process/local` — `wrap_linux.go` uses `TryBwrapPolicy`
