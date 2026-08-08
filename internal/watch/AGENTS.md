# agents: watch

## role
Filesystem watches with **debounce** for hot reload. MVP uses **mtime polling** (default ~200ms) so unit tests stay hermetic without platform notify quirks.

## surface
| Symbol / area | Notes |
|---------------|--------|
| `Watcher`, `New(opts...)` | Poll loop; event channel |
| `Event` | Path/change payload after debounce |
| Options | `WithDebounce`, `WithPollInterval`, `WithMaxWait` |
| `Add`, `Start`, `Close`, `Events` | Lifecycle; Start idempotent |

## deps
- Project: none
- Third-party: none (stdlib)

## invariants
- Debounce coalesces burst writes into a single Event.
- Missing/vanished paths are skipped, not fatal storms.
- Close stops the loop cleanly; no goroutine leaks.
- No secret material expected in watch paths; still do not log file contents.

## tests
- `watch_test.go`, `coverage_test.go`, `internal_test.go`.
- Unit tests hermetic (`t.Parallel` when safe; careful with timers). **Coverage floor: ≥80% statements** for this package (CI). Do not add production seams only to hit residual lines. Meaningful assertions beat line hits.

## do-not
- Depend on OS fsnotify for unit correctness.
- Watch outside project without policy at higher layers (declare checks watch paths).
- Block forever without ctx/Close.

## related
- `internal/declare` watch policy, daemon hot-reload wiring
