# sandbox/darwin

Applies sandbox profiles on **macOS** and rewrites child argv through `sandbox-exec` when available.

## Apply

```go
applied, err:= darwin.Apply(ctx, pol)
// applied.Profile, applied.Mode
```

`Apply` validates the policy (unknown profile rejected; strict/standard need a project root) and records the effective mode:

- **`off`** — profile off
- **`seatbelt`** — `sandbox-exec` is on PATH (isolation available)
- **`policy`** — path-policy only (seatbelt not available, or permissive)

It does not start a process; the local driver does that.

## Child isolation

```go
argv, ok:= darwin.TrySandboxExec(cmd, projectRoot, pol)
```

When `sandbox-exec` is present, the command becomes:

```text
sandbox-exec -p '<seatbelt profile>' <original argv…>
```

The generated seatbelt profile:

- Allows default operations so local tools work
- Denies file read/write under `~/.ssh`, `~/.gnupg`, `~/.aws` (and absolute entries from `Policy.ReadDeny`)
- Explicitly allows the project root

Off-macOS builds stub these helpers so the package still compiles in cross tests (`ok=false` / `IsolationAvailable==false`).

## Fail-closed

Strict starts without a project root or without `sandbox-exec` (via the local driver wrap) error out rather than running unsandboxed. See also deny `~/.ssh` under strict.
