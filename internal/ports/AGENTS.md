# agents: ports

## role
Discover listening TCP endpoints for a host PID (declared vs discovered model). Linux walks `/proc/net/tcp{,6}` and matches socket inodes under `/proc/<pid>/fd`. Other platforms return nil (no discovery).

## surface
| Symbol / area | Notes |
|---------------|--------|
| `DiscoverListeningPorts(pid int) []string` | `host:port` strings; nil/empty when none, bad pid, or non-Linux |

## deps
- Project: none
- Third-party: none (stdlib only)

## invariants
- `pid <= 0` → nil.
- TCP **LISTEN** only (`st == 0A`); no UDP.
- Dedup by address string; order = `/proc/net/tcp` then `tcp6` appearance.
- IPv4/IPv6 hex addresses decoded little-endian per kernel `/proc` layout.
- Unreadable `/proc` → nil/empty, not error (status remains usable).
- Does **not** own declared ports — those come from process/group config (`Ports` vs `Discovered`).
- Build tags: `discover_linux.go` / `discover_other.go`.

## tests
- `discover_linux_test.go` (`//go:build linux`) — self listen on `127.0.0.1:0`.
- `discover_linux_internal_test.go`, `parse_internal_test.go` — table/fd/parser branches with temp files.
- Unit tests hermetic (`t.Parallel()` when safe). **Coverage floor: ≥80% statements** for this package (CI). Do not add production seams only to hit residual lines. Meaningful assertions beat line hits.

## touch map
- Status/ports RPC → daemon fills `Discovered` from `DiscoverListeningPorts(rec.PID)` for local processes.
- Parser/format changes → keep synthetic table tests in internal tests.

## do-not
- Scrape process logs for ports in this package.
- Fail hard when discovery is empty; declared ports still surface in status.
- Import process drivers; accept a PID only.
- Require public ingress, reverse proxy, or non-TCP protocols here.

## related
Primary consumer: `internal/daemon` status/ports paths.
