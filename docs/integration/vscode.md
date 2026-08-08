# VS Code & GitHub Copilot

Wire pmmcp into **Visual Studio Code** agent / Copilot Chat via MCP configuration.

**Prerequisites:** [integration README](README.md) — daemon running, `pmmcp doctor` green.

Protocol details: [Agent & MCP integration](../mcp.md).

---

## Config locations

| Scope | Path / action |
|-------|----------------|
| **Workspace** | `.vscode/mcp.json` (shareable) |
| **User profile** | Command Palette → **MCP: Open User Configuration** |
| **Add wizard** | **MCP: Add Server** |

VS Code’s schema uses a top-level **`servers`** key (not always `mcpServers`).

---

## Workspace example

`.vscode/mcp.json`:

```json
{
  "servers": {
    "pmmcp": {
      "type": "stdio",
      "command": "pmmcp",
      "args": ["mcp"],
      "env": {
        "PMMCP_PROJECT": "${workspaceFolder}"
      }
    }
  }
}
```

If `type` is optional in your build, `command` + `args` alone often still work:

```json
{
  "servers": {
    "pmmcp": {
      "command": "/usr/local/bin/pmmcp",
      "args": ["mcp"],
      "env": {
        "PMMCP_PROJECT": "${workspaceFolder}"
      }
    }
  }
}
```

Prefer **`${workspaceFolder}`** so multi-root and clone paths stay correct.

---

## Copilot CLI / Agent Host portability

Some Copilot surfaces prefer root **`.mcp.json`** or `~/.copilot/mcp-config.json` with a Claude-like shape:

```json
{
  "mcpServers": {
    "pmmcp": {
      "type": "local",
      "command": "pmmcp",
      "args": ["mcp"],
      "env": {
        "PMMCP_PROJECT": "/home/you/code/my-app"
      }
    }
  }
}
```

If you use both VS Code Chat and Copilot CLI, keep definitions in sync or document which file is canonical for your team.

---

## Trust and start

1. Save config → VS Code prompts to **trust** the MCP server.
2. **MCP: List Servers** → start `pmmcp` if it did not auto-start.
3. In Chat, enable Agent mode and allow tools under **Configure Tools**.
4. Ask: “Use pmmcp to show daemon info and list processes.”

Resources: **Add Context → MCP Resources** (e.g. `pmmcp://processes`).  
Prompts: type `/` and look for pmmcp prompt names when exposed.

---

## Sandbox note (VS Code)

VS Code can sandbox stdio MCP servers (`sandboxEnabled`). pmmcp only needs to **dial a Unix socket / named pipe** under your state dir and speak stdio to the editor. If you enable a tight sandbox, allow the IPC path (and do not block the daemon socket). When in doubt, leave sandbox off for the pmmcp server and rely on pmmcp’s **process** sandbox for child isolation.

---

## Troubleshooting (VS Code-specific)

| Issue | What to check |
|-------|----------------|
| Server won’t start | **MCP: List Servers** → Show Output; PATH for `pmmcp` |
| Wrong key | Workspace file uses `"servers"`, not only `"mcpServers"` |
| Remote SSH window | `pmmcp` / `pmmcpd` must exist **on the remote**; socket is remote-user local |
| Tools not in chat | Agent mode; tool picker; server trusted and running |

---

## See also

- [VS Code MCP docs](https://code.visualstudio.com/docs/copilot/customization/mcp-servers)
- [integration/README.md](README.md) · [mcp.md](../mcp.md)
