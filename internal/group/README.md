# group

App groups: named multi-process collections with dependency-aware start/stop order.

## Overview

An **app group** is a first-class resource (`grp-…` ULID) scoped to a project. Members are named units that may declare `depends_on` other member names. The registry stores groups and computes topological start order (dependencies first) and stop order (reverse).

This matches: full app groups with a DAG, not mere tags on processes.

## When to use

- Daemon handlers for `group.create` / `remove` / `list` / `status` / `start` / `stop` / `restart`
- Any orchestration that needs deterministic member ordering before calling process start/stop

The package does **not** start processes itself. It owns group metadata and ordering; the daemon applies that order to the process manager.

## Key API

```go
r:= group.NewRegistry

g, err:= r.Create(group.Group{
 Name: "webstack",
 ProjectID: "proj-…",
 Members: []group.Member{
 {Name: "db"},
 {Name: "api", DependsOn: []string{"db"}},
 {Name: "web", DependsOn: []string{"api"}},
 },
})
// g.ID is grp-… when left empty

start, _:= r.StartOrder(g.ID) // ["db","api","web"]
stop, _:= r.StopOrder(g.ID) // ["web","api","db"]

got, _:= r.Get(g.ID)
list:= r.List("proj-…") // empty projectID → all
_ = r.Remove(g.ID)
```

### Sentinels

| Error | When |
|-------|------|
| `ErrNotFound` | Get/Remove/StartOrder on unknown ID |
| `ErrExists` | Create with duplicate ID |
| `ErrCycle` | Self-dependency or cyclic depends_on |

Other validation errors (empty name, unknown dependency name, duplicate member names) are plain `error` values with `group:` prefixes.

## Design notes

- **DAG semantics.** Member A `depends_on` B means B must start before A. Cycles are rejected at create time so bad graphs never enter the registry.
- **Stable ordering.** Topological sort (Kahn) breaks ties using the original members slice index so equal-independence members keep declaration order.
- **Cloning.** Create returns a deep copy; Get/List also clone so callers cannot mutate registry state through shared slices (`DependsOn`, `Members`).
- **Identity.** Group IDs are `grp-` ULIDs. Display `Name` is separate and unique within project at the product layer; this package requires non-empty name and unique ID only.
- **Process linkage.** `Member.ProcessID` holds a live `proc-` when bound; ordering APIs use **member names**, not process IDs.
- **In-memory.** Persistence (if any) is the daemon/store’s concern; this registry is the runtime source of truth for the process.

## Testing

```bash
go test./internal/group/
```

Covers CRUD, two-node and chain depends_on, reverse stop order, cycles, self-cycles, and unknown dependencies. Broader product coverage lives in `internal/daemon` tests.

## Related

- [`../id/`](../id/) — `grp-` generation
- [`../daemon/`](../daemon/) — handlers and group start/stop orchestration
- [`../domain/`](../domain/) — process status types used when aggregating group health
- (app groups), (identity), product features `groups-define-and-run`, `groups-depends-on`
