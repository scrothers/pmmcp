# Declarative `pmmcp.yaml`

Everything you can do imperatively — start a process, define a group, set a health check or a watch — you can also declare in a file and apply. A `pmmcp.yaml` at your project root turns a stack into something you can review, diff, and reconcile, instead of a sequence of `start` commands nobody remembers.

The workflow is **validate → diff → apply**, and it's safe by design: the validator refuses dangerous declarations outright, and `apply` shows you exactly what will change before it changes anything.

---

## A worked example

```yaml
apiVersion: pmmcp.dev/v1alpha1
kind: Project

metadata:
  name: my-app

# Top-level services: the simple form — a map of name → spec.
services:
  web:
    argv: ["npm", "run", "dev"]
    cwd: .
    sandbox: standard          # needs the network; still can't read your secrets
    ports:
      - name: http
        port: 3000
    watch:
      enabled: true
      paths: ["./src"]
      action: restart          # restart | signal

# spec: the richer form — defaults, groups with ordering, one-shot jobs, webhooks.
spec:
  profile: dev
  defaults:
    sandbox: strict            # the default profile for services that don't name one

  groups:
    - name: stack
      description: API and its database
      members:
        - name: db
          image: postgres:16
          runtime: container
        - name: api
          argv: ["npm", "start"]
          depends_on:
            db: service_started   # api starts after db

  jobs:
    - name: migrate
      argv: ["./scripts/migrate.sh"]
      oneshot: true

  webhooks:
    - url: https://hooks.example.com/pmmcp
      events: ["process.crashed"]
```

The two shapes coexist: use the flat `services:` map for a handful of standalone processes, and the `spec:` block when you need groups, jobs, or webhooks. `apiVersion` and `kind` are required and must be exactly `pmmcp.dev/v1alpha1` and `Project`.

---

## The schema

### Document

| Field | Type | Notes |
|-------|------|-------|
| `apiVersion` | string | required, `pmmcp.dev/v1alpha1` |
| `kind` | string | required, `Project` |
| `metadata` | map | free-form metadata |
| `services` | map[name → service] | standalone services (flat form) |
| `spec` | object | the richer stack definition (below) |

### `spec`

| Field | Type | Notes |
|-------|------|-------|
| `profile` | string | profile these definitions belong to |
| `defaults.sandbox` | string | default sandbox profile for services that omit one |
| `groups` | list | groups with members and ordering |
| `jobs` | list of service specs | one-shot tasks |
| `webhooks` | list | outbound event webhooks (`url`, `events`) |

### Service spec (used by `services`, group `members`, and `jobs`)

| Field | Type | Notes |
|-------|------|-------|
| `name` | string | unique within the project/profile |
| `argv` | list of strings | the program and its arguments — **not** a shell string |
| `image` | string | container image (with `runtime: container`) |
| `runtime` | string | `local` (default) or `container` |
| `cwd` | string | working directory |
| `sandbox` | string | `strict`·`standard`·`permissive` (`off` is rejected — see below) |
| `depends_on` | map | dependency edges: `name: condition` (drives start/stop order) |
| `oneshot` | bool | run to completion, never auto-restart |
| `ports` | list | `name`, `port`, `host_port`/`container_port`, `protocol`, `bind` |
| `watch` | object | `enabled`, `paths`, `ignore`, `action` (`restart`/`signal`) |

> **Environment and secrets** are applied through profiles and env-files rather than an inline service `env:` field. Reference secrets by `secret://…` in an env-file and attach it at the profile or process level — see [Secrets & environment](secrets.md).

---

## The workflow

### Validate

Parse and policy-check the file. Makes no changes:

```bash
pmmcp validate
```

Validation is **strict** — an unknown field is an error, not a silent no-op, so a typo'd key fails loudly. All problems are reported together, so you fix them in one pass rather than one at a time.

### Diff

Show what `apply` *would* do — which services it would create, update, or leave alone — without touching anything:

```bash
pmmcp diff
```

### Apply

Reconcile the project to the file:

```bash
pmmcp apply
```

`apply` creates the services in the diff and reports what it did (`{ created, diff }`). Run it again after editing the file to converge to the new state. Because it's a reconcile, `apply` is the natural way to evolve a stack: change the YAML, `diff` to preview, `apply` to land it.

### Show

Render the effective declaration pmmcp has for the project:

```bash
pmmcp declare show
```

---

## What gets rejected

The validator is the reason a checked-in `pmmcp.yaml` is safe to trust. It refuses a declaration — with **all** the reasons at once — for any of these:

| Rejection | Why |
|-----------|-----|
| `sandbox_required` | A service (or `defaults`) set `sandbox: off`. A file that lives in your repo must never silently disable isolation. Relax a *specific* process imperatively if you must; the declarative path won't. |
| `argv_shell_risk` | An `argv` invokes a shell — `["bash","-c", …]`, `sh -c`, `powershell -Command`, `cmd /c`. Declarative services are real argv, never shell strings. |
| `path_outside_project` | A watch path escapes the project root. |
| `port_privileged` | A host-facing port below 1024. |
| `webhook_url_denied` | A webhook URL targets a blocked destination (loopback, private/link-local, cloud metadata, non-HTTP scheme). |

Plus the structural checks: a wrong `apiVersion` or `kind`, a service with no `argv`, a `depends_on` pointing at a name that isn't defined, and any unknown field. Codes and details are in the [error reference](reference-errors.md#validation-errors-pmmcpyaml).

This strictness is deliberate and asymmetric: the *imperative* `pm_start` trusts an explicit shell you type, because you typed it; the *declarative* path — which an agent might author and commit — does not. A hostile or careless `pmmcp.yaml` is rejected wholesale, with zero side effects, before anything runs.

---

## Importing existing definitions

Coming from `foreman` or `honcho`? Import an existing Procfile into a `pmmcp.yaml` starting point:

```bash
pmmcp import path=Procfile
```

Because a `Procfile` line *is* a shell command, the importer wraps each entry in an explicit shell invocation and marks it with a `TODO` — so you can see exactly where a shell crept in and replace it with a real `argv` before you `apply`. The import is a draft for you to review, never something applied blind. (This is the one place pmmcp writes a shell wrapper for you, and it makes it loud on purpose — see [Concepts → Argv, not shell](concepts.md#argv-not-shell).)

---

## See also

- [Supervision & orchestration](supervision.md) — the runtime behavior your `pmmcp.yaml` configures
- [Security](security.md) — why `sandbox: off` and shell-argv are refused in a file
- [CLI reference](cli.md) — `validate` / `diff` / `apply` / `import`
- [Agent & MCP integration](mcp.md) — the `pmmcp_apply_stack` prompt
