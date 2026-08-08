# Configuration reference

The daemon reads one optional TOML file, `daemon.toml`. Everything in it has a sensible default, so pmmcp runs with no config at all — the file exists for when you want to change the sandbox posture, log rotation, the socket location, or the webhook allowlist.

The client (`pmmcp`) needs no configuration of its own; it discovers the daemon's socket the same way the daemon computes it.

> **The config is validated strictly.** Unknown keys are a load error, not a silent no-op — a typo'd key name fails loudly rather than being ignored. Only files ending in `.toml` are accepted, and `version` must be `1`.

---

## A complete, annotated `daemon.toml`

Every key below is shown with its default. You only need to write the keys you're changing.

```toml
# daemon.toml — pmmcpd configuration. All keys optional; defaults shown.

version = 1                 # config schema version; must be 1

# Base directory for the database, keyring, logs, and (on Linux fallback) the
# socket. Empty = the platform default (see "Paths" below).
state_dir = ""

[ipc]
endpoint   = ""             # Unix socket path or Windows named pipe. Empty = platform default.
token_file = ""             # path to an IPC auth token (canonical location)

[sandbox]
default = "strict"          # default profile for new processes:
                            #   strict | standard | permissive | off

[logs]                      # rotation for captured process stdout/stderr
max_file_mb = 10            # rotate the active log at this size (MiB)
max_files   = 5             # keep this many rotated files per stream
compress    = true          # gzip rotated files

[relaunch]
enabled = true              # on daemon start, relaunch processes whose desired state is "running"

[webhook]
allowlist = []              # URL prefixes/hosts webhooks may target.
                            # EMPTY = webhooks are disabled entirely (secure by default).
```

### Key reference

| Key | Type | Default | Notes |
|-----|------|---------|-------|
| `version` | int | `1` | Only `1` is valid. |
| `state_dir` | string | platform default | Where all durable state lives. |
| `ipc.endpoint` | string | platform default | Socket / named-pipe path. |
| `ipc.token_file` | string | `""` | Canonical IPC-token location. |
| `sandbox.default` | enum | `"strict"` | `strict`·`standard`·`permissive`·`off`. Applied to any process that doesn't name its own profile. |
| `logs.max_file_mb` | int | `10` | Active-log rotation threshold, MiB. |
| `logs.max_files` | int | `5` | Rotated files kept per stream. |
| `logs.compress` | bool | `true` | gzip rotated files. |
| `relaunch.enabled` | bool | `true` | Boot-relaunch of durable processes. |
| `webhook.allowlist` | []string | `[]` | Empty disables all webhook delivery. See [Logs & events](logs-and-events.md#webhooks). |

> A legacy top-level `token_file` is still accepted as an alias, but `ipc.token_file` wins if both are set. Prefer `[ipc] token_file`.

Two safety-relevant defaults are worth internalizing: **`sandbox.default = "strict"`** means every process is isolated unless it opts out (and opting out needs a capability — see [Security](security.md)), and **`webhook.allowlist = []`** means the daemon will not deliver a single webhook until you explicitly list where they may go (see [Security → SSRF](security.md#outbound-webhooks-and-ssrf)).

---

## Where the config file is found

When you don't pass an explicit path, the daemon looks in this order and uses the **first file that exists**:

1. **`PMMCP_CONFIG`** — if set and non-empty, this exact path is used (no search).
2. The platform config path:

| OS | Config path |
|----|-------------|
| **Linux** | `$XDG_CONFIG_HOME/pmmcp/daemon.toml`, else `~/.config/pmmcp/daemon.toml` |
| **macOS** | `~/Library/Application Support/pmmcp/daemon.toml` (or `$XDG_CONFIG_HOME/pmmcp/daemon.toml` if that variable is set and the file exists) |
| **Windows** | `%APPDATA%\pmmcp\daemon.toml`, else `<home>\AppData\Roaming\pmmcp\daemon.toml` |

If nothing exists, the daemon runs on defaults — that is not an error.

---

## Environment overrides

Six keys can be overridden by environment variables, applied *after* the file is read but *before* defaults fill in. Each takes effect only when set to a non-empty value:

| Variable | Overrides |
|----------|-----------|
| `PMMCP_STATE_DIR` | `state_dir` |
| `PMMCP_IPC_ENDPOINT` | `ipc.endpoint` |
| `PMMCP_SANDBOX_DEFAULT` | `sandbox.default` |
| `PMMCP_TOKEN_FILE` | `token_file` |
| `PMMCP_LOG_LEVEL` | `log.level` |
| `PMMCP_LOG_FORMAT` | `log.format` |

Everything else (log rotation, relaunch, the webhook allowlist) is file-only. `PMMCP_CONFIG` selects the file itself and is not one of these overrides.

> **Client and daemon must agree.** The client finds the socket by computing the same `ipc.endpoint` the daemon does. If you override the endpoint or state directory for the daemon, set the same `PMMCP_IPC_ENDPOINT` / `PMMCP_STATE_DIR` in the client's environment too — otherwise the client dials the wrong path and reports `daemon_unavailable`. For agents, set these in the MCP server's `env` block (see [Agent & MCP integration](mcp.md)).

---

## Paths the daemon computes

When `state_dir` and `ipc.endpoint` are left empty, the daemon derives everything from platform conventions. These are the real locations your logs, database, and socket live at:

### Linux

| What | Path |
|------|------|
| State dir | `$XDG_STATE_HOME/pmmcp`, else `~/.local/state/pmmcp` |
| Database | `<state_dir>/pmmcp.db` |
| Keyring | `<state_dir>/keyring` |
| Per-process logs | `<state_dir>/logs/<proc-id>/{stdout.log,stderr.log}` |
| Log exports | `<state_dir>/exports` |
| IPC socket | `$XDG_RUNTIME_DIR/pmmcp/pmmcpd.sock`, else `<state_dir>/runtime/pmmcpd.sock` |

### macOS

| What | Path |
|------|------|
| State dir | `~/Library/Application Support/pmmcp` |
| Database / keyring / logs | same relative layout as Linux, under the state dir |
| IPC socket | `<temp>/pmmcp-<user>/pmmcpd.sock` |

### Windows

| What | Path |
|------|------|
| State dir | `%LOCALAPPDATA%\pmmcp`, else `<home>\AppData\Local\pmmcp` |
| Database / keyring / logs | same relative layout, under the state dir |
| IPC | named pipe `\\.\pipe\pmmcpd-<username>` |

The socket is created mode `0600` inside a `0700` directory (Linux/macOS); the named pipe uses an owner-only ACL (Windows). Only your OS user can reach it — see [Security → Trust model](security.md#trust-model).

You can always ask the running daemon where it put everything:

```bash
pmmcp daemon-info
```

---

## Applying config changes

`daemon.toml` is read at daemon startup. After editing it, restart the daemon (`systemctl --user restart pmmcpd.service`, or Ctrl-C and re-run `pmmcpd run`). A safe subset of settings can also be re-read live:

```bash
pmmcp reload
```

`reload` (also `pm_daemon_reload` over MCP) re-applies the settings that are safe to change without a restart. Structural changes — the socket path, the state directory — require a full restart. See [Operations](operations.md#reloading-and-restarting).

---

## Next

- **Understand the sandbox default you just saw** → [Security & sandboxing](security.md)
- **Run the daemon as a service** → [Installation](install.md)
- **Wire the endpoint into an agent** → [Agent & MCP integration](mcp.md)
