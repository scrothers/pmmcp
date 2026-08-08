# logcap

Process log capture, rotation, query, export, and redaction.

## Overview

Each managed process gets a log directory. `Capturer` owns the active `stdout.log` / `stderr.log` files (mode `0600`), rotates when size exceeds a threshold (default **10 MiB**, keep **5** archives, **gzip** rotated files), and higher layers query those files with tail/grep/error heuristics and optional structured level filters.

## When to use

- **Local driver** (`process/local`): create a capturer, open stdout/stderr for `exec.Cmd` redirect; wrap with `RedactWriter` when secrets must not hit disk.
- **Daemon handlers**: `Tail`, `Grep`, `Errors`, `FilterLevel`, `ExportTarGz` for MCP/CLI log tools and subscribe-logs bootstrap.
- **Not for** daemon diagnostic logs (`log/slog`) or domain events — those are separate streams.

## Key API

```go
c, err:= logcap.New(logDir, 0, 0) // defaults: 10MiB, 5 files, Compress=true
out, err:= c.OpenStdout
err = c.RotateIfNeeded

text, err:= logcap.Tail(dir, logcap.TailOptions{Stream: "both", Lines: 200})
text, err = logcap.Grep(dir, logcap.GrepOptions{Pattern: `ERROR`, Context: 2})
text, err = logcap.Errors(dir, logcap.ErrorsOptions{Lines: 100})
text, err = logcap.FilterLevel(dir, logcap.StructuredOptions{MinLevel: "error"})

err = logcap.ExportTarGz(dir, w)

rw:= &logcap.RedactWriter{W: file}
```

## Design notes

- **File layout is the contract.** Active names and rotation scheme are stable for tools and export bundles.
- **Read paths tolerate missing files** (empty string), so status works before first write.
- **Output is capped** (~256 KiB) so MCP/CLI responses stay bounded.
- **Structured filtering is best-effort:** JSON objects with `level`/`lvl`, or text patterns like `ERROR:` / `level=error`. Unrecognized lines rank as `info`.
- **Redaction** is write-side via `secret.RedactLine`; pair with env/secret handling.
- **Rotation renames under the capturer dir only**; compress drops the plain `.1` after writing `.1.gz`.

## Testing

```bash
go test./internal/logcap/...
```

All unit tests are hermetic (`t.TempDir`). Coverage includes defaults, rotate+gzip retention, structured filter, and export archive contents.

## Related

- ADRs: 013-logs-and-events, 014-secrets-handling
- See also: `docs/secrets.md`
- Consumers: `internal/process/local`, `internal/daemon`
- Sibling: `internal/secret` (redact rules)
