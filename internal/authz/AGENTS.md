# agents: authz

## role
**Capability matrix** and principals for same-OS-user clients, plus **ShareBook** for cross-session grants (`pm_share` / `pm_unshare`). Daemon enforces per-tool; this package is pure policy evaluation (no IPC).

## surface
| Symbol / area | Notes |
|---------------|--------|
| `Capability` | e.g. `process:*`, `logs:*`, `declare:apply`, `sandbox:relax`, `secrets:read_values`, `secrets:write`, webhooks/group/profile/watch/session caps |
| `Role` | `full`, `operator`, `agent`, `readonly`, `logs` |
| `Principal` | UID, Username, Role, Session |
| `Caps` / `Allow` / `Require` | Unknown role → empty set (default deny) |
| `CurrentUser` | OS user lookup; empty role defaults to **agent** (never silent full) |
| `SameUID` | Principal vs daemon UID |
| `ShareBook`, `Grant` | Share / Unshare / Allowed (target, session, cap) |

## deps
- Project: none
- Third-party: none (stdlib `os/user`)

## invariants
- **Default deny** unknown roles/capabilities.
- `secrets:read_values` is **full only** — agents/operators must not read raw secret values via this matrix.
- `sandbox:relax` is operator+ (capability-gated relaxation is audited at call sites).
- ShareBook is explicit grants only; not a substitute for peer UID on the socket.
- Same-user OS model: SameUID is complementary to `ipc` peer filter.

## tests
- `authz_test.go`, `matrix_test.go`, `share_test.go`, `internal_test.go`.
- Unit tests hermetic (`t.Parallel()` when safe). **Coverage floor: ≥80% statements** for this package (CI). Do not add production seams only to hit residual lines. Meaningful assertions beat line hits.

## touch map
| Change | Also touch |
|--------|------------|
| Capability constants / role packs | daemon handler Require calls, the Security wiki page, CLI role docs |
| Default role on empty | IPC session attribution |
| ShareBook semantics | `pm_share` / `pm_unshare` handlers + audit |

## do-not
- Grant `secrets:read_values` to agent/operator.
- Bypass Require at daemon for “tests green” in production paths.
- Treat ShareBook as network multi-tenant auth.

## related
- `internal/daemon` dispatch, `internal/ipc` peer UID, `internal/secret`
