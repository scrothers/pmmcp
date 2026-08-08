# sandbox

Platform-agnostic **sandbox profiles** and **path policy** for supervised processes.

Agents and the daemon decide *what* isolation means (`strict`, `standard`, `permissive`, `off`). OS-specific packages under `linux/`, `darwin/`, and `windows/` decide *how* to apply it.

## Profiles

| Profile | Intent |
|---------|--------|
| `strict` | Restrictive default (OSS). Needs a project root. Denies secret path classes (e.g. `~/.ssh`). |
| `standard` | Same isolation mechanisms as strict where the platform supports them. |
| `permissive` | Path policy only / relaxed (home may be writable). |
| `off` | No isolation. Explicit escape hatch. |

Unknown profiles are rejected. Strict and standard **fail closed** if there is no project root — the product never pretends a process is sandboxed when it is not.

## Policy API

```go
pol, err:= sandbox.DefaultPolicy(sandbox.Strict, projectRoot)
// pol.WritableRoots, pol.ReadDeny
ok:= pol.AllowsRead(path)
ok = pol.AllowsWrite(path)
```

- **Writable roots** (strict/standard): project directory + host temp.
- **Read deny**: markers such as `.ssh`, `.gnupg`, `.aws`, and `docker.sock`.
- Path checks are fail-closed for empty or unknown profiles.

## Platform packages

| Package | Role |
|---------|------|
| [`linux`](linux/) | Landlock probe + bubblewrap child wrap |
| [`darwin`](darwin/) | Seatbelt / `sandbox-exec` wrap |
| [`windows`](windows/) | Job Object mode + path policy |

The local process driver (`internal/process/local`) calls the platform wrap helpers when starting strict/standard children. The daemon builds a `Policy` before spawn and validates it.

## Design notes

-: sandbox is an MVP gate; strict is the OSS default.
- Isolation strength differs by OS.
- Kernel mechanisms may strengthen later without changing the `Policy` surface.
