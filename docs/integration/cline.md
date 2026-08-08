# Cline & Roo Code

Wire pmmcp into **Cline** and **Roo Code** (VS Code extensions with agent chat).

**Prerequisites:** [integration README](README.md) — daemon running, `pmmcp doctor` green.

Protocol details: [Agent & MCP integration](../mcp.md).

---

## Cline

### Config path (VS Code extension)

Global MCP settings (typical):

```text
# macOS
~/Library/Application Support/Code/User/globalStorage/saoudrizwan.claude-dev/settings/cline_mcp_settings.json

# Linux
~/.config/Code/User/globalStorage/saoudrizwan.claude-dev/settings/cline_mcp_settings.json

# Windows
%APPDATA%\Code\User\globalStorage\saoudrizwan.claude-dev\settings\cline_mcp_settings.json
```

Cline CLI (if used) may use `~/.cline/data/settings/cline_mcp_settings.json`.

Open the file from Cline’s MCP UI (**Configure MCP Servers**) when possible so the path matches your install (VS Code vs Cursor vs Insiders).

### JSON shape

```json
{
  "mcpServers": {
    "pmmcp": {
      "command": "/usr/local/bin/pmmcp",
      "args": ["mcp"],
      "env": {
        "PMMCP_PROJECT": "/home/you/code/my-app"
      },
      "disabled": false
    }
  }
}
```

Some Cline versions add `"autoApprove": []` or per-tool auto-approve lists — leave empty until you trust the workflow.

Restart the extension host or reload the window after edits. Green indicator on the `pmmcp` server means stdio connected.

---

## Roo Code

| Scope | Path |
|-------|------|
| Global | `mcp_settings.json` via Roo **Edit Global MCP** |
| Project | `.roo/mcp.json` via **Edit Project MCP** |

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

Project file is good for team-shared wiring (document that each developer must set `command` / `PMMCP_PROJECT` for their machine).

---

## Usage tips

- Cline/Roo already have a strong terminal tool — still prefer **`pm_start` / `pm_run`** for work that should outlive the chat or need sandbox + logs.
- Auto-approving **all** pmmcp tools is powerful (starts/stops processes as you). Prefer approving mutating tools (`pm_start`, `pm_stop`, `pm_apply`, …) until the workflow is familiar.
- Pin `PMMCP_PROJECT` — extension cwd is not always the open workspace folder.

---

## Troubleshooting

| Issue | What to check |
|-------|----------------|
| Red / failed server | Absolute path; run `pmmcp mcp` in a terminal to see immediate errors |
| Wrong globalStorage path | Open config from the extension UI rather than guessing the folder |
| Cursor-hosted Cline | globalStorage lives under Cursor’s app support dir, not Code’s |

---

## See also

- [integration/README.md](README.md) · [mcp.md](../mcp.md) · [security.md](../security.md)
