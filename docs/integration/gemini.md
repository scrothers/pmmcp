# Gemini CLI

Wire pmmcp into **Google Gemini CLI** via `settings.json` `mcpServers`.

**Prerequisites:** [integration README](README.md) — daemon running, `pmmcp doctor` green.

Protocol details: [Agent & MCP integration](../mcp.md).

---

## Config locations

| Scope | Path |
|-------|------|
| User | `~/.gemini/settings.json` |
| Project | `.gemini/settings.json` |

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

Some builds accept env placeholders like `"$VAR"`; prefer a concrete absolute `PMMCP_PROJECT` unless you have confirmed expansion.

---

## Verify

```bash
gemini
# then:
/mcp list
```

You should see `pmmcp` and its tools. Exercise with: “List processes via pmmcp.”

---

## Project context

Gemini CLI reads `GEMINI.md` (and related instruction files). Add:

```markdown
Use pmmcp MCP tools for supervised long-lived processes.
Do not use nohup. On daemon_unavailable, tell the user to start pmmcpd.
```

---

## Troubleshooting (Gemini-specific)

| Issue | What to check |
|-------|----------------|
| `/mcp list` empty | JSON syntax; restart CLI; absolute command path |
| Project vs user | Open the project directory that owns `.gemini/settings.json` |
| Env | `PMMCP_PROJECT` set under this server’s `env` block |

---

## See also

- [Gemini CLI MCP servers](https://google-gemini.github.io/gemini-cli/docs/tools/mcp-server.html)
- [integration/README.md](README.md) · [mcp.md](../mcp.md)
