# Cursor

Wire pmmcp into **[Cursor](https://cursor.com)** via `mcp.json` (project or global).

**Prerequisites:** [integration README](README.md) — daemon running, `pmmcp doctor` green.

Protocol details: [Agent & MCP integration](../mcp.md).

---

## Config locations

| Scope | Path |
|-------|------|
| **Project** | `.cursor/mcp.json` (repo root) |
| **Global** | `~/.cursor/mcp.json` |

Project wins when both define the same server name. UI path: **Cursor Settings → Tools & MCP → New MCP Server**.

---

## stdio config

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

Interpolation (when supported by your Cursor version):

```json
{
  "mcpServers": {
    "pmmcp": {
      "command": "pmmcp",
      "args": ["mcp"],
      "env": {
        "PMMCP_PROJECT": "${workspaceFolder}"
      }
    }
  }
}
```

`${workspaceFolder}` is ideal so each repo pins itself without hardcoding paths — use it when your Cursor build documents that variable for `env`.

---

## Using tools

1. Open **Customize** / MCP list — ensure `pmmcp` is enabled (green).
2. In Agent chat, approve tool calls when prompted (or use a Run Mode that auto-allows trusted tools).
3. Prompt: “Using pmmcp, list processes and start `web` with `npm run dev`.”

Cursor may show tools as `pmmcp` / `pm_*`. Resources and prompts from the adapter are available when the client surfaces them.

---

## Project rules

`.cursor/rules` or project docs:

```text
Prefer pmmcp MCP for long-running processes. Argv only, strict sandbox.
Never start pmmcpd; report daemon_unavailable to the user.
```

---

## Troubleshooting (Cursor-specific)

| Issue | What to check |
|-------|----------------|
| Server never starts | **Output → MCP Logs**; absolute `command`; JSON validity |
| Project `mcp.json` ignored | File under `.cursor/` at workspace root; restart Cursor |
| Tools disabled after restart | Re-enable in Customize; some builds do not persist per-tool toggles in JSON |
| Cloud / remote agents | Local stdio `pmmcp` only works where the agent process can reach your machine’s socket — not on a remote cloud VM without the daemon |

---

## See also

- [Cursor MCP docs](https://cursor.com/docs/mcp)
- [integration/README.md](README.md) · [mcp.md](../mcp.md)
