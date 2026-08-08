# pmmcp documentation

**pmmcp** is an agent-native process manager. It gives an AI agent — or a human at a terminal — a supervised, sandboxed, inspectable place to run long-lived processes, instead of scattering `nohup … &` and orphaned PIDs across a machine.

It is two binaries:

| Binary | Role |
|--------|------|
| **`pmmcpd`** | The daemon. A long-lived, per-user service that owns every managed process, its logs, and all persistent state. |
| **`pmmcp`** | The client. A CLI *and* a stdio [MCP](https://modelcontextprotocol.io) server, both thin front-ends that talk to the daemon over a private socket. |

The daemon does the work; the client only asks. An agent connects the same way you do, through the same API, under the same rules.

---

## Start here

**New to pmmcp?** Read these three, in order — about 15 minutes:

1. **[Concepts](concepts.md)** — the mental model. Two binaries, one trust boundary, and how *project*, *profile*, and *session* scope everything you do. Read this first; every other guide assumes it.
2. **[Installation](install.md)** — build the binaries, install the daemon as a user service, and confirm it with `pmmcp doctor`.
3. **[Quickstart](quickstart.md)** — replace your first `nohup` with a supervised, sandboxed process, then find its logs, its port, and stop it cleanly.

## Guides by task

| I want to… | Read |
|------------|------|
| Understand how pmmcp is put together | [Concepts](concepts.md) |
| Install it and run the daemon as a service | [Installation](install.md) |
| Run my first process | [Quickstart](quickstart.md) |
| Drive it from a terminal | [CLI reference](cli.md) |
| Connect it to an agent (Claude, etc.) | [Agent & MCP integration](mcp.md) · [Harness guides](integration/README.md) |
| Wire a specific IDE / CLI harness | [Harness integrations](integration/README.md) |
| Tune the daemon | [Configuration reference](configuration.md) |
| Understand isolation and the trust model | [Security & sandboxing](security.md) |
| Keep processes healthy, restart them, run groups | [Supervision & orchestration](supervision.md) |
| Read logs, watch events, fire webhooks | [Logs, events & observability](logs-and-events.md) |
| Give processes secrets without leaking them | [Secrets & environment](secrets.md) |
| Declare a whole stack in a file | [Declarative `pmmcp.yaml`](declarative.md) |
| Run, upgrade, back up, and debug the daemon | [Operations runbook](operations.md) |
| Look up an error or exit code | [Error reference](reference-errors.md) |

## Reading paths

- **Operators** running pmmcp on a host: [Concepts](concepts.md) → [Installation](install.md) → [Configuration](configuration.md) → [Security](security.md) → [Operations](operations.md).
- **Agent integrators** wiring pmmcp into a harness: [Concepts](concepts.md) → [Agent & MCP integration](mcp.md) → **[Harness guide](integration/README.md)** for your tool → [Security](security.md) → [Secrets](secrets.md).
- **Humans at the keyboard**: [Quickstart](quickstart.md) → [CLI reference](cli.md) → [Supervision](supervision.md) → [Logs & events](logs-and-events.md).

---

## The five things to know before anything else

1. **The daemon is never started for you.** The client will not silently spawn `pmmcpd`. If the daemon is down, every command fails with a clear `daemon_unavailable` error and tells you how to start it. This is deliberate — an agent cannot conjure a background service you did not ask for.
2. **Processes are argv, never shell.** You pass a program and its arguments as a list (`["npm", "run", "dev"]`), not a string the shell will re-interpret. There is no hidden `sh -c`. See [Concepts → Argv, not shell](concepts.md#argv-not-shell).
3. **Sandboxing is strict by default.** A process you start is isolated unless you explicitly relax it, and relaxing requires a capability the default agent role does not have. A "strict" process cannot read `~/.ssh`. See [Security](security.md).
4. **Everything is scoped to a project.** pmmcp figures out which project you mean from your working directory (git root, or a `pmmcp.yaml`). Names are unique *within* a project, so `web` in one repo never collides with `web` in another. See [Concepts → Scoping](concepts.md#scoping).
5. **One OS user, one daemon, one trust boundary.** The daemon listens on a private socket only its own OS user can reach. It is not a network service and has no remote control plane. See [Security → Trust model](security.md#trust-model).

## Conventions in these docs

- Commands you type are shown with their real flags. Where output matters, it follows in a fenced block.
- `pmmcp …` is the client; `pmmcpd …` is the daemon. They are different binaries — there is no `pmmcp daemon` subcommand.
- Identifiers you'll see (`proc-…`, `grp-…`, `evt-…`, `aud-…`) are prefixed [ULIDs](concepts.md#identity): sortable, opaque, unique.
- Anything marked **fail-closed** means: if pmmcp cannot do the safe thing, it refuses rather than doing the unsafe thing.

License: **Apache-2.0**. Source: `github.com/scrothers/pmmcp`.
