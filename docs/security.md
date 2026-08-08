# Security & sandboxing

pmmcp runs other people's code — including code an AI agent wrote a moment ago — on your machine. This page is the honest account of what protects you, what doesn't, and the choices you control. Read it before you point an agent at a daemon.

The one-sentence model: **the OS user is the unit of trust, and the sandbox is the wall between "the agent can ask the daemon to run things" and "the things it runs can touch your secrets."**

---

## Trust model

There is exactly one trust boundary: **the operating-system user.**

The daemon listens on a Unix domain socket (mode `0600`, inside a `0700` directory) on Linux and macOS, or an owner-only named pipe on Windows. Every connection is checked with the OS's own peer-credential mechanism — `SO_PEERCRED` on Linux, the pipe's owner SID on Windows — and a connection from any *other* OS user is refused before it sends a byte. There is no TCP port, no network listener, and no remote control plane. pmmcp is not multi-tenant.

The consequence, stated plainly: **a client connected to your daemon acts as you.** It has your filesystem access and your privileges, because it *is* you, as far as the OS is concerned. The CLI you run and the MCP adapter an agent drives are both such clients. This is why the interesting protection isn't at the connection — it's at the sandbox, which constrains what the *processes* the daemon launches can reach, even though the daemon itself runs with your full rights.

To run pmmcp for two people on one machine, each runs their own daemon under their own account. See [Operations → Multiple OS users](operations.md#multiple-os-users).

---

## The sandbox

Every process the daemon starts runs inside a sandbox whose strictness comes from a **profile**. The default is `strict`, and it is **fail-closed**: if the platform's isolation mechanism is unavailable or can't be applied, the process does not start unsandboxed — it does not start at all (`sandbox_failed`, exit 7).

### The four profiles

| Profile | Secret paths | Network | Use it for |
|---------|:---:|:---:|-----------|
| **`strict`** *(default)* | **denied** | **loopback only** — external egress blocked | Untrusted or agent-authored commands; anything that doesn't need the internet |
| **`standard`** | **denied** | shared (host network) | Dev servers that need to reach the network but still shouldn't read your credentials |
| **`permissive`** | allowed | shared | Trusted local work where you want isolation off the critical paths but not the fetters |
| **`off`** | — | — | No child isolation. Requires the `sandbox:relax` capability; always audited |

"Secret paths denied" means these are unreadable by the process, on every restrictive profile:

- `~/.ssh` (and any `/.ssh` tree)
- `~/.gnupg`
- `~/.aws`
- the container engine socket (`docker.sock`)

So even a `standard`-sandboxed dev server — which *can* use the network — still cannot read your SSH keys or your AWS credentials. That's the deliberate split: `strict` withholds the network too; `standard` keeps the network for convenience but keeps the secrets locked.

A strict process **can** read and write its **project directory** and the read-only system trees it needs to run (`/usr`, `/bin`, `/lib`, `/lib64`, `/etc`, `/proc`, `/dev` on Linux). Everything else is denied by default — it's an allowlist, not a blocklist, so a credential you've never heard of in a directory pmmcp doesn't know about is denied simply because it wasn't allowed.

### Per-OS mechanism

The profile *names* mean the same thing everywhere; the enforcement differs by what each OS provides:

| OS | Mechanism | Notes |
|----|-----------|-------|
| **Linux** | `bubblewrap` (`bwrap`) namespaces + Landlock, with `--unshare-net` for strict egress control | Requires `bwrap`. Read-only system trees, project writable, secret trees and the engine socket masked. Strict adds a fresh network namespace (loopback up, egress off). Fail-closed if `bwrap` is missing. |
| **macOS** | `sandbox-exec` (Seatbelt) profile | Denies the same secret trees; fail-closed if unavailable. |
| **Windows** | Job Object + restricted token | Isolation strength is more limited than Linux; strict may require a container runtime rather than a host process. |

Because the mechanisms differ, so does their strength — see [Non-guarantees](#what-pmmcp-does-not-protect-you-from) below. On Linux with `bwrap`, strict is genuinely strong.

### Choosing a profile

You pick per process (`--sandbox` on the CLI, `sandbox` in a tool call or `pmmcp.yaml`), or set the daemon-wide default in [`daemon.toml`](configuration.md):

```toml
[sandbox]
default = "strict"
```

Leave the default at `strict`. Loosen a *specific* process when it genuinely needs more — and know that the declarative path (`pmmcp.yaml`) refuses `sandbox: off` outright (`sandbox_required`), because a checked-in file should never silently disable isolation. See [Declarative](declarative.md#what-gets-rejected).

---

## Capabilities and roles

Operations at the daemon are gated by **capabilities**, bundled into **roles**:

| Role | Holds | Notably lacks |
|------|-------|---------------|
| `readonly` | read/inspect only | all mutations |
| `agent` *(default for an unset role)* | process lifecycle, logs, declare, workspace management | `sandbox:relax`, `logs:export`, `secrets:read_values`, `daemon:configure` |
| `operator` | agent **+** `sandbox:relax`, `logs:export`, `daemon:configure` | `secrets:read_values` |
| `full` | operator **+** `secrets:read_values` | — |

Relaxing a process below the daemon's default sandbox requires `sandbox:relax`; the `agent` role does not have it. Every mutating action — and every *denial* — is written to the [audit log](logs-and-events.md#audit).

**An honest caveat about the shipped client.** Under the one-OS-user trust model, the bundled `pmmcp` client (both the CLI and the stdio MCP adapter) connects to your daemon with the `full` role. So in practice a local client — including an agent's adapter — is fully capable, *including* the ability to relax a sandbox. This is consistent with the trust model (the connection already acts as you), but it means **you should not rely on the role matrix to constrain a local agent.** The guarantees you *can* rely on are the ones that hold regardless of role:

1. **The daemon is never auto-started** — an agent can't create the service.
2. **Strict-by-default, fail-closed sandboxing** — what a launched process can *touch* is constrained by the sandbox, and disabling it is recorded.
3. **A complete audit trail** — every start, stop, share, and sandbox relaxation, plus every denial, is attributable.

The role/capability packs matter most for a future networked or delegated deployment; today, treat local daemon access as equivalent to shell access, and lean on the sandbox and audit trail.

---

## Secrets

Secret **values never travel in tool arguments** (the one exception is `pm_secret_set`, whose whole job is to store a value you hand it — read from stdin on the CLI, never argv). Processes receive secrets by *reference* (`secret://…`) or env-file, resolved into the child's environment only. Secret values are **redacted** everywhere they might otherwise surface — logs, events, status dumps, exports, webhook bodies — using the stable placeholder `***REDACTED:NAME***`. The keyring is enforced `0600`; env-files should be too (pmmcp warns if they aren't). The full mechanics are in [Secrets & environment](secrets.md).

---

## Outbound webhooks and SSRF

Webhooks are the only thing pmmcp sends over the network, and they are locked down by default:

- **Disabled unless you allowlist targets.** `webhook.allowlist` is empty by default, which means *no webhook is delivered at all* until you list where they may go. There is no "any URL" mode.
- **Blocked destinations:** loopback, private and link-local ranges, and cloud metadata endpoints (`169.254.169.254`) are refused.
- **DNS-rebind safe:** the resolved IP is pinned and validated at dial time, and redirects are re-validated at every hop — a hostname can't resolve to an allowed IP and then swing to a blocked one.
- **Fail-closed:** if resolution fails or a target isn't allowlisted, delivery is refused and audited.

Details and payload signing (HMAC) in [Logs & events → Webhooks](logs-and-events.md#webhooks).

---

## Containers

A `container`-runtime process is isolated by the engine, not the host sandbox. pmmcp runs it hardened: capabilities dropped, no new privileges, a read-only root filesystem, no host networking, ports published to loopback, and the container engine socket never mounted in. A missing engine is a start-time error, never a silent fall-back to running unsandboxed on the host. See [Supervision → Container runtime](supervision.md#container-runtime).

---

## What pmmcp does *not* protect you from

Security is only real if its limits are stated:

- **A client already running as your user.** The trust unit is the OS user. Malware running as you can reach the socket the same way your shell can. pmmcp constrains the *processes it launches*; it is not a defense against code already executing with your identity.
- **Uniform strength across OSes.** Linux + `bwrap` strict is strong. macOS Seatbelt and Windows Job Objects are meaningfully weaker envelopes. Don't assume a `strict` process on Windows is as contained as on Linux.
- **`sandbox: off`.** If you (or an `operator`/`full` client) turn the sandbox off, there is no wall. That's why it needs a capability and is audited.
- **Cross-user isolation.** pmmcp is single-user by design. It is not a mechanism for isolating tenants on a shared box.
- **An explicit shell you name.** pmmcp never inserts a shell, but if you pass `["/bin/sh","-c", …]`, that shell runs. The declarative validator rejects that shape; the imperative `pm_start` trusts it. Review agent-authored commands like you'd review a PR.

---

## A safe default posture

- Keep `sandbox.default = "strict"`.
- Give processes secrets by reference, never by value; keep env-files `0600`.
- Leave `webhook.allowlist` empty unless you truly need outbound webhooks, then list exact hosts.
- Run the daemon as an unprivileged user, one per person.
- Watch the [audit log](logs-and-events.md#audit) — it's your record of everything the agent did.

---

## See also

- [Concepts → Trust boundary](concepts.md#the-trust-boundary)
- [Agent & MCP integration](mcp.md#what-an-agent-can-and-cannot-do)
- [Secrets & environment](secrets.md)
- [Error reference](reference-errors.md) — `permission_denied`, `sandbox_failed`
