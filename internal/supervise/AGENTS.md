# agents: supervise

## role
**Restart policy**, health probe loops (http/tcp/exec), and **boot relaunch** helpers used by the daemon. Not a process manager itself — operates on callbacks / specs above `process.Manager`.

## surface
| Symbol / area | Notes |
|---------------|--------|
| `RestartPolicy`, `RestartCounter` | Max, backoff, stable window reset, exponential + jitter options |
| `ShouldRestart`, `NextBackoff` | Pure policy helpers |
| `HealthCheck`, `ProbeResult`, `ProbeOptions` | http / tcp / exec probes |
| `ProbeHTTP` / `ProbeHTTPOpts`, TCP/exec probes | Loopback SSRF guard by default |
| `ErrSSRF` | Non-loopback health targets blocked unless allowed |
| Loop / relaunch | `loop.go`, `relaunch.go` — periodic health + restart wiring |

## deps
- Project: none required as leaf helpers (uses stdlib net/http); daemon wires process APIs
- Third-party: none load-bearing

## invariants
- Health HTTP/TCP probes are **loopback-bound by default** (SSRF); pin/revalidate redirects; proxy env ignored for probes.
- Stable-window must not credit downtime as healthy uptime.
- Restart unlimited only when Enabled and Max ≤ 0 by design — callers may still cap.
- No ownership of OS children here.

## tests
- `supervise_test.go`, `supervise_more_test.go`, `internal_test.go`, `coverage_test.go`.
- Unit tests hermetic (`t.Parallel()` when safe). **Coverage floor: ≥80% statements** for this package (CI). Do not add production seams only to hit residual lines. Meaningful assertions beat line hits.
- LookupIP seams for SSRF tests via export_test — not production globals for coverage theater beyond existing patterns.

## do-not
- Default-allow non-loopback health URLs.
- Move process spawn logic into this package.
- Log secret material from exec probe output.

## related
- `internal/daemon` supervise loops, `internal/webhook` (separate outbound SSRF)
