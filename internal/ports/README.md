# ports

OS-level discovery of listening TCP ports for a process ID.

## Overview

Agents need “where is the server?” after start. pmmcp distinguishes:

- **Declared** ports/URLs from config (desired intent)
- **Discovered** listen addresses from the OS (runtime fact)

This package implements the Linux discovery half: walk the process’s socket inodes and match them against `/proc/net/tcp` and `/proc/net/tcp6` LISTEN rows.

## When to use

- Daemon status handlers: when a local process has a host PID, call `DiscoverListeningPorts(pid)` and attach results to the status view.
- **Not** for allocating free ports, health checks, or container published-port inspect (engine/driver territory).

## Key API

```go
addrs:= ports.DiscoverListeningPorts(pid)
// e.g. ["127.0.0.1:8080", "[::1]:8080"]
// nil/empty if pid invalid, no listeners, non-Linux, or /proc unreadable
```

## Design notes

- **Best-effort.** Restricted containers or hidden `/proc/net` may return empty; callers must still show declared ports.
- **IPv4 and IPv6.** Addresses are normalized with `net.JoinHostPort`.
- **Stable enough for tests.** Order follows table scan; duplicates are suppressed.
- **Cross-platform stub.** Non-Linux builds always return nil so production code stays free of GOOS conditionals at call sites.
- also allows future log scrapers and engine inspect; keep those out of this package unless they share the same pure “pid → []addr” signature.

## Testing

```bash
go test./internal/ports/... # non-linux: stub only
go test -tags= ''./internal/ports/... # on linux runs discover_linux_test
```

Linux test opens a localhost listener and retries discovery briefly for `/proc` registration lag.

## Related

- Port/URL discovery design
- Consumer: `internal/daemon` (status `Discovered` field)
