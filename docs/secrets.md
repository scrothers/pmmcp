# Secrets & environment

Processes need configuration, and some of that configuration is sensitive — a database URL, an API token, a signing key. pmmcp's rule is simple: **secrets move by reference, never by value, and never appear where they could leak.** This guide shows how to give a process its environment and its secrets without either landing in your shell history, a log file, a status dump, or a tool argument an agent logged.

---

## Two kinds of environment

1. **Plain configuration** — non-sensitive values (`NODE_ENV=production`, `PORT=3000`). These can be set inline or in an env-file.
2. **Secrets** — values that must not be seen. These are stored once, then referenced by name. The process gets the resolved value in its environment at launch; nothing else ever does.

---

## Env-files

An env-file is a `dotenv`-style `KEY=value` file, referenced **by path**, not inlined into a spec. A process can pull its configuration from one:

```
# .env
NODE_ENV=production
PORT=3000
DATABASE_URL=secret://keyring/db-url      # a reference, resolved at launch — see below
```

Env-files are read by the daemon and materialized into the child's environment. **Keep them `0600`** — if an env-file is group- or world-readable, pmmcp warns you (it doesn't refuse to load it), so the tightening is on you. The keyring is different: pmmcp *enforces* `0600` on stored secret files and `0700` on the keyring directory, tightening them on every write.

### How the environment is composed

A process starts from a **minimal baseline** — *not* your full shell environment. On top of that, the daemon applies, with the **last value winning**:

1. the env-files the process references (loaded in the order listed), then
2. an inline `env` map,

and a **profile** contributes an env overlay shared by all its processes. Secret `secret://` refs in any of these are resolved into the child at launch.

> The local driver never inherits your full shell environment by default. That's deliberate: a process shouldn't silently receive every variable in your terminal (including ones that happen to be sensitive). Declare what a process needs.

> **Attaching env on the CLI.** The plain `pmmcp start` command takes only `--name`, `--cwd`, `--sandbox`, and `--project` — there is no `--env` flag. Give a process its environment through a **profile** (which carries an `env` overlay) or, for an agent, through a `pm_start` call, which the daemon accepts with `env` and `env_files` fields.

---

## Secret references

A secret is named and stored once, then referred to with a `secret://` URI wherever a value would go. pmmcp resolves the reference into the child's environment at launch and nowhere else. Four schemes:

| Scheme | Form | Resolves from |
|--------|------|---------------|
| `keyring` | `secret://keyring/<name>` or `secret://keyring/pmmcp/<name>` | pmmcp's file-backed keyring (`<state>/keyring`, `0600`) |
| `file` | `secret://file:<path>` (or `secret://file/<path>`) | the contents of a file |
| `sops` | `secret://sops:<path>#<key>` (or `secret://sops/<path>#<key>`) | a [SOPS](https://github.com/getsops/sops)-encrypted file, decrypted at resolve time |
| `env` | `secret://env:<VAR>` (or `secret://env/<VAR>`) | an environment variable of the daemon |

A bare name with no scheme is rejected — a reference must say where it comes from. Traversal is refused, so `secret://keyring/../../etc/passwd` fails at parse.

### Storing a keyring secret

The value is read from **stdin**, never the command line — so it never appears in your shell history or the process table:

```bash
printf %s "$GITHUB_TOKEN" | pmmcp secret set github-token
```

Then reference it wherever the value would go — for example in an env-file the process loads (keep it `0600`):

```
# .env  — referenced by the process; resolved at launch, never stored resolved
GITHUB_TOKEN=secret://keyring/github-token
```

The daemon reads the file, resolves the reference, and places the value in the child's environment only — never in a log, a status dump, or a tool result. Attach the env-file (or the ref) through a [profile](concepts.md#profile--a-named-variant-within-a-project) or an agent's `pm_start` call (`env` / `env_files`).

### Managing references

```bash
pmmcp secret list           # names only — values are never listed
pmmcp secret check ref=secret://keyring/github-token   # verify it resolves
```

`secret list` returns *names*, never values — there is no command that prints a stored secret back to you.

---

## Redaction

Even with references, a secret value can end up in output the moment a process prints it. pmmcp redacts known secret values everywhere they might surface — **captured logs, events, status dumps, log exports, and webhook bodies** — replacing them with a stable placeholder:

```
***REDACTED:DATABASE_URL***    # a named secret
***REDACTED***                 # an unnamed value
```

The named form (`***REDACTED:NAME***`) is stable and greppable, so downstream tooling can key on it. Redaction is applied on the write path — a secret that a process echoes to stdout is scrubbed before it's stored, not after — and on top of value redaction, common secret *shapes* (AWS keys, JWTs, PEM blocks, bearer tokens) are caught by pattern even when pmmcp doesn't know the specific value.

> Redaction is defense-in-depth, not a license to be careless. The primary protection is not printing secrets in the first place; redaction catches the cases where a dependency logs a connection string you didn't expect.

---

## Reading raw values

Reading a stored secret's *value* back out requires the `secrets:read_values` capability, which only the `full` role holds (see [Security → Capabilities](security.md#capabilities-and-roles)). The `agent` role — and thus the default posture for an agent — cannot exfiltrate stored secret values through pmmcp.

---

## Guidance for agents

If you're integrating an agent:

- **Never put a secret value in a tool argument.** Store it once with `pm_secret_set` (its designated write path) and reference it by `secret://…` everywhere else.
- **Give processes secrets by reference or env-file**, not inline.
- Assume anything a process prints *might* contain a secret; rely on redaction but design the process not to print them.

---

## See also

- [Security & sandboxing](security.md) — why the sandbox also denies `~/.aws`, `~/.ssh`, and the engine socket
- [Configuration](configuration.md) — the keyring location and file-permission model
- [Declarative `pmmcp.yaml`](declarative.md) — how declared services relate to profiles and env
