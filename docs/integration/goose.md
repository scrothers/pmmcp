# Goose (Block)

Wire pmmcp into **[Goose](https://block.github.io/goose/)** as a **stdio extension** (Goose’s name for MCP servers).

**Prerequisites:** [integration README](README.md) — daemon running, `pmmcp doctor` green.

Protocol details: [Agent & MCP integration](../mcp.md).

---

## Wizard (recommended)

```bash
goose configure
# → Add Extension
# → Command-line Extension (STDIO)
# → Name: pmmcp
# → Command: /usr/local/bin/pmmcp
# → Args: mcp
# → Enable env: PMMCP_PROJECT=/home/you/code/my-app
```

---

## config.yaml

Typical path: **`~/.config/goose/config.yaml`**

Field names vary slightly by Goose version. One common shape:

```yaml
extensions:
  pmmcp:
    name: pmmcp
    type: stdio
    enabled: true
    cmd: /usr/local/bin/pmmcp
    args: [mcp]
    envs:
      PMMCP_PROJECT: /home/you/code/my-app
```

Alternate shapes seen in the wild:

```yaml
extensions:
  pmmcp:
    type: stdio
    command: /usr/local/bin/pmmcp
    args: ["mcp"]
    enabled: true
    env:
      PMMCP_PROJECT: /home/you/code/my-app
```

If your file uses a list under `extensions:` instead of a map, match the structure of neighboring entries and keep `type: stdio`.

---

## Verify

```bash
goose session
# ask: list processes with pmmcp
```

Or enable the extension in the Goose UI and check that tools load.

---

## Notes

- Goose may impose an extension **timeout**; pmmcp is a local binary and should start quickly.
- Prefer absolute `cmd`/`command` so non-login shells find the binary.

---

## Troubleshooting

| Issue | What to check |
|-------|----------------|
| Extension fails to start | Run `/usr/local/bin/pmmcp mcp` manually; fix PATH |
| Config not applied | `goose configure` vs hand-edited YAML key names (`cmd` vs `command`, `envs` vs `env`) |
| Daemon unavailable | Start `pmmcpd`; set IPC env on the extension if non-default |

---

## See also

- [integration/README.md](README.md) · [mcp.md](../mcp.md)
