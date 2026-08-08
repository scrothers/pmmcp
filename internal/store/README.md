# store

Durable **repository interfaces** for pmmcp daemon state. This package holds contracts and sentinel errors only — no database driver, no SQL, no file I/O.

## Why it exists

Per, process inventory and desired state live in a single-user SQLite database opened **only by `pmmcpd`**. Clients (`pmmcp`, MCP) never touch the file; they call the daemon over private IPC. Keeping the interface here lets the daemon depend on `store.ProcessStore` while the concrete backend stays swappable.

## API

```go
type ProcessStore interface {
 Migrate(ctx context.Context) error
 Create(ctx context.Context, p *domain.Process) error
 Get(ctx context.Context, id string) (*domain.Process, error)
 Update(ctx context.Context, p *domain.Process) error
 Delete(ctx context.Context, id string) error
 List(ctx context.Context, f ProcessFilter) ([]*domain.Process, error)
 Close error
}
```

| Sentinel | Meaning |
|----------|---------|
| `ErrNotFound` | No row for the given process ID |
| `ErrConflict` | Create hit a uniqueness conflict (e.g. duplicate ID) |

`ProcessFilter` narrows `List`: `ProjectID`, `Status`, and `Name`. Empty fields mean “no constraint.”

Records are [`domain.Process`](../domain) values (command argv, status/desired, project/session tags, restart chain links, env **key names** only, etc.).

## Implementation

The production driver is [`store/sqlite`](sqlite/) (`modernc.org/sqlite`, pure Go). Open and migrate from the daemon composition root only:

```go
st, err:= sqlite.Open(filepath.Join(cfg.StateDir, "pmmcp.db"))
//...
if err:= st.Migrate(ctx); err != nil {... }
// st satisfies store.ProcessStore
```

## What this package does not do

- Open or migrate a database
- Define groups, events, or audit tables (future extensions may add sibling interfaces)
- Enforce authz — callers (daemon + authz) own that

## Tests

There are no unit tests in this package itself (no statements to cover beyond type definitions). Contract behavior is covered under `internal/store/sqlite`.
