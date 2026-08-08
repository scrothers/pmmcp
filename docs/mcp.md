# Agent & MCP integration

pmmcp is agent-native: the same operations you run from the CLI are exposed to an AI agent as [MCP](https://modelcontextprotocol.io) tools. An agent gets a supervised, sandboxed place to run processes — a real answer to "start the dev server and watch it" that isn't `nohup` and isn't a shell it can wander out of.

This guide covers the **protocol**: how the stdio adapter works, the tool catalog, resources, prompts, and what an agent can and cannot do.

**Per-harness install steps** (Claude Code, Grok Build, OpenCode, Codex, Cursor, Windsurf, VS Code, Gemini CLI, Cline, Continue, Goose, Zed, Amp, and generic hosts) live under **[docs/integration/](integration/README.md)**.

---

## How it connects

The client runs as a stdio MCP server:

```bash
pmmcp mcp
```

That's the whole command — no flags. It speaks MCP over stdin/stdout using the official Go SDK, and for each tool call it dials your daemon over the private socket. It is a **client**: it owns nothing, and it connects to the daemon **as your OS user**. Point your harness at it (shape varies — JSON `mcpServers`, TOML `mcp_servers`, OpenCode `mcp` + `command` array, etc.):

```json
{
  "mcpServers": {
    "pmmcp": {
      "command": "/usr/local/bin/pmmcp",
      "args": ["mcp"],
      "env": {
        "PMMCP_PROJECT": "/home/you/code/my-app"
      }
    }
  }
}
```

Copy-paste configs for each major product: **[Harness integrations](integration/README.md)**.

- **`PMMCP_PROJECT`** pins the project so the agent's actions land in the repo you mean, regardless of the adapter's working directory. Omit it to fall back to cwd-based [project detection](concepts.md#scoping).
- If you've moved the daemon's socket or state dir, set the matching **`PMMCP_IPC_ENDPOINT`** / **`PMMCP_STATE_DIR`** here too, or the adapter dials the wrong path. See [Configuration → Environment overrides](configuration.md#environment-overrides).

Tools surface to the agent as `pm_*` (a harness may prefix them, e.g. `pmmcp__pm_start`).

---

## The daemon must already be running

**The adapter never starts the daemon.** If `pmmcpd` isn't up, every tool call returns an error result whose text is `daemon_unavailable: dial daemon: …`, marked **retryable**. The agent should surface that to you — "the pmmcp daemon isn't running; start it with `pmmcpd run` or enable the service" — and *not* try to background anything itself.

This is a deliberate boundary: an agent cannot decide, unprompted, to leave a long-lived daemon running on your machine. You start it once (see [Installation](install.md)); from then on the agent just uses it. When you see `daemon_unavailable`, it's [Operations → The daemon is not running](operations.md#the-daemon-is-not-running).

---

## The tool catalog

Sixty-five tools, mirroring the CLI one-to-one. Grouped:

| Group | Tools |
|-------|-------|
| **Meta / daemon** | `pm_whoami` · `pm_daemon_info` · `pm_daemon_reload` · `pm_project_current` · `pm_project_list` |
| **Lifecycle** | `pm_start` · `pm_stop` · `pm_restart` · `pm_update` · `pm_remove` · `pm_list` · `pm_status` · `pm_run` · `pm_wait` · `pm_enable` · `pm_disable` · `pm_health_check` |
| **Groups** | `pm_group_create` · `pm_group_remove` · `pm_group_list` · `pm_group_status` · `pm_group_start` · `pm_group_stop` · `pm_group_restart` |
| **Profiles** | `pm_profile_list` · `pm_profile_get` · `pm_profile_create` · `pm_profile_update` · `pm_profile_delete` · `pm_profile_use` |
| **Session / share** | `pm_session_info` · `pm_session_end` · `pm_share` · `pm_unshare` |
| **Logs** | `pm_logs` · `pm_grep` · `pm_errors` · `pm_logs_export` · `pm_logs_ship` · `pm_logs_subscribe` · `pm_logs_unsubscribe` |
| **Events / audit / metrics** | `pm_events` · `pm_events_subscribe` · `pm_events_unsubscribe` · `pm_events_subscriptions` · `pm_audit_query` · `pm_metrics_snapshot` |
| **Declare** | `pm_validate` · `pm_diff` · `pm_apply` · `pm_declare_show` |
| **Ports / runtime / sandbox** | `pm_ports` · `pm_runtime_info` · `pm_sandbox_profiles` |
| **Secrets** | `pm_secret_list` · `pm_secret_ref_check` · `pm_secret_set` |
| **Watch / webhooks** | `pm_watch_set` · `pm_watch_status` · `pm_webhook_create` · `pm_webhook_update` · `pm_webhook_delete` · `pm_webhook_list` · `pm_webhook_test` |
| **Import** | `pm_import` |

Each has the same semantics as its CLI cousin — cross-reference the [CLI reference](cli.md) for behavior.

### The tools an agent uses most

- **`pm_start`** — start a process. Inputs: `name` (required), `command` (required, an **argv array** — note the field is `command`, e.g. `["npm","run","dev"]`), `cwd`, `sandbox`. Returns `{ id, name, pid, status, log_dir, … }`. The daemon accepts more fields than the tool schema advertises (env-files, ports, runtime, profile, memory limits) — a harness may pass them through.
- **`pm_status`** / **`pm_list`** — inspect. `pm_status` returns a full view (ports, sandbox, health, exit info); `pm_list` takes `project`/`status` filters and returns an array of process views.
- **`pm_logs`** — read output. Inputs `id`/`name`, `stream`, `lines`; returns text, not JSON. Pair with `pm_grep` / `pm_errors`.
- **`pm_run`** — a one-shot task; set `wait` to block for the result (`{ id, status, exit_code, timed_out }`).
- **`pm_apply`** — reconcile a `pmmcp.yaml`. Inputs are the YAML/path and current running names; it creates the services in the diff and returns `{ created, diff }`. *(There is no `start` flag on apply — it always creates the diffed services.)*

---

## Resources and prompts

The adapter also exposes read-only **resources** (for an agent to pull context without a tool call) and **prompts** (guided workflows).

**Resources** — `pmmcp://…`:

| URI | Content |
|-----|---------|
| `pmmcp://processes` | current managed processes |
| `pmmcp://daemon` | daemon info (paths redacted) |
| `pmmcp://project/current` | the resolved project |
| `pmmcp://declare` | this project's `pmmcp.yaml` |
| `pmmcp://ports` | declared + observed ports |
| `pmmcp://events/recent` | recent domain events |
| `pmmcp://docs/error-codes` | the error-code reference |
| `pmmcp://docs/tool-index` | a short tool index |
| `pmmcp://project/{id}` | project metadata by id |
| `pmmcp://process/{name-or-id}` | one process's status |
| `pmmcp://process/{name-or-id}/log` | recent log lines for a process |
| `pmmcp://group/{name}` | a group's status |

**Prompts:**

| Prompt | Purpose |
|--------|---------|
| `pmmcp_start_safe` | start a process the right way — argv, strict sandbox |
| `pmmcp_debug_crash` | diagnose a crash via status → logs → errors → events |
| `pmmcp_apply_stack` | validate → diff → apply a `pmmcp.yaml` |
| `pmmcp_import_compose` | import a Procfile/compose, review warnings before applying |
| `pmmcp_oneshot_task` | run a one-shot task with `pm_run` and wait |

---

## What an agent can and cannot do

The honest model — because "an agent is driving your machine" deserves a precise answer.

**The containment that actually holds is the sandbox, not the connection.** The adapter connects to your daemon as your OS user (that is the single [trust boundary](concepts.md#the-trust-boundary)), so treat an agent with pmmcp access as able to *ask* the daemon to do anything you could. What stops an agent-launched process from reading your SSH keys or exfiltrating over the network is the **strict, fail-closed [sandbox](security.md)** every process gets by default — not a property of the MCP transport. Rely on it:

1. **Strict sandbox by default, fail-closed.** A process started without a named profile is strict: project directory only, no `~/.ssh`, no credentials, loopback-only network. If the sandbox can't be applied, the process doesn't start. Relaxing it is possible for a connected client but is always recorded in the [audit log](logs-and-events.md#audit).
2. **No daemon auto-start.** Covered above — the agent can't spin up the service.
3. **Argv, not shell.** Always pass `command` as an argv array. pmmcp never wraps your command in a shell, so a value can't smuggle in a second command. (A caller *can* pass an explicit `["/bin/sh","-c",…]`; the [declarative validator](declarative.md) rejects that shape in `pmmcp.yaml`, but the imperative `pm_start` trusts an explicit shell you name — so review agent-authored commands the way you'd review any code.)
4. **Secrets by reference.** Never put a secret *value* in a tool argument. Give processes secrets by `secret://` reference or env-file, and read [Secrets](secrets.md). The one intentional write path is `pm_secret_set`, which stores a value you provide.
5. **Everything mutating is audited.** Start, stop, share, sandbox-relax, and denials all land in the audit trail with the acting session — `pmmcp audit` or `pm_audit_query` answers "what did the agent do?"

For the complete capability and trust model, read [Security](security.md).

---

## Troubleshooting

| Symptom | Cause | Fix |
|---------|-------|-----|
| Every tool returns `daemon_unavailable` | daemon not running, or wrong socket | Start `pmmcpd`; if you overrode paths, set `PMMCP_IPC_ENDPOINT` in the adapter `env`. [More](operations.md#the-daemon-is-not-running) |
| Agent's processes land in the wrong project | cwd-based detection picked another root | Set `PMMCP_PROJECT` in the adapter `env` |
| `sandbox_failed` on start | strict sandbox couldn't be applied (e.g. no `bwrap`) | Install the mechanism, or choose a profile the host supports. [Security](security.md) |
| `ipc_version_mismatch` | client and daemon are different versions | Rebuild both from the same source. [Operations](operations.md#version-mismatch) |

---

## See also

- **[Harness integrations](integration/README.md)** — Claude, Grok, OpenCode, Codex, Cursor, and more
- [Concepts](concepts.md) — the model the agent operates within
- [Security & sandboxing](security.md) — the guarantees you're relying on
- [Secrets & environment](secrets.md) — giving processes credentials safely
- [CLI reference](cli.md) — the same operations, for humans
