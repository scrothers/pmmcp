# watch

Package `watch` provides a **debounced filesystem watcher** for hot-reload workflows: when a watched file (or directory mtime) changes, consumers receive a single `Event` after a quiet period and typically restart the associated process.

Design: filesystem watches for hot reload.

## Why polling?

The MVP uses **mtime + size polling** (default every 200ms) so unit tests stay hermetic and free of platform `fsnotify` quirks. The public API (`Watcher`, `Event`, options, `Add` / `Start` / `Events` / `Close`) is intentionally backend-agnostic so an fsnotify implementation can share the same surface later.

## Quick start

```go
w:= watch.New(
 watch.WithDebounce(100*time.Millisecond),
 watch.WithPollInterval(50*time.Millisecond),
)
if err:= w.Add("/path/to/app"); err != nil {
 return err
}
ctx, cancel:= context.WithCancel(context.Background)
defer cancel
w.Start(ctx)
defer w.Close

for ev:= range w.Events {
 // debounced change at ev.Path, time ev.At
 _ = restartProcess(ctx, processID)
}
```

## Types

### Event

| Field | Meaning |
|-------|---------|
| `Path` | Absolute path that changed (the path registered with `Add`) |
| `At` | Wall time when the debounced event was emitted |

### Watcher

| Method | Behavior |
|--------|----------|
| `New(opts...)` | Construct; does not start polling |
| `Add(path)` | Register file or directory; must exist; stored as absolute path |
| `Start(ctx)` | Begin poll loop in a goroutine; safe to call once |
| `Events` | Receive-only channel of debounced events (buffer 64) |
| `Close` | Stop loop, close `Events`; idempotent |

### Options

| Option | Default | Description |
|--------|---------|-------------|
| `WithDebounce(d)` | 300ms | Minimum time after first change before emit |
| `WithPollInterval(d)` | 200ms | How often paths are `Stat`’d |

Non-positive durations are ignored (defaults retained).

## Change detection

Each tick:

1. `Stat` every registered path.
2. If **mtime is newer** than the last snapshot **or size differs**, update the snapshot and mark the path pending (first change time) if not already pending.
3. For each pending path whose age ≥ debounce, emit `Event` and clear pending.

Size is included so content writes that do not move mtime across a second boundary still fire.

### Directories

Directories are watched by **directory mtime only** — not recursive. Nested file creates may or may not update the parent directory mtime depending on the OS and filesystem; for reliable hot reload of a tree, register specific files or a path the editor actually touches.

### Missing paths after Add

If a path disappears after registration, `Stat` errors are skipped for that tick (no synthetic “delete” event). Re-create with a new size/mtime to fire again if the path returns under the same name.

### Slow consumers

If the events channel is full, the emit is **dropped** so the poll loop never blocks. Callers should keep handlers short or buffer work elsewhere.

## Lifecycle

```text
New → Add* → Start → Events → Close
 ↘ (optional) ctx cancel also stops loop
```

- `Start` after `Close`, or a second `Start`, is a no-op.
- `Add` after `Close` returns `watch: closed`.
- `Close` without `Start` closes channels immediately.
- Prefer daemon-lifetime context for long-lived watches (per-RPC contexts end when the call returns).

## Integration with the daemon

`internal/daemon` wires watches in `startWatchForProcess`:

- Debounce 100ms, poll 50ms (snappier than package defaults for interactive reload).
- On event → restart process; emit `process.watch_restart` / `process.watch_restart_error` domain events.
- `stopAllWatchers` closes all on daemon shutdown.
- User surface: watch set/status IPC methods and `pmmcp watch` CLI.

This package never restarts processes itself.

## Who imports this package

| Importer | Usage |
|----------|--------|
| `internal/daemon` (`supervise_loops.go`, `server.go`) | Hot-reload watchers per process ID |

## Files

| File | Contents |
|------|----------|
| `doc.go` | Package comment (polling + debounce rationale) |
| `watch.go` | Watcher implementation |
| `watch_test.go` | Debounced write + missing path |

## Testing

```bash
go test./internal/watch/ -count=1
```

Hermetic: `t.TempDir`, `Chtimes` to force mtime ordering, short debounce/poll for speed.

## Non-goals / future

- Recursive watches and ignore globs (`.git`, editor junk) — planned product defaults, not in this package yet.
- fsnotify backend for lower CPU / lower latency.
- Group-level watches (restart whole group) — daemon/config concern.
- Sandbox allowlisting of watch paths — enforced above this leaf.
