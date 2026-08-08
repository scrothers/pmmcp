# agents: ipc

## role
Private **gRPC transport** between `pmmcp` clients and `pmmcpd` over **Unix domain sockets** (mode 0600) or **Windows named pipes**. Listen wraps Accept with **same-UID peer credentials** (SO_PEERCRED on Linux). Client Dial + Call; no public TCP control plane.

## surface
| Symbol / area | Notes |
|---------------|--------|
| `Listen(endpoint)` | UDS path or `\\.\pipe\…`; dir 0700; refuse symlink/stale live socket steal |
| `peerFilterListener` | Accept: same UID only; fail closed if creds unreadable |
| `PeerUID` / `AllowedUID` | Linux SO_PEERCRED; non-Linux best-effort (trust 0600) |
| `Client`, `Dial` | gRPC + custom dialer; Hello version handshake; dial fail → `daemon_unavailable` |
| `Call` / streams | JSON payload inside gRPC; session/role attribution |
| `WriteFrame` / `ReadFrame` | Legacy length-prefixed JSON (max 32MiB) — residual/tests, not runtime wire |
| Build tags | `dial_unix` / `dial_windows`, `peercred_linux` / `peercred_other`, namedpipe stubs |

## deps
- Project: `internal/api`, `internal/api/gen/pmmcp/v1`, `internal/domain`
- Third-party: `google.golang.org/grpc` (+ insecure local creds); Windows `github.com/Microsoft/go-winio`; Linux `golang.org/x/sys/unix`

## invariants
- **No default TCP/HTTP admin listener** — private UDS/pipe only.
- Socket **0600**, parent dir **0700**; tighten loose dirs; refuse symlink socket dir/path.
- Peer filter: other UIDs closed; unreadable creds fail closed on Linux path.
- Missing daemon → `domain.CodeDaemonUnavailable`; clients must **not** auto-start `pmmcpd`.
- API major version skew fail closed (`CodeIPCVersionMismatch` via Hello).
- Never log session secrets or full payloads containing secrets.

## tests
- `codec_test.go`, `client_test.go`, `listener_*`, `peercred_test.go`, `peercred_reject_test.go` (linux-oriented reject).
- Unit tests hermetic (`t.Parallel` when safe). **Coverage floor: ≥80% statements** for this package (CI). Do not add production seams only to hit residual lines. Meaningful assertions beat line hits.
- Full Dial against live daemon: integration/e2e tiers.

## touch map
| Change | Also touch |
|--------|------------|
| Listen hardening / peer filter | SECURITY.md, daemon main wire-up |
| Client Hello / version | `internal/api` version, CLI/MCP dial paths |
| Named pipe ACL | Windows packaging/docs |
| Frame helpers | Prefer not resurrecting as runtime wire |

## do-not
- Bind `0.0.0.0` or add public gRPC.
- Skip peer filter or world-readable sockets.
- Auto-start daemon on dial failure.
- Hand-edit `internal/api/gen/**`.
- Import process drivers/daemon into this transport package.

## related
- `internal/daemon` server, `internal/cli` / `mcp` clients, `internal/authz` roles on Call
