# Agent harness integrations

Wire **pmmcp** into the coding agent you already use. Every guide here is the same product surface — a local stdio [MCP](https://modelcontextprotocol.io) adapter — with harness-specific config paths and UX.

```bash
pmmcp mcp          # the only command the harness needs to launch
```

That process speaks MCP over stdin/stdout and dials **your** `pmmcpd` over the private socket. It owns no processes. The daemon must already be running.

---

## Before any harness

1. **Install both binaries** and put them on `PATH` — [Installation](../install.md).
2. **Start the daemon** once (service or foreground):

   ```bash
   pmmcpd run
   # or: pmmcp install-service   # then enable with your OS tool
   ```

3. **Confirm:**

   ```bash
   pmmcp doctor      # exit 0
   which pmmcp       # absolute path for configs that need it
   ```

4. **Prefer pinning the project** in the MCP `env` so the agent's cwd does not steal scope:

   ```text
   PMMCP_PROJECT=/absolute/path/to/your/repo
   ```

Shared protocol, tools, resources, prompts, and security model: **[Agent & MCP integration](../mcp.md)**.

---

## Guides by harness

| Harness | Guide | Config surface |
|---------|-------|----------------|
| **Claude Code** / **Claude Desktop** | [claude.md](claude.md) | `.mcp.json`, `claude mcp add`, Desktop JSON |
| **Grok Build** (xAI) | [grok.md](grok.md) | `~/.grok/config.toml`, `grok mcp add` |
| **OpenCode** | [opencode.md](opencode.md) | `opencode.json` → `mcp` |
| **OpenAI Codex** (CLI / Desktop) | [codex.md](codex.md) | `~/.codex/config.toml`, `codex mcp add` |
| **Cursor** | [cursor.md](cursor.md) | `.cursor/mcp.json`, `~/.cursor/mcp.json` |
| **Windsurf** (Cascade) | [windsurf.md](windsurf.md) | `~/.codeium/windsurf/mcp_config.json` |
| **VS Code** + GitHub Copilot | [vscode.md](vscode.md) | `.vscode/mcp.json`, user MCP config |
| **Gemini CLI** | [gemini.md](gemini.md) | `~/.gemini/settings.json` |
| **Cline** / **Roo Code** | [cline.md](cline.md) | `cline_mcp_settings.json`, `.roo/mcp.json` |
| **Continue** | [continue.md](continue.md) | `.continue/mcpServers/` |
| **Goose** (Block) | [goose.md](goose.md) | `~/.config/goose/config.yaml` |
| **Zed** | [zed.md](zed.md) | `context_servers` in settings |
| **Amp** | [amp.md](amp.md) | `amp mcp add`, `.amp/settings.json` |
| **Any other stdio host** | [generic.md](generic.md) | command + args + env |

---

## Canonical stdio recipe

Every harness eventually needs some form of:

| Field | Value |
|-------|--------|
| **Command** | Absolute path to `pmmcp` (or `pmmcp` if on `PATH`) |
| **Args** | `["mcp"]` only |
| **Env (recommended)** | `PMMCP_PROJECT` = absolute project root |
| **Env (if non-default paths)** | `PMMCP_IPC_ENDPOINT`, `PMMCP_STATE_DIR` — [Configuration](../configuration.md) |

Do **not** point the harness at `pmmcpd`. The adapter is the client; the daemon is separate and long-lived.

### Example JSON (Claude / Cursor / Desktop style)

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

### Example TOML (Grok / Codex style)

```toml
[mcp_servers.pmmcp]
command = "/usr/local/bin/pmmcp"
args = ["mcp"]
env = { PMMCP_PROJECT = "/home/you/code/my-app" }
```

---

## What the agent gets

- **~65 tools** named `pm_*` (harnesses often prefix: `pmmcp__pm_start`, `pmmcp_pm_start`, …).
- **Resources** under `pmmcp://…` and **prompts** such as `pmmcp_start_safe` — see [mcp.md](../mcp.md).
- The same semantics as the CLI: argv never shell, strict sandbox by default, full audit trail.

### Teach the agent (worth pasting into project rules)

```text
Use pmmcp MCP tools (pm_*) for long-lived processes — not nohup, not bare background shells.
Pass command as an argv array. Prefer strict sandbox. If tools return daemon_unavailable,
tell the user to start pmmcpd; do not try to spawn the daemon yourself.
```

---

## Trust and safety (all harnesses)

Connecting pmmcp means the agent can *ask* your user daemon to start/stop processes **as your OS user**. Containment is the **process sandbox**, not the MCP hop. Read [Security](../security.md) and [mcp.md → What an agent can and cannot do](../mcp.md#what-an-agent-can-and-cannot-do).

Practical habits:

- Keep default **strict** sandbox for agent-started work.
- Pin `PMMCP_PROJECT` so tools never land in a random cwd.
- Use `pm_audit_query` / `pmmcp audit` after sessions you care about.
- Sixty-five tools add context cost — disable the server when you are not doing process work.

---

## Shared troubleshooting

| Symptom | Fix |
|---------|-----|
| Tools missing | Daemon up? `pmmcp doctor`. Absolute `command` path? Restart the harness after config edits. |
| `daemon_unavailable` | Start `pmmcpd`; align `PMMCP_IPC_ENDPOINT` / `PMMCP_STATE_DIR` with the daemon. |
| Wrong project | Set `PMMCP_PROJECT` in the MCP server env. |
| `sandbox_failed` | Install platform sandbox deps (e.g. Linux `bubblewrap`) or choose a supported profile. |
| `ipc_version_mismatch` | Rebuild/reinstall **matching** `pmmcp` and `pmmcpd`. |

Harness-specific notes live in each guide.

---

## See also

- [Agent & MCP integration](../mcp.md) — protocol, tool catalog, resources, prompts
- [Installation](../install.md) · [Configuration](../configuration.md) · [Security](../security.md)
- [CLI reference](../cli.md) — human equivalents of every `pm_*` tool
