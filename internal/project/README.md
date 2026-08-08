# project

Project **detection** and **identity keys** from a working directory. This package answers: “given a cwd, which project root am I in, and what stable string keys it?”

## Responsibilities

- Walk from `cwd` upward until a **root marker** is found.
- Treat the current directory as the project root when no marker exists.
- Produce a **canonical key** for maps and scoping (cleaned absolute path, symlinks resolved when possible).

## Non-responsibilities

- Persisting project rows or `proj-` ULIDs (not implemented here).
- Loading or validating `pmmcp.yaml` contents (declarative apply lives in `internal/declare`).
- Profile CRUD (see [`internal/profile`](../profile/)).
- Enforcing security boundaries (sandbox uses the root path as input elsewhere).

## Root markers

In order of check within a directory (any one is enough):

1. `.git` — directory **or** file (Git worktrees use a `.git` file)
2. `pmmcp.yaml`
3. `pmmcp.yml`

Detection uses `os.Lstat` presence only.

## API

### `Detect(ctx, cwd) (root, key, error)`

```go
root, key, err:= project.Detect(ctx, "/home/me/src/app/cmd/api")
// root = /home/me/src/app (where.git or pmmcp.yaml lives)
// key = project.Key(root) (stable map key)
```

- Resolves `cwd` to an absolute, cleaned path.
- Honors context cancellation between directory steps.
- If the walk reaches the filesystem root without a marker, returns the original absolute `cwd` as both conceptual root and key input.

### `Key(root) string`

Canonical identity for a root path:

1. `filepath.Abs` + `Clean`
2. `filepath.EvalSymlinks` when it succeeds
3. Clean again

Two paths that are the same directory via symlink should share one key (see `symlink_test.go`).

## Usage in the daemon

- **`project.current`** (and equivalents): call `Detect` on client-supplied or process cwd; return root + key.
- **Start / list scoping**: detect project for the request cwd; store `projects[key]=root`; filter process lists by project key so two repos never collide on process names.
- Sandbox roots for local children typically align with this project root.

## Design notes

### Why path keys?

MVP identity is “where on disk is this tree?” Agents hop repos by path; auto-detect must be cheap and offline. allows optional git-remote identity later — document any change carefully to avoid silent re-keying.

### Monorepos

Walking **up** means a nested service without its own marker inherits the outer git root. Put `pmmcp.yaml` in a subdirectory if that subtree should be a separate project.

### Not a security boundary

Anyone who can reach the daemon with a chosen cwd can aim detection at another tree they can see. Isolation is OS user + sandbox + authz, not project detection.

## Testing

```bash
go test./internal/project/
```

Covers nested git dirs/files, yaml markers, fallback-to-cwd, and symlink key stability.

## Related

- project and profile scope
- docs
- Phase: `13-project-detect.md`
- Companion: [`internal/profile`](../profile/)
