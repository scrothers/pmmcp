# sandbox/linux

Applies sandbox profiles on **Linux** and provides **bubblewrap** (and optional Landlock) isolation for children.

## Apply

```go
applied, err := linux.Apply(ctx, pol)
// applied.Profile, applied.Mode  // "landlock" | "policy" | "off" | …
```

`Apply` validates the policy and records the effective mode:

1. Unknown profile → error  
2. Strict/standard without a project root → error  
3. Landlock ruleset probe succeeds → Mode `"landlock"`  
4. Otherwise → Mode `"policy"` (path-policy helpers on `sandbox.Policy`)

The daemon process itself is **not** Landlock-restricted. Child isolation for strict/standard is done by rewriting the command line with bubblewrap in the local process driver.

## Bubblewrap

```go
argv, ok := linux.TryBwrapPolicy(cmd, projectRoot, &pol)
```

When `bwrap` is on PATH, the child runs under a minimal jail:

- Whole host root mounted **read-only**
- Project directory and temp **read-write**
- `/dev` and `/proc` available
- Secret trees (e.g. `~/.ssh`) covered with empty **tmpfs** so keys cannot be read through the ro-bind of `/`
- Network still shared (no network namespace) so local dev servers work

If bubblewrap is missing, the local driver **refuses** strict/standard starts (fail closed).

## Landlock

- `LandlockAvailable()` — kernel ABI probe  
- `LandlockRestrictPaths(pol)` — restrict the **current thread** (for short-lived helpers; not the daemon)

Landlock is allowlist-based; path-policy `ReadDeny` markers remain a second layer.

## Capability probes

| Helper | Meaning |
|--------|---------|
| `BwrapAvailable()` | `bwrap` on PATH |
| `LandlockAvailable()` | kernel supports Landlock |
| `IsolationAvailable()` | bwrap **or** Landlock |

Cross-compile stubs (`!linux`) report Landlock unavailable so tests can build on other OSes.
