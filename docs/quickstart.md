# Quickstart

Ten minutes, one real process. You'll start a dev server under supervision, find its logs and its port, run a one-shot task, and stop everything cleanly — the whole loop you used to cobble together with `nohup`, `ps`, `grep`, and `kill`.

**Before you start:** the daemon must be running and `pmmcp doctor` must be green. If not, do [Installation](install.md) first.

```bash
pmmcp doctor      # exit 0 = daemon up and compatible
```

---

## 1. Start something

`cd` into a project (a git repo, or any directory) and start a long-running command. Everything after `--` is the program and its arguments, passed through untouched — no shell in between (see [Concepts → Argv, not shell](concepts.md#argv-not-shell)).

```bash
cd ~/code/my-app
pmmcp start --name web -- npm run dev
```

```
started proc-01J9Z…  name=web  pid=48213  status=running  logs=/home/you/.local/state/pmmcp/logs/proc-01J9Z…
```

That process is now owned by the daemon, not your shell. Close this terminal and `web` keeps running. It started under the **strict sandbox** (the default): it can read and write this project's directory and bind loopback ports, but it cannot see `~/.ssh`, your cloud credentials, or the wider filesystem. If your command needs more than that — outbound network, paths outside the project — choose a different profile with `--sandbox` and read [Security](security.md) to understand the trade-off.

pmmcp figured out which **project** you're in from the working directory. The name `web` is unique within *this* project, so a `web` in another repo won't collide.

---

## 2. See what's running

```bash
pmmcp list
```

```
proc-01J9Z…  web  running  pid=48213  npm run dev
```

By default `list` shows the current project (from your cwd). Add `--all` to see every project, or `--include-exited` to include processes that have already stopped. For the full picture of one process — ports, sandbox, health, exit info — ask for its status:

```bash
pmmcp status web
```

That prints a JSON object you can read or pipe to `jq`. You can refer to a process by its `name` (within the project) or by its `proc-…` id anywhere a command takes `<id-or-name>`.

---

## 3. Read its logs

The daemon captures stdout and stderr for you — no more hunting for where output went.

```bash
pmmcp logs web             # tail the last 100 lines
pmmcp grep web "GET /api"  # regex search across captured logs
pmmcp errors web           # just the error-looking lines
```

Logs are rotated and kept on disk (see [Configuration → logs](configuration.md)), so `logs` works even after a restart. For live streaming and structured output, see [Logs, events & observability](logs-and-events.md).

---

## 4. Find its port and URL

A dev server picked a port — you don't have to go read the log to find it:

```bash
pmmcp ports web
```

This shows the ports the process declared and the ones pmmcp actually observed it listening on, with a usable URL. More in [Logs & events → Ports & URLs](logs-and-events.md#ports-and-urls).

---

## 5. Run a one-shot task

Not everything is long-lived. For a migration, a build step, or any run-to-completion job, use `run` — it starts the task, and you can wait for it to finish:

```bash
pmmcp run --name migrate -- ./scripts/migrate.sh
pmmcp wait migrate
```

```
migrate  status=exited  exit_code=0  timed_out=false
```

A one-shot is never auto-restarted, and its exit code is preserved. See [Supervision → One-shot jobs](supervision.md#one-shot-jobs).

---

## 6. Stop cleanly

```bash
pmmcp stop web
```

`stop` is a **tree kill**: it sends `SIGTERM` to the process *and all its descendants*, waits a grace period, then `SIGKILL`s anything left, and releases the ports. No orphaned `node` child clinging to a socket, no `nohup.out` to clean up.

To also forget the record (and optionally its logs):

```bash
pmmcp remove web
```

---

## What you just learned

| You did | The old way | Why pmmcp's way is better |
|---------|-------------|---------------------------|
| `pmmcp start --name web -- npm run dev` | `nohup npm run dev &` | Survives your shell, is named, is sandboxed, logs are captured |
| `pmmcp logs web` | `tail -f nohup.out` | Rotated, searchable, survives restarts |
| `pmmcp ports web` | read the log, guess the port | Declared *and* observed ports with a URL |
| `pmmcp stop web` | `ps`, `grep`, `kill`, hope | Whole process tree, ports freed, no orphans |
| `pmmcp run … && pmmcp wait` | `./migrate.sh; echo $?` | Named, captured, exit code preserved, sandboxed |

---

## Keep going

You've done the imperative basics. The next steps up:

- **Keep it alive.** Health checks, restart-on-crash with backoff, relaunch after reboot → [Supervision & orchestration](supervision.md).
- **Run more than one thing together.** Groups with `depends_on` ordering → [Supervision → Groups](supervision.md#groups).
- **Declare the whole stack in a file.** `pmmcp.yaml` with `apply`/`diff` → [Declarative `pmmcp.yaml`](declarative.md).
- **Let an agent drive it.** The same operations over MCP → [Agent & MCP integration](mcp.md) · [Harness guides](integration/README.md).
- **Master the CLI.** Every command and flag → [CLI reference](cli.md).
