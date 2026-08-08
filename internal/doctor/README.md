# doctor

Diagnoses **daemon connectivity** and prints remediation hints. Used by `pmmcp doctor`.

## Overview

`Check` probes the configured IPC endpoint without starting pmmcpd. On Unix sockets (and other ipc-dialable endpoints) it dials via `ipc.Dial` and runs `Hello` to report API and daemon version. For endpoints that look like `host:port` (no path), it dials TCP only so alternate endpoints can be checked without changing ipc defaults. Failures append human-readable remediation lines.

## When to use

- CLI `doctor` command (`internal/cli` → `doctor.Check`)
- Scripts that need a boolean “is the daemon reachable?” plus actionable text

Not for: health of managed child processes (use daemon health/status APIs).

## Key API

```go
type Report struct {
    OK    bool
    Lines []string
}

func Check(ctx context.Context, endpoint string) Report
```

Example lines on success: `endpoint: …`, `daemon: ok api=… version=…`.  
On failure: dial/hello error plus `remediation: start pmmcpd (pmmcpd run) or pmmcp install-service`.

## Design notes

- Side-effect free relative to process supervision: dial only, close connection.
- TCP vs path split avoids forcing non-product endpoints through unix-socket assumptions.
- Keeps messaging stable so CLI tests can assert on `pmmcpd` / `install-service` strings.

## Testing

Unit tests with a missing socket path and empty endpoint (`t.Parallel()`). No live daemon required for the failure path.

## Related packages

- [`internal/cli`](../cli) — `pmmcp doctor`
- [`internal/ipc`](../ipc) — Dial / Hello
- [`internal/config`](../config) — `IPC.Endpoint`
