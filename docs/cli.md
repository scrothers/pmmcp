# CLI reference

Every `pmmcp` command, what it does, and how to invoke it. The CLI is a thin client over the daemon: each command becomes one call and prints the answer. If the daemon is down, any command that talks to it fails with `daemon_unavailable` (exit `3`) — the CLI never starts the daemon for you.

The companion binary `pmmcpd` has exactly two commands: `pmmcpd run` (start the daemon) and `pmmcpd version`. Everything else on this page is `pmmcp`, the client.

---

## Conventions

- **`<id-or-name>`** — refer to a process by its `name` (unique within the current project) or its `proc-…` id. Group, profile, and webhook commands likewise accept a name or a prefixed id (`grp-…`, `prof-…`, `wh-…`).
- **`--json`** *before* a subcommand switches human output to pretty-printed JSON, e.g. `pmmcp --json list`. (After a subcommand, `--json '{…}'` means something else — a raw JSON payload for that command.)
- **`--` (double dash)** separates pmmcp's own flags from the program you're launching. Only `start` and `run` take a child command. Everything after `--` is the child's argv, verbatim and never re-parsed — so the child's own `--flags` pass straight through.
- Refer to processes across projects with `--all` (on `list`) or `--project DIR` / `-C DIR` to scope explicitly. Otherwise the current working directory selects the project.

---

## Diagnostics

```
pmmcp version                 # print the client version  (also: --version, -v)
pmmcp doctor                  # check daemon reachability; prints a report, exit 3 if down
pmmcp whoami                  # your OS user, role, capabilities, and session id
pmmcp daemon-info             # daemon version, uptime, and resolved paths  (also: pmmcp info)
pmmcp reload                  # re-apply the safe subset of daemon config without a restart
```

`doctor` is special: it does not dial the daemon as a normal client, so it works as a first check even before you know the socket is right. It prints where it's looking and whether the daemon answered a version handshake.

---

## Process lifecycle

```
pmmcp start --name NAME [--cwd DIR] [--sandbox PROFILE] [--project|-C DIR] -- CMD [ARGS...]
```
Start a long-lived process. `--name` is required; the command after `--` is required. The process runs under the strict sandbox unless `--sandbox` names another profile (`strict`·`standard`·`permissive`·`off`; relaxing below the default needs a capability — see [Security](security.md)). Prints `started <id> name=<name> pid=<pid> status=<status> logs=<dir>`.

```
pmmcp stop    <id-or-name>          # graceful tree-kill: SIGTERM → grace → SIGKILL, frees ports
pmmcp restart <id-or-name>          # stop, then start again (mints a new proc- id, linked to the old)
pmmcp remove  <id-or-name>          # stop and forget the record   (also: pmmcp rm)
pmmcp update  <id-or-name> KEY=VAL… # change a spec field in place (may rolling-restart)
```

```
pmmcp list [--include-exited] [--all] [--project DIR] [--json]     # also: pmmcp ls
pmmcp status <id-or-name>                                          # full status as JSON
```
`list` is scoped to the current project's cwd by default; `--all` spans every project, `--include-exited` includes stopped processes. Human output is `<id>  <name>  <status>  pid=<pid>  <command…>`.

```
pmmcp run [--name NAME] [--cwd DIR] -- CMD [ARGS...]   # one-shot job (never auto-restarted)
pmmcp wait   <id-or-name>                              # block until the process exits or is healthy
pmmcp enable  <id-or-name>                             # mark durable: desired state = running
pmmcp disable <id-or-name>                             # mark desired = stopped; stop if running
pmmcp health-check <id-or-name>                        # force a health probe now  (also: pmmcp health)
```

`enable`/`disable` set the *desired* state the daemon reconciles toward — an `enable`d process is relaunched after a daemon restart or reboot. See [Supervision](supervision.md).

---

## Groups

Run related processes together with dependency ordering. See [Supervision → Groups](supervision.md#groups).

```
pmmcp group create  KEY=VAL…        # define a group (members, depends_on)
pmmcp group list                    # also: ls
pmmcp group status  <id-or-name>    # aggregate phase + per-member readiness
pmmcp group start   <id-or-name>    # start members in dependency order
pmmcp group stop    <id-or-name>    # stop in reverse order
pmmcp group restart <id-or-name>
pmmcp group remove  <id-or-name>    # also: rm
```

---

## Profiles

Named configuration variants within a project. See [Concepts → Profile](concepts.md#profile--a-named-variant-within-a-project).

```
pmmcp profile list                  # also: ls
pmmcp profile get    <id-or-name>
pmmcp profile create KEY=VAL…
pmmcp profile update <id-or-name> KEY=VAL…
pmmcp profile delete <id-or-name>   # also: rm
pmmcp profile use    <id-or-name>   # select the active profile
```

---

## Logs, events, and observability

```
pmmcp logs   <id-or-name>              # tail recent output (last 100 lines)
pmmcp grep   <id-or-name> <pattern>    # regex search across captured logs
pmmcp errors <id-or-name>              # extract error-looking lines
pmmcp logs export KEY=VAL…             # bundle logs (+events, status) to an archive
pmmcp logs ship   KEY=VAL…             # ship logs to a configured sink
pmmcp events [process-id]              # domain events (last 50): time, id, type, message
pmmcp audit  KEY=VAL…                  # query the audit trail (who did what)
pmmcp metrics                          # point-in-time metrics snapshot
pmmcp ports  [id-or-name]              # declared + observed ports, with URLs
pmmcp runtime KEY=VAL…                 # runtime info (driver/engine details)
```

Full treatment in [Logs, events & observability](logs-and-events.md).

---

## Declarative stacks

Manage a whole project from a `pmmcp.yaml`. See [Declarative `pmmcp.yaml`](declarative.md).

```
pmmcp validate KEY=VAL…      # parse + policy-check pmmcp.yaml (no changes)
pmmcp diff     KEY=VAL…      # show what apply would create/update/remove
pmmcp apply    KEY=VAL…      # reconcile the project to the file
pmmcp declare show KEY=VAL…  # show the effective declaration
pmmcp import   KEY=VAL…      # import a Procfile / compose file into pmmcp.yaml
```

---

## Secrets

Secrets are referenced by name, never printed. See [Secrets & environment](secrets.md).

```
pmmcp secret list                    # list secret NAMES (never values)   (also: ls)
pmmcp secret set NAME < value        # store a secret — value read from STDIN only
pmmcp secret check KEY=VAL…          # verify a secret:// reference resolves
```

> `secret set` reads the value from **stdin**, never argv. Passing `value=…` on the command line is rejected (`invalid_argument`) — so a secret never lands in your shell history or the process table. Pipe or redirect it: `printf %s "$TOKEN" | pmmcp secret set github-token`.

---

## Webhooks, watch, sessions, sharing, projects

```
pmmcp webhook create|update|delete|list|test …   # outbound event webhooks (SSRF-allowlisted)
pmmcp watch set KEY=VAL… | pmmcp watch status     # filesystem watch → restart on change
pmmcp session info … | pmmcp session end …        # inspect / end a session
pmmcp share …  | pmmcp unshare …                  # grant/revoke cross-session access
pmmcp project current … | pmmcp project list      # resolve / list projects
```

Webhooks: [Logs & events → Webhooks](logs-and-events.md#webhooks). Watch: [Supervision → Hot reload](supervision.md#hot-reload). Sharing and sessions: [Security](security.md) and [Concepts → Session](concepts.md#session--who-is-asking-right-now).

---

## The MCP adapter

```
pmmcp mcp
```

Run the client as a stdio [MCP](https://modelcontextprotocol.io) server so an agent can drive pmmcp with the same operations, as tools. Takes no flags or arguments. Configure your harness to launch this — see [Agent & MCP integration](mcp.md).

---

## Host service

```
pmmcp install-service [--bin PATH]    # write a user-service definition (does NOT start it)
pmmcp uninstall-service               # remove the definition
```

`install-service` only writes a definition and prints how to enable it; the daemon is never auto-started. Details and per-OS specifics in [Installation](install.md).

---

## Exit codes

Every command maps its result to a stable exit code, so scripts can branch on it. The code comes from the underlying error's category:

| Exit | Meaning | Error code |
|------|---------|------------|
| `0` | success | — |
| `1` | any other failure | `internal`, `spawn_failed`, `unimplemented`, … |
| `2` | bad arguments / usage | `invalid_argument` |
| `3` | daemon not reachable | `daemon_unavailable` |
| `4` | not found | `not_found` |
| `5` | permission denied | `permission_denied` |
| `6` | name/state conflict | `conflict`, `name_conflict`, `already_exists` |
| `7` | sandbox could not be applied | `sandbox_failed` |
| `8` | client/daemon version mismatch | `ipc_version_mismatch` |

On failure the CLI prints `pmmcp: <code>: <message>` to stderr and exits with the code above. The full catalog of error codes and what to do about each is in [Error reference](reference-errors.md).

---

## See also

- [Quickstart](quickstart.md) — the commands above in a worked flow
- [Agent & MCP integration](mcp.md) — the same operations as MCP tools
- [Operations runbook](operations.md) — when a command doesn't do what you expect
