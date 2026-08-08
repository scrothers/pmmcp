# Windsurf (Cascade)

Wire pmmcp into **Windsurf** / **Cascade** via the global MCP config JSON.

**Prerequisites:** [integration README](README.md) — daemon running, `pmmcp doctor` green.

Protocol details: [Agent & MCP integration](../mcp.md).

---

## Config file

```text
~/.codeium/windsurf/mcp_config.json
```

Open from Cascade: MCP / tools icon → configure (opens this file).

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

Restart Cascade or reload MCP after saving. Confirm the server shows as connected in the MCP list.

---

## Usage

In Cascade chat, ask for process management explicitly until the model picks tools reliably:

> Use the pmmcp MCP tools to list processes, then start name `api` with command argv `["go","run","./cmd/api"]`.

---

## Notes

- Windsurf’s MCP config is **user-global** by default — set `PMMCP_PROJECT` per project if you switch repos often, or maintain separate env when Cascade supports per-workspace overrides in your version.
- Same trust model as other hosts: adapter = your user → your daemon ([security.md](../security.md)).

---

## Troubleshooting (Windsurf-specific)

| Issue | What to check |
|-------|----------------|
| Cascade fails to load MCPs | Validate JSON; remove other broken server entries temporarily |
| Path errors | Absolute path to `pmmcp`; on macOS Apple Silicon, often `/opt/homebrew/bin/pmmcp` |
| Daemon errors | `pmmcp doctor` outside Windsurf first |

---

## See also

- [integration/README.md](README.md) · [mcp.md](../mcp.md)
