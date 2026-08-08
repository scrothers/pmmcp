# agents: domain

## role
Pure process-manager value types, status/desired enums, argv validation, and structured error codes with CLI exit mapping. Leaf of the import graph: no I/O, no project imports.

## surface
| Symbol / area | Notes |
|---------------|--------|
| `Process`, `Group`, `Project`, `Profile` | Pure value records; IDs are plain strings |
| `ValidateCommand`, `(*Process).Validate` | Non-empty argv; no empty args; status/desired shape |
| `Status`, `Desired`, `AllStatuses`, `ParseStatus` | Lifecycle + reconcilable desired state |
| `Code`, `Error`, `NewError`, `WrapError`, `WithDetails` | Stable machine codes for CLI/MCP/gRPC |
| `ExitCode`, `ExitCodeFromError` | CLI exit map |
| `ErrInvalidCommand`, `ErrInvalidProcess` | Validation sentinels |

## deps
- Project: none (leaf)
- Third-party: none (stdlib only: `errors`, `fmt`, `time`)

## invariants
- No OS, network, filesystem, or SQL imports in non-test sources.
- Command is an argv list only — never an implicit shell string.
- `Process.EnvKeys` holds key **names** only, never values.
- Status set: starting|running|unhealthy|stopping|exited|failed|crashed; desired: running|stopped.
- IDs are opaque strings here; generation and prefix rules live in `internal/id`.
- Single-threaded by contract (no goroutines). Unset `Code` on a non-nil `*Error` exits 1, not 0.

## tests
- `domain_test.go`, `errors_test.go` — ValidateCommand/Process, status/desired, exit map, import purity.
- Unit tests hermetic (`t.Parallel` when safe). **Coverage floor: ≥80% statements** for this package (CI). Do not add production seams only to hit residual lines. Meaningful assertions beat line hits.

## touch map
- New status/desired value → constants + `Valid` + `AllStatuses` (if status) + docs.
- New error code → `Code*` + `ExitCode` switch when CLI-visible.
- Domain field on process record → keep pure; no store/ipc types.

## do-not
- Import engine, store, ipc, secret, or OS packages.
- Generate ULIDs or call `crypto/rand` here.
- Log or print; return `*Error` / sentinels only.
- Put secret or env **values** on `Process` (names only via `EnvKeys`).
- Use UUIDs for identity vocabulary.

## related
Consumers: `daemon`, `process/*`, `group`, `cli`, `api` (version compatibility errors).
