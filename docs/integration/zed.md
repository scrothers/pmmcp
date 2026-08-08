# Zed

Wire pmmcp into **[Zed](https://zed.dev)** Agent Panel via **context servers** (Zed’s MCP integration).

**Prerequisites:** [integration README](README.md) — daemon running, `pmmcp doctor` green.

Protocol details: [Agent & MCP integration](../mcp.md).

---

## Config locations

| Scope | Path |
|-------|------|
| User | `~/.config/zed/settings.json` (Linux/macOS); Zed → Settings on macOS |
| Project | `.zed/settings.json` |

UI: **Settings → AI → MCP Servers → Add Local Server**.

---

## settings.json

Current Zed shape (flat `command` / `args` / `env`):

```json
{
  "context_servers": {
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

Older examples used a nested command object:

```json
{
  "context_servers": {
    "pmmcp": {
      "command": {
        "path": "/usr/local/bin/pmmcp",
        "args": ["mcp"],
        "env": {
          "PMMCP_PROJECT": "/home/you/code/my-app"
        }
      },
      "settings": {}
    }
  }
}
```

If one shape fails to show a green indicator, try the other for your Zed version — check **Settings → AI → MCP Servers** status dot.

---

## Usage

Open the **Agent Panel**, ensure tools from `pmmcp` are available, then ask for process list / start / logs. Zed reloads context servers when settings change; if not, restart Zed.

---

## Troubleshooting

| Issue | What to check |
|-------|----------------|
| Red / no indicator | Absolute path; JSON validity; try alternate command shape |
| Multi-root workspace | Project `.zed/settings.json` may not apply to every root — prefer user config + `PMMCP_PROJECT` |
| No tools in panel | Agent model/tool permissions; server actually running |

---

## See also

- [Zed MCP docs](https://zed.dev/docs/ai/mcp)
- [integration/README.md](README.md) · [mcp.md](../mcp.md)
