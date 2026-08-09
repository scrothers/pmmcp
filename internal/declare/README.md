# declare

Declarative **pmmcp.yaml** parsing, validation, name-set diff, and Procfile import.

## Overview

`Parse` unmarshals YAML into a `Document` (`apiVersion`, optional `kind`, `metadata`, `services`). `Validate` enforces the canonical API version and per-service runtime rules. `DiffServices` compares declared service names to a list of running names. `ImportProcfile` builds a draft document from a classic Procfile.

This package does **not** talk to the daemon or process manager. Apply/validate/diff RPC handlers live in `internal/daemon` and call these helpers.

## When to use

- Validating or diffing a declaration before/without side effects
- Building import paths (Procfile → Document)
- Unit tests that need fixtures without booting pmmcpd

Use daemon methods (`declare.validate` / `diff` / `apply` / `import`) for the product control-plane path.

## Key API

| Symbol | Purpose |
|--------|---------|
| `Parse([]byte)` | YAML → `*Document` (no semantic validation) |
| `(*Document).Validate` | apiVersion, kind, services, depends_on |
| `DiffServices(doc, runningNames)` | `[]DiffEntry` with action create/update/delete/noop |
| `ImportProcfile([]byte)` | Procfile lines → Document + Validate |
| `CanonicalAPIVersion` | `pmmcp.dev/v1alpha1` |
| `ErrInvalid` | Sentinel for malformed/unsupported docs |

`ServiceSpec` fields: `name`, `argv`, `image`, `runtime`, `depends_on`, `sandbox`, `oneshot`.

## Design notes

- Leaf package: only third-party dep is `yaml.v3`.
- Map keys become service names when `name` is omitted.
- Diff is intentionally shallow (presence by name); field-level reconcile can grow later without changing the Document shape.
- Procfile import preserves shell form as `["/bin/sh","-lc",cmd]` so later conversion can remain explicit.

## Testing

Hermetic unit tests in `declare_test.go` (`t.Parallel`, stdlib `testing` only). No network or filesystem beyond test inputs in memory.

## Related packages

- [`internal/daemon`](../daemon) — validate/diff/apply/import handlers
- [`internal/api`](../api) — `declare.*` method constants and payloads
- See also: [Declarative](https://github.com/scrothers/pmmcp/wiki/Declarative)
