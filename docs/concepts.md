# Concepts

This is the mental model for pmmcp. Ten minutes here will make every other guide obvious. If you read only one document, read this one.

---

## The shape of the system

pmmcp splits cleanly into two programs that never share memory and communicate only over a private socket:

```
        you (CLI)            an agent (MCP)
             \                   /
              \                 /   stdio
           pmmcp             pmmcp
          (client)          (client)
              \                 /
               \               /   private socket, one OS user only
                \             /
                 v           v
              ┌───────────────────┐
              │      pmmcpd        │   the daemon — the only authority
              │                   │
              │  processes  logs   │
              │  groups     events │
              │  state      audit  │
              └───────────────────┘
                        │
             spawns & supervises
                        v
        your dev server, worker, database sidecar…
```

**`pmmcpd`, the daemon**, is the single source of truth. It spawns and supervises processes, captures their output, persists all state to a local database, and enforces every rule. It runs as *your* OS user, listens on a socket only your user can open, and exposes no network port and no remote control plane.

**`pmmcp`, the client**, is deliberately dumb. The CLI and the MCP adapter are the same binary in two costumes; both do nothing but translate a request ("start this", "show me the logs") into a call to the daemon and render the answer. The client owns no processes and no state. Close it, and everything it started keeps running — because the daemon, not the client, is the parent.

Why the split? Because an agent's connection is ephemeral and a server is not. If the agent that started your database crashes, the database must not. The daemon outlives every client, so process lifetime is decoupled from the lifetime of whoever asked.

> **There is no `pmmcp daemon` command.** The client is `pmmcp`; the daemon is `pmmcpd`. They are separate binaries. If you find yourself reaching for `pmmcp daemon …`, you want `pmmcpd` instead.

### The daemon is never auto-started

The client will **never** start the daemon behind your back. If you run a command and the daemon isn't up, you get a structured `daemon_unavailable` error with instructions — not a silently-spawned background service.

This matters most for agents. An AI harness pointed at pmmcp cannot decide, on its own, to leave a long-lived daemon running on your machine. Starting `pmmcpd` is always a human, deliberate act (see [Installation](install.md)). See [Operations → daemon_unavailable](operations.md#the-daemon-is-not-running) when you hit it.

---

## The trust boundary

pmmcp's security model has exactly one unit of trust: **the OS user.**

The daemon listens on a Unix domain socket (Linux/macOS) or a named pipe (Windows), created with permissions that only its own user can open, inside a directory only that user can enter. Every incoming connection is checked against the operating system's own peer-credential mechanism (`SO_PEERCRED` on Linux, the pipe's owner SID on Windows). A connection from any *other* OS user is refused before it can send a single byte.

Everything downstream follows from this: an agent connected to your daemon is acting **as you**, with your filesystem access and your privileges — which is exactly why the *sandbox* exists to constrain what the processes it launches can touch, even though the daemon itself runs with your full rights. The sandbox is the wall between "the agent can ask the daemon to run things" and "the things it runs can read your SSH keys."

What pmmcp is **not**: it is not a multi-tenant service, not a network daemon, not a way to control one machine from another. One user, one daemon, one socket. To run pmmcp for two people on one box, each runs their own daemon under their own account — see [Operations → Multiple OS users](operations.md#multiple-os-users).

The full model, including what pmmcp explicitly does *not* defend against, is in [Security](security.md).

---

## Scoping: project, profile, session

Three nested scopes decide *where a process belongs* and *who may touch it*. Understanding them removes almost all surprise from day-to-day use.

### Project — the boundary that matters

Every process, group, and profile belongs to a **project**. A project is a working tree, identified by its canonical path. pmmcp detects it automatically, in this order:

1. an explicit `--project` flag (or `PMMCP_PROJECT` in the environment),
2. otherwise, walk up from your current directory looking for a `pmmcp.yaml`,
3. otherwise, walk up looking for a `.git` root,
4. otherwise, fall back to the current directory.

The practical consequence: **names are unique within a project, not globally.** You can have a process called `web` in your frontend repo and another `web` in your API repo, and they never collide, because they live in different projects. `pmmcp list` shows you *this* project's processes by default; a process's own working directory — not the client's — determines which project it joins.

A project is a *naming and organization* boundary. It is not, by itself, a security boundary — that is the sandbox's job. But a project's root is usually what a strict sandbox makes writable, so the two line up in practice.

### Profile — a named variant within a project

A **profile** is a named bundle of defaults (environment overlays, sandbox posture, restart policy, log and watch settings) that you select per project. `default` is the profile you get if you choose nothing. You might keep a `dev` profile with a relaxed sandbox and verbose logging, and a `staging` profile that mirrors production. Uniqueness is per `(project, profile)`, so the same name can exist once per profile.

A profile is *configuration scoping*. It is **not** a security boundary — do not use profiles to enforce isolation; use the sandbox.

### Session — who is asking, right now

A **session** is one client connection: your CLI invocation, or an agent's MCP conversation. It exists to answer "who did this?" for the [audit log](logs-and-events.md#audit) and to optionally clean up on disconnect. The rules that make it predictable:

- A session maps one-to-one to a client connection. When a harness supplies its own session id, pmmcp uses it (so an agent's actions stay attributable across reconnects); otherwise the daemon mints a `sess-…` id.
- **Processes survive disconnect by default.** Close your terminal, drop the agent — the process keeps running. This is the whole point of the daemon.
- Opt into `stop_on_disconnect` for a process that genuinely should die with its session (a throwaway probe, say).
- Attribution is not a hard lock. A process is *attributed* to the session that started it, but any session belonging to the same OS user, holding the `process:stop` capability in that project, may stop it. The trust unit is the user, not the connection.

---

## Argv, not shell

When you start a process, you give pmmcp a **program and its arguments as a list**:

```
["npm", "run", "dev"]
```

not a string like `"npm run dev"`. pmmcp calls `execve` (or the platform equivalent) directly with that list. **There is no shell in between** — no `sh -c`, no word-splitting, no glob expansion, no `$VAR` interpolation, no `;` / `&&` / `|` / backtick interpretation.

This is a security property, not an inconvenience. A shell string is an injection surface: a value that looks like data (`--name`, a filename, an env value) can smuggle in a command. An argv list cannot — every element is exactly one argument, forever. When an agent constructs a command from untrusted input, this is the difference between "runs the program" and "runs whatever the input said."

If you genuinely need shell features — a pipeline, a redirect — you make the shell *explicit and visible*:

```
["/bin/sh", "-c", "foo | bar > out.log"]
```

Now the shell is your `argv[0]`, in plain sight, and reviewable. pmmcp will never insert one for you. (The one place a shell wrapper is applied is importing a legacy `Procfile`, which is inherently shell-shaped — and even there it is written out explicitly for you to see. See [Declarative](declarative.md#importing).)

---

## Isolation: the sandbox, in one paragraph

A process pmmcp starts runs inside a **sandbox** whose strictness is set by a profile: `strict` (the default), `standard`, `permissive`, or `off`. Strict is deny-by-default — the process sees the project directory and the system runtime it needs, and little else: not `~/.ssh`, not your cloud credentials, not the container socket, and with no outbound network. Loosening the sandbox is an explicit, audited act that requires the `sandbox:relax` capability — which the default agent role does not hold. And it is **fail-closed**: if the platform's isolation mechanism is missing or cannot be applied, the process does not start unsandboxed — it does not start at all. The full behavior, per profile and per OS, is in [Security](security.md).

---

## Drivers: how a process actually runs

The thing you start can be materialized two ways, chosen by a `runtime` setting:

- **`local`** (the default) — a real child process on the host, spawned directly, sandboxed with the host's isolation primitives.
- **`container`** — a container, run through Podman (preferred) or Docker. Used for sidecars: a Postgres, a Redis, anything that ships as an image.

Both are the same to you: the same `start`/`stop`/`status`/`logs` verbs, the same ids, the same supervision. The driver is an implementation detail of *where the bytes run*, deliberately hidden behind one interface. A missing engine is a clear start-time error, never a silent fall-back to running unsandboxed on the host. See [Supervision](supervision.md) and [Security](security.md#containers).

---

## The three streams

pmmcp keeps three kinds of record, and never conflates them. Knowing which is which saves you from looking in the wrong place:

| Stream | What it holds | Ids | You read it with |
|--------|---------------|-----|------------------|
| **Logs** | A process's own stdout/stderr | — | `pmmcp logs`, `grep`, `errors` |
| **Events** | Lifecycle facts: started, exited, became unhealthy, restarted, group converged | `evt-…` | `pmmcp events` |
| **Audit** | Who *did* what through the control plane: who started, stopped, shared, relaxed a sandbox — and who was denied | `aud-…` | `pmmcp audit` |

Logs are the application talking. Events are the *system* narrating what happened to the application. Audit is the *accountability* trail of human and agent actions. A crash is an event; the log tells you why; the audit shows nobody touched it — it fell over on its own. All three are covered in [Logs, events & observability](logs-and-events.md).

---

## State and durability

The daemon persists everything that must survive a restart — process definitions, desired state, groups, profiles, events, and the audit trail — to a single local SQLite database that only the daemon opens. Log bodies live as rotating files on disk, indexed by the database. When the daemon restarts (a reboot, an upgrade), it reads back the desired state and relaunches the processes that were meant to be running. Nothing about your supervised stack depends on a client staying connected. See [Operations → Backup & upgrade](operations.md#backup-and-upgrade).

---

## Identity

Every durable thing pmmcp creates gets a **prefixed [ULID](https://github.com/ulid/spec)**: a lowercase, time-sortable, opaque identifier with a type prefix that tells you what it is at a glance.

| Prefix | Thing |
|--------|-------|
| `proc-` | a process |
| `grp-` | a group |
| `prof-` | a profile |
| `proj-` | a project |
| `sess-` | a session |
| `evt-` | an event |
| `aud-` | an audit record |

You rarely type these — you refer to processes by name within a project — but you'll see them in output, and they're what you quote in a bug report. They sort chronologically as plain strings, so a sorted list of ids is a timeline. A restart mints a *new* `proc-` id linked to its predecessor, so "the web server" across ten restarts is ten process records with a traceable chain, not one record with a lossy history.

---

## Where to go next

You now have the model. Pick your path:

- Put it on a machine → **[Installation](install.md)**
- Run something → **[Quickstart](quickstart.md)**
- Wire it to an agent → **[Agent & MCP integration](mcp.md)** · **[Harness guides](integration/README.md)**
- Understand the walls → **[Security & sandboxing](security.md)**
