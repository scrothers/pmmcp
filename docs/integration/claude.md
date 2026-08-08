# Claude Code & Claude Desktop

Wire pmmcp into **Anthropic Claude Code** (terminal agent) and **Claude Desktop** (app). Both use the same stdio adapter: `pmmcp mcp`.

**Prerequisites:** [integration README](README.md) — daemon running, `pmmcp doctor` green, absolute path to `pmmcp` known.

Protocol details: [Agent & MCP integration](../mcp.md).

---

## Claude Code (CLI)

Claude Code scopes MCP in three places:

| Scope | Where | How |
|-------|--------|-----|
| **User** | `~/.claude.json` | `claude mcp add` (default) or edit JSON |
| **Project** | `.mcp.json` at repo root (commit for the team) | `claude mcp add --scope project …` |
| **Local** | project entry inside `~/.claude.json` | machine-only, not shared |

MCP is **not** stored in `.claude/settings.json` (that file is for other Claude Code settings).

### Recommended: CLI add (user scope)

```bash
# Replace the command path with `which pmmcp` on your machine
claude mcp add pmmcp --scope user \
  --env PMMCP_PROJECT="$(pwd)" \
  -- /usr/local/bin/pmmcp mcp
```

- Everything after `--` is the server command and its args.
- Use an **absolute** path to `pmmcp` when the agent’s environment has a thin `PATH`.

### Project scope (share with the repo)

```bash
cd /path/to/your/app
claude mcp add pmmcp --scope project \
  --env PMMCP_PROJECT="/path/to/your/app" \
  -- /usr/local/bin/pmmcp mcp
```

This writes (or updates) **`.mcp.json`** in the project root:

```json
{
  "mcpServers": {
    "pmmcp": {
      "type": "stdio",
      "command": "/usr/local/bin/pmmcp",
      "args": ["mcp"],
      "env": {
        "PMMCP_PROJECT": "/path/to/your/app"
      }
    }
  }
}
```

Commit `.mcp.json` if the team should share the wiring. Prefer a documented absolute path, or document “set `command` to your local `pmmcp`”. Do not commit secrets; `PMMCP_PROJECT` is not a secret.

### Verify

```bash
claude mcp list
# or inside a session:
/mcp
```

You should see server `pmmcp` and tools such as `pm_list`, `pm_start`, `pm_logs` (names may appear as `pmmcp__pm_*` depending on UI).

### Project rules (optional but useful)

In `CLAUDE.md` or `.claude/CLAUDE.md`:

```markdown
## Process management

Prefer pmmcp MCP tools (`pm_*`) over shell backgrounding for long-lived
processes. Pass argv arrays. Default sandbox is strict. If tools return
`daemon_unavailable`, ask the human to start `pmmcpd` — do not spawn it.
```

---

## Claude Desktop

Desktop reads a single JSON file:

| OS | Path |
|----|------|
| **macOS** | `~/Library/Application Support/Claude/claude_desktop_config.json` |
| **Windows** | `%APPDATA%\Claude\claude_desktop_config.json` |
| **Linux** | `~/.config/Claude/claude_desktop_config.json` (if Desktop is available) |

### Config

```json
{
  "mcpServers": {
    "pmmcp": {
      "command": "/usr/local/bin/pmmcp",
      "args": ["mcp"],
      "env": {
        "PMMCP_PROJECT": "/Users/you/code/my-app"
      }
    }
  }
}
```

Fully **quit and relaunch** Claude Desktop after edits (reload is not always enough).

### Windows notes

Use absolute Windows paths, or wrap via `cmd` when needed:

```json
{
  "mcpServers": {
    "pmmcp": {
      "command": "C:\\Users\\you\\go\\bin\\pmmcp.exe",
      "args": ["mcp"],
      "env": {
        "PMMCP_PROJECT": "C:\\Users\\you\\code\\my-app"
      }
    }
  }
}
```

---

## First agent workflow

1. Human: `pmmcp doctor` is green.
2. In Claude: “List managed processes with pmmcp.” → expect `pm_list`.
3. “Start a process named `web` with command `npm` `run` `dev` under strict sandbox.”
4. “Show the last 50 log lines for `web`.”
5. “Stop `web`.”

If every tool fails with `daemon_unavailable`, the daemon is down or the adapter’s env points at the wrong socket — [Operations](../operations.md#the-daemon-is-not-running).

---

## Troubleshooting (Claude-specific)

| Issue | What to check |
|-------|----------------|
| `claude mcp list` empty | Scope: user vs project. Are you in the project that owns `.mcp.json`? |
| Project server ignored | Trust / approve project MCP when Claude prompts; file must be **repo-root** `.mcp.json`, not `.claude/mcp.json`. |
| Desktop shows no tools | Quit app completely; validate JSON; use absolute `command`. |
| Tools work in CLI but not Desktop | Separate configs — copy the block into Desktop’s file. |

---

## See also

- [integration/README.md](README.md) · [mcp.md](../mcp.md) · [security.md](../security.md)
