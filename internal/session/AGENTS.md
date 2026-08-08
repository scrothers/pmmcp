# agents: session

## role
In-memory harness session registry: 1:1 client connection context for attribution and role binding. Not durable; process supervision and stop-on-disconnect live in the daemon, not here.

## surface
| Symbol / area | Notes |
|---------------|--------|
| `Session` | `ID` (`sess-` ULID), `HarnessID`, `Role`, `CreatedAt`, `EndedAt *time` |
| `Registry` | maps `byID` + `byHarness`; `NewRegistry` |
| `Open(harnessID, role)` | reuses live session for same harness id; empty harness → new anonymous; always server-minted `sess-` |
| `Get` / `GetByHarness` | return **copies** (never the registry pointer) |
| `End(internalID)` | removes from both maps; returns whether found |
| `(*Session).PrimaryID` | harness id if set, else internal id |

## deps
- Project: `internal/id` (`id.New(id.Session)`)
- Third-party: none

## invariants
- Internal map key is always `sess-…`; harness id is a secondary index only
- Repeated `Open` with the same non-empty harness id returns the existing live session (attribution continuity)
- Get/Open return snapshots — callers cannot mutate registry state via the pointer
- Request-carried session ids are labels under the shared-UID model; transport binding is the daemon's job
- Thread-safe (`sync.Mutex`); ID generation happens outside the lock
- No DB, IPC, or OS user lookup; no `func init`

## tests
- `session_test.go` — Open reuse by harness, Get/GetByHarness copies, End removes, PrimaryID, `sess-` prefix
- Unit tests hermetic (`t.Parallel` when safe). **Coverage floor: ≥80% statements** for this package (CI). Do not add production seams only to hit residual lines. Meaningful assertions beat line hits.

## touch map
- `internal/daemon` — ensureSession, session info/end handlers; profile.RemoveSession on end
- Audit/event records may carry `SessionID` for correlation

## do-not
- Do not stop processes or enforce SOD inside this package
- Do not use harness id as the primary map key
- Do not treat Get success as authentication of the caller as session owner
- Do not add durable persistence without an explicit storage design
- Do not auto-create on every Get miss

## related
- `internal/daemon`, `internal/profile` (session selection cleanup), `internal/id`
