# agents: logcap

## role
Process **log capture, rotation, tail/grep/errors, structured level filter, tar.gz export**, and **secret redaction on write paths** so logs never store known secret material in cleartext.

## surface
| Symbol / area | Notes |
|---------------|--------|
| `Capturer` / open helpers | Per-process log dir; stdout/stderr streams |
| `RotatingWriter` | Size-based rotation, archives |
| `RedactWriter` | Wraps writer with `secret.RedactLine` (or equivalent) on the write path |
| `Tail` / `Grep` / `Errors` | Read APIs with stream options; line length caps |
| `FilterLevel` | Structured JSON/heuristic level filtering |
| `ExportTarGz` / `ExportTarGzWithOptions` | Bundle logs (+ optional archives) with manifest |

## deps
- Project: `internal/secret` (redaction on write)
- Third-party: none load-bearing beyond stdlib compress/archive

## invariants
- Redact on **write** paths — do not rely solely on read-time scrubbing.
- Never log or export resolved secret values intentionally.
- Cap oversized lines; stream normalize stdout/stderr/both.
- Hermetic unit tests use `t.TempDir()` only.

## tests
- `logcap_test.go`, `capturer_*`, `writer_*`, `read_*`, `structured_*`, `export_*`, `redact_writer_*` (+ fault tests).
- Unit tests hermetic (`t.Parallel()` when safe). **Coverage floor: ≥80% statements** for this package (CI). Do not add production seams only to hit residual lines. Meaningful assertions beat line hits.

## do-not
- Bypass RedactWriter for production captures “for debugging.”
- Read/write outside process log dirs without bounds.
- Chase residual I/O error branches with production hooks.

## related
- `internal/secret` redaction, daemon follow-logs, CLI log tools
