# domain

Pure **domain leaf** for pmmcp: process records, lifecycle enums, command validation, and machine-readable errors.

## Overview

Everything here is side-effect free. Higher layers (`store`, `process`, `daemon`, `cli`) share these types so status strings, argv rules, and exit codes stay consistent. ID minting and prefix checks are deliberately elsewhere (`internal/id`).

## When to use

- Any package that needs `Process` shape, status/desired values, or `domain.Code` for API/CLI mapping
- Validating argv before spawn (`ValidateCommand`)
- Mapping structured errors to process exit codes (`ExitCode` / `ExitCodeFromError`)

Do **not** put filesystem, network, or container logic here.

## Key API

**Types:** `Process`, `Group`, `Project`, `Profile` — pure structs.

**Validation:**

```go
ValidateCommand([]string) error
func (p *Process) Validate error
```

**Status / desired:** `Status*` constants, `DesiredRunning` / `DesiredStopped`, `.Valid`, `ParseStatus`.

**Errors:** `Code` string constants (`not_found`, `permission_denied`, …), `*Error` with `Code` / `Message` / `Retryable` / wrapped `Err`, constructors `NewError` / `WrapError`, CLI helpers `ExitCode` / `ExitCodeFromError`.

## Design notes

- Import purity is a hard invariant: unit test parses package sources and fails on forbidden stdlib imports.
- `Process.EnvKeys` documents injected env **names** only so logs and listings cannot leak values.
- Predecessor/Successor ID fields support restart generations without domain knowing how restarts run.
- Runtime field is a free string (`local` | `container` at call sites); domain does not open engines.

## Testing

Fast unit suite only. Coverage of status tables, validation edges, exit-code map, and the no-I/O import check.

## Related packages

- [`internal/id`](../id) — ULID generation / prefixes
- [`internal/store`](../store) — persists `domain.Process`
- [`internal/process`](../process) — spawn/inspect using domain status
- [`internal/daemon`](../daemon) — maps handlers to domain codes
- [`internal/cli`](../cli) — exit codes for the user binary
