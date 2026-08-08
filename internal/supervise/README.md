# supervise

Package `supervise` implements the **policy and probe primitives** for keeping managed processes alive: restart limits and backoff, health HTTP checks, a simple crash-loop helper, boot relaunch eligibility, and status mapping onto `domain.Status`.

It does **not** own process start/stop. The daemon (or any caller) supplies monitor and restart functions and applies the policy results.

Related: restart policy and health checks.

## Responsibilities

| Area | What this package provides |
|------|----------------------------|
| Restart policy | `RestartPolicy`, `ShouldRestart`, `NextBackoff` |
| Crash-loop state | `RestartCounter` with stable-window reset |
| Health probe | `ProbeHTTP`, `ProbeResult`, `HealthCheck` descriptor |
| Loop helper | `CrashLoop` with injectable monitor/restart funcs |
| Boot relaunch | `EligibleForRelaunch` from durable desired state |
| Status mapping | `MapStatus` → `domain.Status` |

## Restart policy

```go
policy:= supervise.RestartPolicy{
 Enabled: true,
 Max: 5, // 0 = unlimited while Enabled
 Backoff: 500 * time.Millisecond,
 StableWindow: 30 * time.Second, // 0 → 30s in NewRestartCounter
}
```

### ShouldRestart

- `Enabled == false` → never restart.
- `restarts < 0` → never restart.
- `Max <= 0` and enabled → unlimited.
- Otherwise restart while `restarts < Max`.

`restarts` is the count of restarts **already performed** in the current window (0 means first failure, not yet restarted).

### NextBackoff

Linear growth: `Backoff * (restarts + 1)`. Zero or negative `Backoff` means immediate retry.

> mentions exponential backoff with jitter as product direction; the current implementation is linear. Callers that need exponential growth can compute delays themselves until this package grows the API.

### RestartCounter

Prefer `RestartCounter` for production loops (used by the daemon auto-restart path):

1. On healthy samples: `ObserveHealthy(now)` — accumulates continuous healthy time; after `StableWindow`, resets `count` to 0.
2. On unhealthy / not running: `ObserveUnhealthy(now)` — clears healthy accumulation (does **not** increment count).
3. After a successful restart: `RecordRestart` — increments count and clears healthy accumulation.
4. Before attempting: `ShouldRestart` / `Count`.

```go
c:= supervise.NewRestartCounter(policy)
//... each tick...
if running && healthy {
 c.ObserveHealthy(now)
 continue
}
c.ObserveUnhealthy(now)
if !c.ShouldRestart {
 continue
}
time.Sleep(supervise.NextBackoff(policy, c.Count))
if err:= restart(ctx, id); err == nil {
 c.RecordRestart
}
```

## Health checks

`HealthCheck` describes a probe (`Type`: `http` | `tcp` | `exec`). Only **HTTP GET** is implemented:

```go
res:= supervise.ProbeHTTP(ctx, "http://127.0.0.1:8080/healthz", 2*time.Second)
if !res.OK {
 // res.Message has status text or error
}
```

Rules:

- Empty URL → unhealthy.
- Default timeout 2s if `timeout <= 0`.
- Status codes in **[200, 400)** count as healthy.
- Response body drained (up to 1 MiB) so connections can close cleanly.
- Uses `http.NewRequestWithContext` and a per-call client timeout.

TCP and exec probes are reserved for later work; the struct exists so higher layers can pass config without inventing types.

## CrashLoop

`CrashLoop` is a self-contained ticker loop for tests or simple embedders:

```go
go supervise.CrashLoop(ctx, 2*time.Second, ids, policy,
 func(ctx context.Context, id string) (running, healthy bool, err error) { /* inspect */ },
 func(ctx context.Context, id string) error { /* restart */ },
)
```

Notes:

- Uses a plain `map[string]int` restart count — **no** stable-window reset (use `RestartCounter` for that).
- On monitor error **or** running+healthy, skips the id.
- Sleeps backoff then restarts; only increments on successful restart.
- Blocks until `ctx` is cancelled.

Production daemon code uses its own ticker (`runAutoRestartLoop`) with `RestartCounter` instead of calling `CrashLoop` directly.

## Boot relaunch

```go
if supervise.EligibleForRelaunch(rec.Desired, supervise.RestartPolicy{}) {
 // start process from durable store record
}
```

Eligibility today:

- `desired == domain.DesiredRunning` → true.
- Anything else → false.
- `RestartPolicy` is accepted for API stability but **ignored** (crash-loop `Enabled` must not gate boot restore).

Callers may further filter (e.g. “already running under the manager”).

## Status mapping

| running | healthy | `MapStatus` |
|---------|---------|-------------|
| false | * | `domain.StatusExited` |
| true | false | `domain.StatusUnhealthy` |
| true | true | `domain.StatusRunning` |

## Who imports this package

| Importer | Usage |
|----------|--------|
| `internal/daemon` (`supervise_loops.go`) | Auto-restart loop: `RestartPolicy`, `RestartCounter`, `NextBackoff`, `ProbeHTTP` |
| `internal/daemon` (`server.go`) | Boot path: `EligibleForRelaunch` |
| `internal/daemon` (`handlers_ext.go`) | `process.health_check`: `ProbeHTTP` |

## Files

| File | Contents |
|------|----------|
| `doc.go` | Package comment |
| `supervise.go` | Policy, counter, probe |
| `loop.go` | `CrashLoop`, `MapStatus`, func types |
| `relaunch.go` | `EligibleForRelaunch` |
| `supervise_test.go` | Unit tests (external package) |

## Testing

```bash
go test./internal/supervise/ -count=1
```

Coverage includes policy edge cases (disabled, max, unlimited, negative restarts), stable-window reset, backoff math, and `ProbeHTTP` via `httptest` (OK, 5xx, timeout, empty URL).

## Non-goals

- Process lifecycle I/O (start/stop/inspect live under `internal/process`).
- Persisting restart counts or policies (store/daemon concern).
- Full product modes (`never` / `on-failure` / `always` named enums) — current API is the enabled/max/backoff shape.
- SSRF policy for health URLs (daemon-configured localhost URLs today).
- Group-level health aggregation (see `internal/group` / ).
