# OpenCode

Wire pmmcp into **[OpenCode](https://opencode.ai)** — the open-source terminal coding agent. MCP servers are declared under the `mcp` key in OpenCode’s JSON config.

**Prerequisites:** [integration README](README.md) — daemon running, `pmmcp doctor` green.

Protocol details: [Agent & MCP integration](../mcp.md).

---

## Config locations

| Scope | Typical path |
|-------|----------------|
| User | `~/.config/opencode/opencode.json` (or `opencode.jsonc`) |
| Project | `opencode.json` / `opencode.jsonc` in the project root |

Schema: `https://opencode.ai/config.json`.

---

## Local stdio server

OpenCode uses **`type: "local"`** and a **command array** (command + args together), plus **`environment`** (not `env`).

```json
{
  "$schema": "https://opencode.ai/config.json",
  "mcp": {
    "pmmcp": {
      "type": "local",
      "command": ["/usr/local/bin/pmmcp", "mcp"],
      "enabled": true,
      "environment": {
        "PMMCP_PROJECT": "/home/you/code/my-app"
      }
    }
  }
}
```

Notes:

- `"command": ["pmmcp", "mcp"]` works if `pmmcp` is on the PATH OpenCode inherits.
- Optional `"cwd"` sets the server process working directory; still set `PMMCP_PROJECT` so project scope does not depend on cwd alone.
- `"timeout"` (ms) controls tool discovery; raise only if the host is extremely slow to spawn.

Disable without deleting:

```json
"pmmcp": {
  "type": "local",
  "command": ["/usr/local/bin/pmmcp", "mcp"],
  "enabled": false
}
```

---

## Context cost

OpenCode’s docs warn that MCP tool schemas inflate context. pmmcp ships **~65 tools**. Prefer:

- Enable only while doing process / supervision work.
- Or disable pmmcp tools globally and re-enable on a dedicated agent (OpenCode `tools` + `agent` blocks — see [OpenCode MCP docs](https://opencode.ai/docs/mcp-servers)).

Example pattern (names may need adjustment to match how tools are registered):

```json
{
  "tools": {
    "pmmcp*": false
  },
  "agent": {
    "ops": {
      "tools": {
        "pmmcp*": true
      }
    }
  }
}
```

---

## Verify

1. Restart OpenCode (or reload config per your version).
2. Prompt: “List pmmcp processes” / “use the pmmcp tools”.
3. Confirm `pm_list` (or prefixed equivalent) runs and returns data — not `daemon_unavailable`.

---

## Project instructions

In `AGENTS.md` or OpenCode rules:

```markdown
Long-lived processes go through pmmcp MCP (`pm_*`). Never nohup.
If the daemon is down, tell the user — do not start pmmcpd yourself.
```

---

## Troubleshooting (OpenCode-specific)

| Issue | What to check |
|-------|----------------|
| Server not listed | `type` must be `"local"`; `command` is an **array** |
| Env ignored | Use `environment`, not `env` |
| Context overflow | Disable other heavy MCPs; or gate pmmcp per agent |
| Wrong project | `environment.PMMCP_PROJECT` absolute path |

---

## See also

- [OpenCode MCP servers](https://opencode.ai/docs/mcp-servers)
- [integration/README.md](README.md) · [mcp.md](../mcp.md)
