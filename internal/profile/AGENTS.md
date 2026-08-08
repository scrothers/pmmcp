# agents: profile

## role
In-memory CRUD registry for named workspace profiles under a project, plus per-session active profile selection. Not a security boundary (sandbox + OS user are). Durable persistence may wrap later.

## surface
| Symbol / area | Notes |
|---------------|--------|
| `DefaultName` | `"default"` |
| `Profile` | `ID` (`prof-`), `Name`, `ProjectID`, `Env map[string]string` |
| `Store` | `byID`, `byKey(projectID\0name)`, `sessionUse` |
| `NewStore` | empty |
| `Create` / `Get` / `Update` / `Delete` / `List` | List filters by projectID (empty = all) |
| `Use(ctx, session, name)` | session required; empty name → default; profile need not exist |
| `Active(session)` | selected name or `DefaultName` |
| `RemoveSession(session)` | drop selection; daemon calls on session end |

## deps
- Project: `internal/domain` (error codes), `internal/id` (`id.New(id.Profile)`)
- Third-party: none

## invariants
- Name regex: `^[a-z][a-z0-9_-]{0,63}$`; empty create name → `DefaultName`
- Uniqueness on `(ProjectID, Name)`; conflict → `domain.CodeConflict`
- `ProjectID` required on create; immutable on update
- Returned profiles always deep-copy `Env` (no shared maps)
- Session selection is metadata only — does not create profile rows
- IDs are `prof-` ULIDs when auto-assigned; ctx cancel checked on mutating ops
- Thread-safe; no `func init`

## tests
- `profile_test.go` — CRUD, default name, conflict, Use/Active, validation, RemoveSession, cancel
- Unit tests hermetic (`t.Parallel` when safe). **Coverage floor: ≥80% statements** for this package (CI). Do not add production seams only to hit residual lines. Meaningful assertions beat line hits.

## touch map
- Daemon product handlers for profile create/get/update/delete/list/use
- Session-end path should call `RemoveSession` to bound `sessionUse`

## do-not
- Do not treat profiles as authz or sandbox policy
- Do not return shared `Env` map aliases
- Do not silently move a profile across projects on Update
- Do not persist here without an explicit store design
- Do not store raw secret values that belong in `secret: / ` / keyring

## related
- `internal/project`, `internal/session`, `internal/domain`, `internal/id`, 
