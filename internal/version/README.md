# version

Package `version` holds **build-time version metadata** shared by both pmmcp binaries and the daemon’s status surface.

## Variables

| Variable | Default | Purpose |
|----------|---------|---------|
| `Version` | `0.0.0-dev` | Semantic / release version string |
| `Commit` | `unknown` | Git commit identifier |
| `BuildDate` | `unknown` | Build timestamp |

All three are package-level `var`s so the Go linker can override them with `-ldflags -X` without source changes.

## String()

```go
version.String()
// e.g. "0.0.0-dev (commit=unknown date=unknown)"
// or "1.2.3 (commit=abc1234 date=2026-08-07T12:00:00Z)"
```

Used by:

- `pmmcp version` / `--version` / `-v` (`internal/cli`)
- `pmmcpd version` / `--version` / `-v` (`cmd/pmmcpd`)

## Link-time injection

Example build:

```bash
go build -ldflags "\
 -X github.com/scrothers/pmmcp/internal/version.Version=1.2.3 \
 -X github.com/scrothers/pmmcp/internal/version.Commit=$(git rev-parse --short HEAD) \
 -X github.com/scrothers/pmmcp/internal/version.BuildDate=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
 -o bin/pmmcp ./cmd/pmmcp
```

If flags are omitted (typical local `go run` / `go build`), defaults remain so tools still print a usable line.

## Who imports this package

| Importer | Usage |
|----------|--------|
| `cmd/pmmcpd` | CLI version subcommand |
| `internal/cli` | CLI version subcommand for `pmmcp` |
| `internal/daemon` | `DaemonVersion` and related status/API fields set from `version.Version` |

## Design notes

- **No runtime git detection** — keeps the package hermetic and fast at process start.
- **No semver library** — strings only; comparison and upgrade logic live elsewhere if needed (the product design).
- **Single source of truth** for both client and daemon identity strings so releases stay aligned.

## Files

| File | Contents |
|------|----------|
| `version.go` | Package comment, vars, `String` |

No tests in-package: pure data + string concat with no branches worth covering separately.
