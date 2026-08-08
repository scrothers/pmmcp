# Grok Build (xAI)

Wire pmmcp into **Grok Build** — the terminal TUI / headless agent (`grok`). MCP servers live in TOML and are managed with `grok mcp …` or `/mcps` in the TUI.

**Prerequisites:** [integration README](README.md) — daemon running, `pmmcp doctor` green.

Protocol details: [Agent & MCP integration](../mcp.md).

---

## Add with the CLI (recommended)

```bash
# After -- is the full server command (args are not eaten by grok)
grok mcp add pmmcp \
  -e PMMCP_PROJECT="$(pwd)" \
  -- "$(command -v pmmcp)" mcp
```

- `-e` is repeatable for more env vars (`PMMCP_IPC_ENDPOINT`, …).
- Prefer `$(command -v pmmcp)` expanded to an absolute path in committed configs.

List / diagnose:

```bash
grok mcp list
grok mcp doctor pmmcp
grok inspect          # shows MCP servers discovered for this directory
```

---

## Config file

User config: **`~/.grok/config.toml`**

Optional repo overlay: **`.grok/config.toml`** (trusted folder; deepest wins per Grok’s config chain).

```toml
[mcp_servers.pmmcp]
command = "/usr/local/bin/pmmcp"
args = ["mcp"]
env = { PMMCP_PROJECT = "/home/you/code/my-app" }
enabled = true
# Optional: pmmcp starts fast; leave defaults unless you need them
# startup_timeout_sec = 30
# tool_timeout_sec = 600
```

Reload: Grok can enable/disable without a full restart (`/mcps` or `grok mcp enable|disable pmmcp`). Prefer restarting a long session after first add so tool schemas are fresh.

---

## TUI

- Slash command **`/mcps`** (or the MCP Servers tab in the plugins UI) — see source, enabled state, tool count.
- Space toggles enable/disable.
- Ask in natural language: “use pmmcp to list processes” / call `pm_list`.

Tool names are typically namespaced with the server id (e.g. `pmmcp` + `pm_start`). Use `grok mcp doctor` if discovery fails.

---

## Project rules

Grok reads `AGENTS.md` / project instructions. Add:

```markdown
## Long-lived processes

Use the pmmcp MCP server (`pm_*` tools), not background shells.
Argv arrays only. Strict sandbox by default. Never start `pmmcpd` yourself —
surface `daemon_unavailable` to the user.
```

---

## Headless / scripts

Headless `grok -p "…"` uses the same MCP config as the TUI for that cwd. Ensure `PMMCP_PROJECT` is set in the server env so non-interactive runs do not depend on shell cwd heuristics alone.

---

## Troubleshooting (Grok-specific)

| Issue | What to check |
|-------|----------------|
| Server missing after edit | `grok mcp list`; valid TOML; `enabled = true` |
| Startup timeout | Unlikely for a local Go binary; confirm `command` is executable |
| Large tool lists | pmmcp exposes many tools — disable when unused (`grok mcp disable pmmcp`) to save context |
| Wrong project | Set `PMMCP_PROJECT` via `-e` or `env = { … }` |

---

## See also

- Grok user guide: MCP servers (`~/.grok/docs/user-guide/07-mcp-servers.md` when installed)
- [integration/README.md](README.md) · [mcp.md](../mcp.md)
