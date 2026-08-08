# agents: service

## role
User-level **daemon service install/uninstall** facade: dispatches to OS packages (`linux` systemd --user, `darwin` LaunchAgent, `windows` logon-task artifacts). Does not start/enable the service automatically.

## surface
| Symbol / area | Notes |
|---------------|--------|
| `Install(ctx, pmmcpdPath)` | Current GOOS dispatch |
| `Uninstall(ctx)` | Current GOOS dispatch |
| `installFor` / `uninstallFor` | Test seam for all OS arms + unsupported OS error |

## deps
- Project: `internal/service/{linux,darwin,windows}`
- Third-party: none

## invariants
- Writes unit/plist/artifacts only — **no** systemctl/launchctl/schtasks enable by default (operator enables deliberately).
- Unsupported OS → clear error suggesting `pmmcpd run`.
- Unlike process/engine parents, this parent **does** import OS packages (thin switch only).

## tests
- `service_test.go` — dispatch via `installFor`/`uninstallFor` without requiring foreign OS.
- Unit tests hermetic (`t.Parallel()` when safe). **Coverage floor: ≥80% statements** for this package (CI). Do not add production seams only to hit residual lines. Meaningful assertions beat line hits.

## do-not
- Auto-start daemon from install.
- Open network control listeners.
- Embed platform unit bodies here (keep in OS packages).

## related
- `internal/service/{linux,darwin,windows}`, packaging docs
