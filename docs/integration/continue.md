# Continue

Wire pmmcp into **[Continue](https://continue.dev)** (VS Code / JetBrains extension). MCP is available in **agent** mode.

**Prerequisites:** [integration README](README.md) — daemon running, `pmmcp doctor` green.

Protocol details: [Agent & MCP integration](../mcp.md).

---

## Config options

### A. YAML / JSON in Continue config

In your Continue config (`~/.continue/config.yaml` or workspace config), under `mcpServers`:

```yaml
mcpServers:
  - name: pmmcp
    type: stdio
    command: /usr/local/bin/pmmcp
    args:
      - mcp
    env:
      PMMCP_PROJECT: /home/you/code/my-app
```

### B. Drop-in Claude/Cursor-style JSON

Place a standard MCP JSON file under the workspace:

```text
.continue/mcpServers/pmmcp.json
```

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

Continue auto-picks up files in `.continue/mcpServers/` (plural **Servers**).

---

## Usage

1. Switch Continue to **Agent** mode (MCP tools are not used in pure chat/edit modes the same way).
2. Confirm tools appear in the tool list.
3. Prompt for `pm_list` / start a named process.

---

## Troubleshooting

| Issue | What to check |
|-------|----------------|
| No MCP tools | Agent mode enabled? Config path correct? |
| Directory name | `.continue/mcpServers/` not `mcpServer` |
| Daemon errors | Same as CLI — `pmmcp doctor` |

---

## See also

- [Continue MCP docs](https://docs.continue.dev/customize/deep-dives/mcp)
- [integration/README.md](README.md) · [mcp.md](../mcp.md)
