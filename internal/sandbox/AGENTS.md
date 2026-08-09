# agents: sandbox

## role
Platform-agnostic sandbox **profiles and path policy**. Defines intent (`strict` / `standard` / `permissive` / `off`) and `Policy` helpers (`AllowsRead` / `AllowsWrite`); OS packages under `sandbox/{linux,darwin,windows}` apply that intent. Default OSS posture is **strict**. Parent imports no drivers.

## surface
| Symbol / area | Notes |
|---------------|--------|
| `Profile` | `Strict`, `Standard`, `Permissive`, `Off` |
| Mode consts | `ModePolicy`, `ModeOff`, `ModeLandlock`, `ModeBwrap`, `ModeContainerSubstitute` |
| `Policy` | Writable roots, read roots/denies; project-root aware |
| `DefaultPolicy(profile, projectRoot)` | Builds policy; unknown profile → `ErrUnknownProfile` |
| `Valid`, `AllowsRead`, `AllowsWrite`, `HasProjectRoot` | Fail-closed path checks; SSH-style denials (`.ssh`) |
| Sentinels | `ErrUnknownProfile`, `ErrProjectRootRequired`, `ErrStrictUnsupported` |

## deps
- Project: none (leaf)
- Third-party: none (stdlib only)

## invariants
- Unknown profiles are rejected; never treated as open host.
- Strict/standard path policy requires a real project root (`HasProjectRoot`); empty root is not silent open host.
- Default read-deny includes SSH key paths (portable slash handling).
- `AllowsRead` / `AllowsWrite` fail closed on empty/unknown profile.
- Platform Apply lives in child packages; this package does not claim kernel enforcement.

## tests
- `policy_test.go` — profiles, default policy, read/write allows/denies, project root edges.
- Unit tests hermetic (`t.Parallel()` when safe). **Coverage floor: ≥80% statements** for this package (CI). Do not add production seams only to hit residual lines. Meaningful assertions beat line hits.

## touch map
| Change | Also touch |
|--------|------------|
| Profile / mode vocabulary | Platform `Apply`, `process/local` wrap, the Security wiki page |
| Default deny roots (e.g. `.ssh`) | Platform tests; declare policy if YAML defaults change |
| `ErrStrictUnsupported` semantics | `sandbox/windows`, `process/local` Windows wrap |

## do-not
- Import OS drivers or process packages (keep parent driver-free).
- Soften strict to no-op when isolation is missing — that is a platform `Apply` / local-driver fail-closed duty.
- Log or store secret paths as values.

## related
- `internal/sandbox/{linux,darwin,windows}` — platform Apply + isolation helpers
- `internal/process/local` — wraps argv with bwrap / sandbox-exec / Windows fail-closed
