<!-- Keep it to one logical change. Split refactors from features. -->

## What & why

<!-- What does this change, and what problem does it solve? Link the issue if one exists. -->

Closes #

## Kind of change

- [ ] Bug fix
- [ ] Feature
- [ ] Refactor (no behavior change)
- [ ] Docs
- [ ] CI / tooling

## Gates (all must pass — CI enforces the same set)

- [ ] `task fmt` — `gofmt -s` clean
- [ ] `task vet` — `go vet` clean (all target GOOS)
- [ ] `task test` — unit suite green, race-clean
- [ ] `task lint` — golangci-lint clean
- [ ] `task license` — Apache-2.0 headers on new Go files
- [ ] Coverage holds the per-package floor (≥80%; new packages need tests + `doc.go`)

## Parity & docs (when applicable)

- [ ] New/changed `pm_*` tool: catalog (`ToolMethod` + `ToolDescription`) + daemon handler + CLI verb (or documented omission) all updated together
- [ ] User-facing behavior changed → wiki updated (https://github.com/scrothers/pmmcp/wiki)
- [ ] Package behavior changed → its `internal/**/AGENTS.md` / `README.md` still accurate
- [ ] Security-relevant change (sandbox, authz, secrets, IPC) → called out below

## Security notes

<!-- Does this widen what an agent can do? Touch the sandbox boundary, capabilities, secret handling, or IPC surface? Say so explicitly, even if the answer is "reviewed, no impact". -->
