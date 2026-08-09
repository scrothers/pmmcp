# agents: secret

## role
**Env-files**, **secret: / ** reference resolution (schemes: **keyring / sops / file / env**), local **file keyring**, **SOPS decrypt**, and **redaction** for log/status surfaces. Resolve values into child env at launch only — never return raw secrets to agents by default.

## surface
| Symbol / area | Notes |
|---------------|--------|
| `LoadEnvFile` / `LoadEnvFileMaybeSOPS` | dotenv parse; warn on loose modes; SOPS when markers/`.enc.*` |
| `ParseRef` / `LooksLikeRef` / `Resolve` / `Check` / `ResolveEnvMap` | `secret: / keyring|sops|file|env` |
| `Ref`, `ResolveOptions` | ProjectRoot containment; Keyring backend; `AllowFileOutsideProject` default false |
| `FileBackend` | Keyring dir **0700**, files **0600**; name traversal rejected |
| `DecryptFile` | `github.com/getsops/sops/v3/decrypt` (heavy dep — deliberate) |
| `Redactor` / package `RegisterValue` / `RedactLine` / `RedactMap` / `RedactedFor` | Named placeholders `***REDACTED:NAME***`; key-name + registered values + global patterns (AWS/GitHub/JWT/PEM) |

## deps
- Project: none (leaf-ish; used widely)
- Third-party: `github.com/getsops/sops/v3` (SOPS decrypt — large graph; product decision)

## invariants
- Never log resolved secret values.
- `file`/`sops` paths contained to project root via symlink-aware checks; fail closed without root.
- Keyring names validated against traversal (`..`, separators).
- Agent role must not exfiltrate stored secret **values** (authz `secrets:read_values` is full-only) — this package still must not print values.
- Env files: load but warn if group/other-readable.
- Redact on log write paths (callers + logcap).

## tests
- `secret_test.go`, `uri_test.go`, `keyring_test.go`, `redact_test.go`, `extract_test.go`, `coverage_test.go`, `whitebox_test.go`.
- Unit tests hermetic (`t.Parallel` when safe). **Coverage floor: ≥80% statements** for this package (CI). Do not add production seams only to hit residual lines. Meaningful assertions beat line hits.
- SOPS success paths may use test-only decrypt fakes; avoid requiring cloud KMS in unit tests.

## touch map
| Change | Also touch |
|--------|------------|
| URI schemes / ParseRef | daemon secret handlers, https://github.com/scrothers/pmmcp/wiki/Secrets, SECURITY.md |
| Redaction patterns/placeholder | logcap RedactWriter, daemon status dumps |
| FileBackend perms | install paths, doctor checks |
| SOPS dependency | go.mod isolation; prefer not expanding cloud SDKs further without review |
| Resolve containment | declare env refs, process start path |

## do-not
- Put raw secrets in MCP args, CLI argv, audit detail, or status dumps.
- Store secret values in desired-state YAML (refs only).
- Disable path containment for convenience in production defaults.
- Chase 100% coverage via unnatural crypto failures.

## related
- `internal/authz` (CapSecretsReadValues / CapSecretSet), `internal/logcap`, `https://github.com/scrothers/pmmcp/wiki/Secrets`
