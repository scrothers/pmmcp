# id

Prefixed Crockford ULID identifiers for all durable pmmcp resources.

## Overview

Public IDs look like:

```text
proc-01ARZ3NDEKTSV4RRFFQ69G5FAV
grp-01…
prof-01…
evt-01…
aud-01…
sess-01…
proj-01…
```

Each ID is `<type-prefix>-` plus a 26-character lowercase Crockford base32 ULID. Prefixes make entity type obvious in logs and MCP payloads; ULIDs are time-sortable and opaque.

This package is the **only** place that generates and validates that format.

## When to use

- Any create path for processes, groups, profiles, events, audit records, sessions, or projects.
- Validating client-supplied IDs before lookup.
- Checking that a string is a specific resource kind (`HasPrefix`).

Domain types may store ID **strings**; they should not reimplement parse rules. Generation stays out of pure domain packages where possible so domain remains free of crypto/time side effects.

## Key API

```go
pid, err:= id.New(id.Proc) // "proc-" + ULID
gid, err:= id.New(id.Group) // "grp-" + ULID

prefix, u, err:= id.Parse(pid)
ok:= id.Valid(pid)
ok = id.HasPrefix(pid, id.Proc)
```

### Prefix constants

| Constant | Prefix string | Entity |
|----------|---------------|--------|
| `id.Proc` | `proc` | Supervised process |
| `id.Group` | `grp` | App group |
| `id.Profile` | `prof` | Named profile |
| `id.Event` | `evt` | Domain event |
| `id.Audit` | `aud` | Audit record |
| `id.Session` | `sess` | Session |
| `id.Project` | `proj` | Project |

### Errors

`ErrInvalid` wraps unknown prefixes, empty input, wrong body length, non-Crockford characters, and malformed shapes (including UUID-looking bodies). Use `errors.Is(err, id.ErrInvalid)`.

## Design notes

- **House standard.** Prefixed lowercase Crockford ULIDs; never UUIDs for new resource primary keys. UUIDv7 only if an external system forces it (document the exception).
- **Entropy.** `ulid.Monotonic(crypto/rand.Reader, 0)` with UTC timestamps — secure randomness, monotonic within a millisecond for the process.
- **Case.** Emitted bodies are lowercase; `Parse` uppercases for `ulid.ParseStrict` then returns the prefix as a known `Prefix` value.
- **Opacity.** Clients must treat IDs as opaque. Names, paths, and ports live in separate fields.
- **Extending prefixes.** Add a `Prefix` const and an entry in the unexported `known` map in the same change; update tests and docs if the public vocabulary grows.
- **Leaf package.** No imports of other internal packages — safe for domain-adjacent callers and avoids import cycles.

## Testing

```bash
go test./internal/id/
```

Round-trips every known prefix, rejects unknown prefixes, and rejects a table of malformed strings (empty, no hyphen, bad charset, wrong length, UUID shape, etc.).

## Related

- [`../domain/`](../domain/) — stores ID strings; defers generation/validation here
- [`../group/`](../group/), [`../event/`](../event/), [`../audit/`](../audit/), [`../session/`](../session/), [`../profile/`](../profile/) — create-time `id.New`
- [`../daemon/`](../daemon/) — process and other resource creation
- Identity ULIDs (prefixed Crockford base32)
