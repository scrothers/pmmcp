# Contributing

Thank you for interest in pmmcp.

**Full project law for implementers and coding agents:** [AGENTS.md](AGENTS.md) (architecture, security, MCP/CLI parity, testing/coverage floor, workflow). Package-specific briefs live under `internal/<pkg>/AGENTS.md`.

## Requirements

- Go **1.24+**
- `gofmt`, `go vet`
- `golangci-lint` v2 recommended (`go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest`)
- License headers: CI runs `google/addlicense` on every first-party `.go` file (see `task license`)

## Workflow

1. Branch from the default branch — do not commit directly to `main`.
2. Keep changes focused; prefer one logical change per PR.
3. Before opening a PR:

   ```bash
   task fmt
   task vet
   task test
   task lint      # if golangci-lint is installed
   task license   # Apache-2.0 headers on all Go files
   ```

   New Go files without a header: `task license:fix`.

4. Follow house Go style: no `func init()`, wrap errors with `%w`, `context.Context` as first parameter on I/O.

## Architecture

- Library code under `internal/` only.
- Driver pattern: parent interfaces import no drivers; `…/drivers` packages assemble implementations.
- Two binaries only: `pmmcp` (client) and `pmmcpd` (daemon).

## License

By contributing, you agree that your contributions are licensed under the Apache License 2.0.
