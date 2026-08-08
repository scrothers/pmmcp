# OpenAI Codex (CLI & Desktop)

Wire pmmcp into **OpenAI Codex** — CLI and Desktop share `config.toml` with `[mcp_servers.*]` tables.

**Prerequisites:** [integration README](README.md) — daemon running, `pmmcp doctor` green.

Protocol details: [Agent & MCP integration](../mcp.md).

---

## Add with the CLI

```bash
codex mcp add pmmcp \
  --env PMMCP_PROJECT="$(pwd)" \
  -- "$(command -v pmmcp)" mcp
```

Useful commands:

```bash
codex mcp list
codex mcp show pmmcp
codex mcp remove pmmcp
```

Inside a Codex session, **`/mcp`** shows connected servers.

---

## Config file

| Scope | Path |
|-------|------|
| User | `~/.codex/config.toml` |
| Project (trusted) | `.codex/config.toml` in the repo |

```toml
[mcp_servers.pmmcp]
command = "/usr/local/bin/pmmcp"
args = ["mcp"]

[mcp_servers.pmmcp.env]
PMMCP_PROJECT = "/home/you/code/my-app"
```

Some versions accept inline env:

```toml
[mcp_servers.pmmcp]
command = "/usr/local/bin/pmmcp"
args = ["mcp"]
env = { PMMCP_PROJECT = "/home/you/code/my-app" }
```

If tools never appear, confirm the table name is **`mcp_servers`** (plural with underscore), not `mcp.servers`.

Restart Codex after manual TOML edits.

---

## Desktop GUI

1. Open **Settings** → Codex config / open `config.toml`.
2. Paste the `[mcp_servers.pmmcp]` block above.
3. Restart the Desktop app.
4. Confirm under Plugins / MCP that `pmmcp` is connected.

Marketplace plugins are unrelated — pmmcp is a **local stdio** server you add yourself.

---

## AGENTS.md

Codex honors project agent instructions. Example:

```markdown
## Processes

Use pmmcp MCP tools for supervised processes (pm_start, pm_logs, pm_stop, …).
Pass command as argv. Do not background with shell. On daemon_unavailable,
instruct the user to start pmmcpd.
```

---

## Security notes

- Project-level `.codex/config.toml` can launch local commands — only use in **trusted** repos.
- pmmcp’s adapter runs as your user and talks to your daemon; sandbox still applies to *managed* processes ([security.md](../security.md)).

---

## Troubleshooting (Codex-specific)

| Issue | What to check |
|-------|----------------|
| Config ignored | Key is `mcp_servers.<name>`; restart Codex |
| `/mcp` empty | `codex mcp list`; absolute `command` path |
| Project-only server missing | File is `.codex/config.toml` at project root; project trusted |
| Windows | Use `pmmcp.exe` full path; forward slashes often work in TOML strings |

---

## See also

- [integration/README.md](README.md) · [mcp.md](../mcp.md) · [operations.md](../operations.md)
