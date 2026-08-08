# agents: doctor

## role
Local connectivity and config diagnosis against the configured IPC endpoint. Dials and requires a successful Hello handshake; never starts, restarts, or installs the daemon.

## surface
| Symbol / area | Notes |
|---------------|--------|
| `Report` | `OK bool`, `Lines []string` |
| `Check(ctx, endpoint)` | empty endpoint → fail + remediation; unix vs TCP branch |
| unix path | `ipc.Dial` then `Hello` |
| TCP `host:port` | gRPC `Daemon.Call` Hello with insecure credentials + short timeout |
| remediation | `pmmcpd run` / `install-service` + OS-specific enable hints |

## deps
- Project: `internal/api`, `internal/api/gen/pmmcp/v1`, `internal/ipc`
- Third-party: `google.golang.org/grpc` (+ insecure credentials) for TCP Hello only

## invariants
- Never auto-starts `pmmcpd` (product promise: missing daemon → unavailable)
- Path-like endpoints (contain `/` or `\\` prefix) use the private IPC client
- TCP path still requires Hello — a stray listener is not “healthy”
- Major API version skew on TCP fails the report (client vs daemon)
- Remediation always mentions start/install paths appropriate to GOOS
- No config mutation, no service install from this package

## tests
- `doctor_test.go`, `export_test.go` — empty endpoint, missing socket wording, remediation per OS via `remediationFor`, TCP vs path classification
- Unit tests hermetic (`t.Parallel()` when safe). **Coverage floor: ≥80% statements** for this package (CI). Do not add production seams only to hit residual lines. Meaningful assertions beat line hits.

## touch map
- CLI `pmmcp doctor` (and MCP equivalents if any) call `Check` with loaded `ipc.endpoint`
- Pairs with `config.DoctorView` for redacted local config dump at the CLI layer

## do-not
- Do not start `pmmcpd`, load launchd/systemd, or mutate config here
- Do not treat dial-only success as OK without Hello
- Do not swallow dial/hello errors without remediation lines
- Do not open the state DB or store drivers from doctor

## related
- `internal/ipc`, `internal/config`, `internal/api`, root product promise “no surprise daemon spawn”
