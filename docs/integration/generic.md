# Generic stdio MCP host

Any agent or IDE that can spawn a **local MCP server over stdio** can use pmmcp. If your harness is not listed in [README.md](README.md), use this recipe.

**Prerequisites:** [integration README](README.md) — daemon running, `pmmcp doctor` green.

Protocol details: [Agent & MCP integration](../mcp.md).

---

## Contract

| Item | Value |
|------|--------|
| Transport | **stdio** (stdin/stdout JSON-RPC per MCP) |
| Command | Path to the `pmmcp` binary |
| Arguments | Exactly one: `mcp` |
| Working directory | Irrelevant if `PMMCP_PROJECT` is set |
| Lifecycle | Host spawns adapter per session; **does not** start `pmmcpd` |
| Network | None required for the adapter; it dials a **local** IPC endpoint |

```bash
pmmcp mcp
# process stays up until the host closes stdin / sends EOF
```

Smoke-test outside the harness:

```bash
# Should block waiting for MCP initialize on stdin; Ctrl-C to stop
pmmcp mcp
```

If that fails immediately, fix install/PATH before debugging the harness.

---

## Environment variables

| Variable | Purpose |
|----------|---------|
| **`PMMCP_PROJECT`** | Absolute project root (strongly recommended) |
| `PMMCP_IPC_ENDPOINT` | Must match daemon if non-default |
| `PMMCP_STATE_DIR` | Must match daemon if non-default |
| `PMMCP_CONFIG` | Alternate client config file (rarely needed) |

Full list: [Configuration → Environment overrides](../configuration.md#environment-overrides).

---

## Minimal JSON (widely copied)

```json
{
  "mcpServers": {
    "pmmcp": {
      "command": "/usr/local/bin/pmmcp",
      "args": ["mcp"],
      "env": {
        "PMMCP_PROJECT": "/absolute/path/to/project"
      }
    }
  }
}
```

## Minimal TOML

```toml
[mcp_servers.pmmcp]
command = "/usr/local/bin/pmmcp"
args = ["mcp"]
env = { PMMCP_PROJECT = "/absolute/path/to/project" }
```

## Minimal “command array” style (OpenCode-like)

```json
{
  "type": "local",
  "command": ["/usr/local/bin/pmmcp", "mcp"],
  "environment": {
    "PMMCP_PROJECT": "/absolute/path/to/project"
  }
}
```

---

## What success looks like

1. Host completes MCP `initialize` / `tools/list`.
2. Tool list includes many `pm_*` names (≈65).
3. `pm_daemon_info` or `pm_list` returns data, not `daemon_unavailable`.
4. Optional: resources `pmmcp://…` and prompts `pmmcp_*` if the host implements those MCP features.

---

## Hosts that often do **not** fit

| Situation | Why |
|-----------|-----|
| Cloud-only agent with no local process spawn | Cannot reach your user socket |
| Host supports only remote HTTP MCP | pmmcp ships **stdio** only (today) |
| Host runs tools inside a disposable container without the host socket | Adapter cannot dial `pmmcpd` unless you mount/IPC carefully |

For remote development (SSH), install and run **both** `pmmcp` and `pmmcpd` **on the remote user session**, and configure the harness’s remote MCP to spawn remote `pmmcp mcp`.

---

## Security checklist

- [ ] Daemon is user-local, not exposed on a network port
- [ ] Agent-started processes stay on **strict** sandbox unless intentionally relaxed
- [ ] Secrets via `secret://` / env-files, not tool argument plaintext ([secrets.md](../secrets.md))
- [ ] You accept that a connected agent acts as your OS user at the daemon API ([security.md](../security.md))

---

## See also

- [integration/README.md](README.md) — all first-class harness guides  
- [mcp.md](../mcp.md) — tools, resources, prompts, agent rules  
- [generic pattern in README](README.md#canonical-stdio-recipe)
