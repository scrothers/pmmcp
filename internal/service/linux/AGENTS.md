# agents: service/linux

## role
Writes **systemd --user** unit `pmmcpd.service` under `~/.config/systemd/user`. Does not run systemctl.

## surface
| Symbol / area | Notes |
|---------------|--------|
| `UnitName` | `pmmcpd.service` |
| `Install(ctx, pmmcpdPath)` | Mkdir user unit dir; write unit with `ExecStart=<quoted path> run` |
| `Uninstall(ctx)` | Remove unit if present |
| `quoteSystemd` | Double-quote + escape `\`, `"`, double `%` for systemd |

## deps
- Project: none
- Third-party: none

## invariants
- Path quoting prevents unit-directive injection; reject control characters in path.
- `Restart=on-failure` only (clean stop honored).
- No `systemctl enable/start` from Install.

## tests
- `linux_test.go` — install body, quoting, uninstall, cancel, bad paths.
- Unit tests hermetic (`t.Parallel()` when safe). **Coverage floor: ≥80% statements** for this package (CI). Do not add production seams only to hit residual lines. Meaningful assertions beat line hits.

## do-not
- Shell-out to systemctl from this package.
- Write system-wide units under `/etc`.
- Leave unquoted paths with spaces/`%` in ExecStart.

## related
- `internal/service` facade
