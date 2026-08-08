# agents: observability

## role
Point-in-time metrics snapshots for managed processes (RSS/CPU when the OS exposes them) plus daemon goroutine count. Linux reads `/proc`; other platforms leave process counters at zero. No public HTTP scrape endpoint — agents use the snapshot API (`metrics.snapshot`).

## surface
| Symbol / area | Notes |
|---------------|--------|
| `Snapshot`, `ProcMetrics` | `At` UTC, per-process counters, `Goroutines` |
| `ProcRef` | PID + optional start-time for reuse checks |
| `SnapshotProcesses` | id→PID map; no reuse detection |
| `SnapshotVerified` | id→`ProcRef`; zeros metrics on start-time mismatch |
| `ReadStartTime` | Linux: `/proc/<pid>/stat` field 22; other OS: zero, nil error |

## deps
- Project: none
- Third-party: none (stdlib: `runtime`, `time`; Linux also `os`/`strconv`/`strings`)

## invariants
- No network listeners, Prometheus bind, or exporters in this package.
- Linux: RSS from `/proc/<pid>/statm` (pages × page size); CPU from utime+stime / `USER_HZ=100`.
- Unreadable `/proc` or dead PID → zero counters (not hard failure) for metrics reads.
- `procMatches`: zero `StartTime` skips check; read failure fails closed (no attribution).
- Map iteration order is non-deterministic — do not assert slice order unless sorted by caller.
- Build tags: `proc_linux.go` / `proc_other.go`.

## tests
- `observability_test.go`, `observability_extra_test.go` — empty map, self-PID, dead PID, verified reuse.
- `proc_linux_internal_test.go` (` / go:build linux`) — parse helpers, `ReadStartTime`.
- Unit tests hermetic (`t.Parallel` when safe). **Coverage floor: ≥80% statements** for this package (CI). Do not add production seams only to hit residual lines. Meaningful assertions beat line hits.

## touch map
- Metrics RPC → `internal/daemon` builds refs (record start-time at spawn) then `SnapshotVerified`.
- Platform metrics → extend tagged files; keep non-Linux zero semantics.

## do-not
- Add a public always-on metrics HTTP listener here.
- Import process manager or store; accept PIDs/refs only.
- Treat zero RSS/CPU off-Linux as an error.
- Log secrets/PII; metrics are counters only.

## related
Primary consumer: `internal/daemon` (`handlers_ext` metrics path).
