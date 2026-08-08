# agents: service/darwin

## role
Writes macOS **LaunchAgent** plist for `pmmcpd run` under `~/Library/LaunchAgents`. Does not `launchctl load`.

## surface
| Symbol / area | Notes |
|---------------|--------|
| `Label` / `PlistName` | `com.scrothers.pmmcpd` / `.plist` |
| `LegacyLabel` | Pre-1.0 label removed on uninstall |
| `Install(ctx, pmmcpdPath)` | Mkdir LaunchAgents + log dir; write plist |
| `Uninstall(ctx)` | Remove current + legacy plists |

## deps
- Project: none
- Third-party: none (stdlib)

## invariants
- ProgramArguments are argv-style strings (`pmmcpd`, `run`) — path XML-escaped; reject control chars in path.
- KeepAlive honors successful exit (no restart on clean exit); ThrottleInterval set.
- Logs to `~/Library/Logs/pmmcp` (0700 log dir).
- No auto launchctl.

## tests
- `darwin_test.go` — install/uninstall, escape, reject bad paths, cancel (uses temp HOME where needed).
- Unit tests hermetic (`t.Parallel()` when safe). **Coverage floor: ≥80% statements** for this package (CI). Do not add production seams only to hit residual lines. Meaningful assertions beat line hits.

## do-not
- Run `launchctl load` from Install.
- Leave legacy plists on uninstall.
- Inject unescaped path into XML.

## related
- `internal/service` facade
