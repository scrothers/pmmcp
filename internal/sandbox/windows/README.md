# sandbox/windows

Applies sandbox profiles on **Windows** with the same fail-closed contract as Linux and macOS.

## Apply

```go
applied, err:= windows.Apply(ctx, pol)
// applied.Profile, applied.Mode
```

| Profile | Result |
|---------|--------|
| `off` | Mode `"off"` |
| `strict` / `standard` | Requires a project root; Mode `"job-object"` |
| `permissive` | Mode `"policy"` |
| unknown | error |

Strict without a project root is an error — never a silent open host.

## What enforces isolation?

| Layer | Where | What it does |
|-------|--------|----------------|
| Path policy | `sandbox.Policy` | `AllowsRead` / `AllowsWrite` (deny `.ssh`-style paths under restrictive profiles) |
| Job Objects | `process/local` (post-start) | Tree kill, kill-on-close for supervised children |
| Container substitute | container driver | Stronger FS isolation when local-OS fidelity is weaker |

Unlike Linux (bubblewrap) and macOS (`sandbox-exec`), this package does **not** rewrite the child command line for a filesystem jail. Job Object mode signals that the local driver attaches a Job after spawn; path policy and optional containers cover secret-path denials.

## Future work

Restricted tokens, AppContainer, or a container-as-strict-path mode may strengthen isolation without changing the `Apply` signature.
