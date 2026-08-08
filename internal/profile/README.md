# profile

In-memory registry for **named profiles** under a project. Profiles hold reusable defaults (today: environment key/value overlays) and track which profile name each session has selected.

## Why profiles exist

Agents and humans run the same repo in different shapes (`dev`, `test`, `agent-debug`). A profile is a named config slice **inside** a project — not a second project, and not a session or tenancy boundary.

Security isolation still comes from sandbox profiles and OS user identity. Profiles are convenience and scoping metadata.

## Types

### `Profile`

| Field | Description |
|-------|-------------|
| `ID` | Prefixed ULID (`prof-…`), assigned on create if empty |
| `Name` | Unique within a project; default name is `default` |
| `ProjectID` | Owning project identity string (required on create) |
| `Env` | String map of environment overlays |

### `Store`

Thread-safe CRUD plus session selection:

| Method | Role |
|--------|------|
| `Create` | Insert; conflict if `(projectID, name)` exists |
| `Get` | By profile ID |
| `Update` | Rename and/or replace `Env` |
| `Delete` | By profile ID |
| `List` | By project ID, or all if project ID empty |
| `Use` | Bind a session id → profile **name** (name need not exist yet) |
| `Active` | Name selected for session, or `default` |

### Name rules

```text
^[a-z][a-z0-9_-]{0,63}$
```

Invalid names return `domain.CodeInvalidArgument`.

## Usage

```go
st:= profile.NewStore

p, err:= st.Create(ctx, profile.Profile{
 Name: "dev",
 ProjectID: "proj-…", // or project key string used by daemon
 Env: map[string]string{"LOG_LEVEL": "debug"},
})

_ = st.Use(ctx, sessionID, "dev")
name:= st.Active(sessionID) // "dev"
```

## How the daemon uses it

`internal/daemon.Server` holds `profiles *profile.Store`, created in product state setup. MCP/CLI methods map to store operations (list/create/update/delete/use) with audit events on mutations. Process start may later merge profile env into the child environment; selection is already available via `Use` / `Active`.

## Persistence

This package is **in-memory only**. Process records live in SQLite (`internal/store`); profiles may gain a durable adapter later without changing the conceptual API. Do not assume profiles survive daemon restart until persistence lands.

## Relation to project

| Concept | Package | Role |
|---------|---------|------|
| Project root / key | [`internal/project`](../project/) | Detect cwd → root + stable key |
| Profile | **this package** | Named variant **under** a project |
| Session | `internal/session` | Who is talking to the daemon |

## Errors

Uses structured `domain.Error` where appropriate:

- `invalid_argument` — missing project/session/id, bad name
- `conflict` — duplicate name in project
- `not_found` — unknown id

Context cancellation is wrapped as `profile: <op>: …`.

## Testing

```bash
go test./internal/profile/
```

## Related docs

- project and profile scope
- docs
- Phases: `30-profiles-list-use.md`, `42-profile-crud.md`
