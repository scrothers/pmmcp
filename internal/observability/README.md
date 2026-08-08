# observability

Lightweight process metrics snapshots for agents and daemon RPCs.

## Overview

`SnapshotProcesses` accepts a map of process id → OS PID and returns a UTC timestamped `Snapshot` with:

- Per-process **RSS** and **CPU seconds** when the platform can provide them (Linux `/proc`)
- Current **goroutine** count from the Go runtime

There is intentionally **no** scrape HTTP endpoint in this package. Consumers (daemon MCP/CLI methods) call the snapshot API and return structured results.

## When to use

- Daemon metrics/status enrichment for running local PIDs.
- Tests that assert resource counters exist on Linux for a live PID.
- **Not** for long-term TSDB export, OTel pipelines, or container cgroup accounting (container PIDs may be 0 — counters stay zero unless a host PID is known).

## Key API

```go
snap:= observability.SnapshotProcesses(map[string]int{
 "proc-01AR…": 12345,
})
// snap.At, snap.Goroutines, snap.Processes[i].RSSBytes / CPUSec
```

| Field | Source |
|-------|--------|
| `RSSBytes` | Linux `/proc/<pid>/statm` resident pages × page size |
| `CPUSec` | Linux utime+stime ticks / 100 |
| `Goroutines` | `runtime.NumGoroutine` |

## Design notes

- **Best-effort OS sampling.** Missing `/proc` entries yield zeros so status stays available under restricted environments.
- **Separation of concerns.** Audit trail is `internal/audit`; lifecycle facts are `internal/event`; child stdout/stderr is `internal/logcap`. This package is only numeric counters.
- **Cross-platform.** Non-Linux builds compile a no-op `readProcMetrics` so callers need no GOOS switches.
- Future OTel/statsd exporters should sit above or beside this package, not replace the snapshot API used by agents.

## Testing

```bash
go test./internal/observability/...
```

Hermetic unit tests; self-PID RSS assertion is Linux-conditional.

## Related

- Audit and observability design
- Consumer: `internal/daemon`
