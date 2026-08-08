# Package authz

## Overview

`authz` authenticates clients in the local same-user model and evaluates a capability matrix. Roles (`full`, `operator`, `agent`, `readonly`, `logs`) expand into fine-grained capabilities (`process:start`, `logs:read`, and so on). Cross-session sharing is tracked by an in-memory `ShareBook`.

## When to use

- Daemon handlers: call `authz.Require(principal, cap)` before mutating or reading sensitive resources
- Building a `Principal` from the OS user and requested role pack (`CurrentUser`)
- Checking peer UID against the daemon (`SameUID`)
- Implementing `session.share` / `session.unshare` (`ShareBook`)

## Key types and functions

| Symbol | Purpose |
|--------|---------|
| `Capability` / `Cap*` | Fine-grained permission bits |
| `Role` / `Role*` | Named capability packs |
| `Principal` | Authenticated client (UID, username, role, session) |
| `Caps` | Map of capabilities for a role |
| `Allow` / `Require` | Boolean check vs error on deny |
| `CurrentUser` | Principal for the current OS user |
| `SameUID` | Principal UID equals daemon UID |
| `ShareBook` | Explicit grants: Share, Unshare, Allowed |

## Design notes

- **Same-user trust**: the private IPC endpoint is local (UDS/named pipe); authorization layers role packs and optional shares on top of OS peer identity.
- **Agent vs full**: agents can manage processes and read logs/events/audit but cannot relax sandbox or reconfigure the daemon.
- **Shares**: grants are (target, capability, to_session); used when one session explicitly allows another.
- **Leaf package**: no project imports; easy to unit-test.

## Testing

```bash
go test ./internal/authz/...
```

Includes a table-driven capability matrix covering role packs and expected allow/deny outcomes.

## Related packages

- [`internal/daemon`](../daemon/) — wires principals, `Require`, and `ShareBook`
- [`internal/audit`](../audit/) — records who acted after authorized ops
- [`internal/api`](../api/) — `SharePayload`, role on `Request`
- [`internal/session`](../session/) — session lifecycle (harness ids)
