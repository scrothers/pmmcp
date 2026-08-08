# secret

Loads environment files, resolves secret references, decrypts SOPS material, and redacts sensitive values for logs and status.

## Env files

```go
m, err:= secret.LoadEnvFile(".env")
// or auto-decrypt when SOPS markers /.enc.* suffixes are present:
m, err = secret.LoadEnvFileMaybeSOPS("secrets.enc.env")
```

Supported dotenv lines: `KEY=VAL`, quoted values, full-line `#` comments, optional `export` prefix. Variables are **not** expanded (`${VAR}`). Duplicate keys: last wins.

## Redaction

```go
safe:= secret.RedactMap(env) // copy with sensitive values replaced
line:= secret.RedactLine("API_KEY=x") // → API_KEY=[REDACTED]
```

Keys whose names contain `TOKEN`, `SECRET`, `PASSWORD`, or `API_KEY` (case-insensitive) are redacted. Used by the daemon and `logcap.RedactWriter` so secrets are less likely to land in logs or status dumps. This is **best-effort**, not perfect scrubbing.

## Secret URIs (`secret://`)

Prefer references over pasting values into MCP/CLI arguments.

| URI | Meaning |
|-----|---------|
| `secret://keyring/pmmcp/<name>` | Value from the file keyring backend |
| `secret://sops:path/to/file#field` | Decrypt SOPS file; optional field key |
| `secret://file:path` | Read file contents (project-rooted by default) |
| `secret://env:VAR` | Read process environment |

```go
val, err:= secret.Resolve(ref, secret.ResolveOptions{
 ProjectRoot: project,
 Keyring: backend,
})
// Expand a whole env map:
out, err:= secret.ResolveEnvMap(env, opts)
// Existence only (no value returned):
ok, errMsg:= secret.Check(ref, opts)
```

Relative `file`/`sops` paths require `ProjectRoot`. Paths outside the project are denied unless `AllowFileOutsideProject` is set.

## File keyring

`FileBackend` stores secret **values** as `0600` files under a directory (`0700`). The daemon uses `$stateDir/keyring`. This is a portable stand-in for an OS keyring (no platform keyring dependency).

```go
kr, err:= secret.NewFileBackend(dir)
path, err:= kr.Set("api-token", value)
val, err:= kr.Get("api-token")
names, err:= kr.ListNames
```

## SOPS

- `DecryptFile(path)` — cleartext bytes via `github.com/getsops/sops/v3`
- `LooksLikeSOPS(path)` — filename conventions or body markers (`ENC[`, `sops:`)

Decrypted material is held in memory for injection into the child environment; it is not written world-readable.
