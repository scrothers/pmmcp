# Amp

Wire pmmcp into **[Amp](https://ampcode.com)** (CLI / agent) via `amp mcp add` or settings JSON.

**Prerequisites:** [integration README](README.md) — daemon running, `pmmcp doctor` green.

Protocol details: [Agent & MCP integration](../mcp.md).

---

## CLI add

User-global:

```bash
amp mcp add pmmcp -- "$(command -v pmmcp)" mcp
```

Workspace-only (writes `.amp/settings.json`):

```bash
cd /path/to/your/app
amp mcp add --workspace pmmcp -- "$(command -v pmmcp)" mcp
```

Then set project pin in the generated JSON (CLI may not pass env in all versions):

```json
{
  "amp.mcpServers": {
    "pmmcp": {
      "command": "/usr/local/bin/pmmcp",
      "args": ["mcp"],
      "env": {
        "PMMCP_PROJECT": "/path/to/your/app"
      }
    }
  }
}
```

User settings use the same `amp.mcpServers` key under Amp’s global config directory.

---

## Skills vs always-on

Amp recommends bundling MCP in **skills** when tools should load on demand. pmmcp’s large tool surface is a good candidate for skill-scoped loading if you only need process tools during “run the stack” workflows. For day-to-day agent process control, always-on `amp.mcpServers` is fine — disable when idle if context pressure hurts.

Workspace MCP requires **explicit approval** before run (Amp security model).

---

## Verify

```bash
amp
# ask to list pmmcp tools / processes
```

---

## Troubleshooting

| Issue | What to check |
|-------|----------------|
| Not approved | Accept workspace MCP prompt |
| Missing env | Edit settings JSON; add `PMMCP_PROJECT` |
| Daemon down | `pmmcp doctor` outside Amp |

---

## See also

- [Amp manual](https://ampcode.com/manual)
- [integration/README.md](README.md) · [mcp.md](../mcp.md)
