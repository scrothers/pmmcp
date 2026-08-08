# AGENTS.md — pmmcp

**Law for every coding agent and human implementer working in this repository.**  
Self-contained: you do not need host-level CLAUDE.md or external house rules to build correctly here.

**pmmcp** is an agent-native local process platform: a long-lived user daemon (`pmmcpd`) owns processes, logs, and state; a thin client (`pmmcp`) speaks CLI and MCP over a private socket. Module: `github.com/scrothers/pmmcp`. License: Apache-2.0.

---

## How to use this file

**Read order for any change:**

1. **This file** — global product, security, architecture, testing, and workflow law.
2. **`internal/<pkg>/AGENTS.md`** for every package you will touch (see schema below).
3. **Code** — briefs can lag; when in doubt, the code and tests win for local facts, but they must not violate this file’s security or architecture law.
4. **`docs/`** — human-facing behavior and UX wording when the change is user-visible.

**Hierarchy:** package `AGENTS.md` files **specialize**; they **never override** root security, architecture, coverage doctrine, or parity rules.

**Package AGENTS.md schema** (every `internal/**/AGENTS.md`):

| Section | Required |
|---------|----------|
| `## role` | yes |
| `## surface` | yes (table) |
| `## deps` | yes |
| `## invariants` | yes |
| `## tests` | yes (include ≥80% floor, or N/A if generated) |
| `## touch map` | preferred; required for control-plane/security packages |
| `## do-not` | yes |
| `## related` | optional |

Title form: `# agents: <path under internal/>` (example: `# agents: process/local`).

---

## Product identity

### Is

- A **user-scoped daemon** that is the sole parent of managed processes.
- **Named, project-scoped processes** with prefixed ULID IDs, status, logs, groups, restarts, health checks.
- **Agent-native control**: MCP tools (`pm_*`) and CLI share one catalog and one IPC API.
- **Strict sandbox by default** (OS-specific), fail closed when isolation cannot apply.
- **Secrets by reference** (`secret://`), redaction on log paths, audit of who did what.
- **Declarative** `pmmcp.yaml` apply/diff plus Procfile/Compose import.

### Is not

- Kubernetes, nomad, or a cluster orchestrator.
- A shell/session manager (tmux/screen replacement).
- A public multi-tenant or network-exposed control plane.
- A Node-only process manager or a build system.
- A place to run untrusted multi-user workloads as root.

### Load-bearing user promises

| Promise | Implication for implementers |
|---------|------------------------------|
| Daemon outlives clients | Clients never “own” child PIDs; supervision lives in `pmmcpd` |
| No surprise daemon spawn | Missing daemon → `daemon_unavailable`; MCP/CLI must not auto-start `pmmcpd` |
| Same OS user only | Private UDS/named pipe; peer UID filter where available |
| Argv, never implicit shell | `exec` of argv list; explicit `["/bin/sh","-c",…]` is visible and reviewable |
| Agents can prove work | Logs, events, audit, status — no silent orphans |

Marketing prose lives in `README.md`. This section is operational.

---

## Security model (must follow)

Aligned with `SECURITY.md`. Violations are product bugs, not style nits.

| Control | Rule |
|---------|------|
| **Control plane** | Private UDS (or Windows named pipe) only. Socket/pipe mode `0600` / owner ACL. Linux: `SO_PEERCRED` rejects other UIDs. |
| **No public API** | No default TCP/HTTP admin listener. Do not “temporarily” bind `0.0.0.0` for convenience. |
| **Sandbox** | Default **strict**. Fail closed if the OS mechanism is missing for strict/standard. Relaxing isolation is capability-gated and **audited**. |
| **Secrets** | Prefer env-files + `secret://` refs. Never put raw secret values in MCP tool args, CLI argv, audit detail, or status dumps. Resolve into **child env at launch only**. Redact known secret shapes on log write paths. Keyring dir `0700`, files `0600`. |
| **Authz** | Every sensitive method goes through capabilities/roles. Agent role must not exfiltrate stored secret *values*. |
| **Webhooks** | SSRF-aware: block link-local/metadata by default; allowlist when expanding. |
| **State paths** | Prefer tight permissions on state/log dirs; warn on loose env-file modes. |

**Do not:**

- Log resolved secret values “for debugging.”
- Disable peer-cred checks or sandbox fail-closed paths to make a test green in production code paths.
- Add a network control listener without an explicit, reviewed design change to this document and SECURITY.md.
- Store secrets in desired-state YAML; store URIs/refs only.

---

## MCP / CLI / API parity

One catalog. Two costumes (CLI and MCP). One daemon.

### Adding or changing a tool

All of the following must stay in sync:

1. `internal/api` — `Method*` constant and `AllMethods` (and DTOs if needed).
2. `internal/daemon` — handler for that method (authz included).
3. `internal/cli` — `ToolMethod` + `ToolDescription`.
4. CLI verb **or** an entry in intentional omissions (documented why).
5. Tests that enforce catalog parity (`catalog_test.go` and friends).
6. User-visible docs under `docs/` when behavior is user-facing.

**Current pin:** **65** `pm_*` tools in `ToolMethod`. `api.AllMethods` is larger by design (includes `hello` handshake and any non-tool methods). Changing the tool count is deliberate product work, not a drive-by.

### Client rules

- `internal/cli` is a **thin IPC client**. No process supervision, no owning children.
- `pmmcp mcp` is stdio MCP adapter to the same API.
- Payload parsing stays strict: malformed input → clear error, never silent fallback that invents meaning.

### Resources and prompts

- MCP resources/prompts: `internal/mcp`, `internal/prompts`.
- Prompts must not instruct agents to dump secrets or disable sandbox casually.

---

## Repository map

| Path | Role |
|------|------|
| `cmd/pmmcp`, `cmd/pmmcpd` | Thin mains: context + `Execute` only |
| `internal/cli`, `internal/daemoncmd` | Client and daemon command trees |
| `internal/*` | All product logic |
| `internal/AGENTS.md` | Package index |
| `internal/<pkg>/AGENTS.md` | Per-package agent brief |
| `internal/<pkg>/README.md` | Human package overview |
| `api/proto/` | Protobuf sources (`buf`) |
| `internal/api/gen/` | **Generated** gRPC stubs — do not hand-edit |
| `test/integration/` | `//go:build integration` |
| `test/e2e/` | `//go:build e2e` |
| `docs/` | Product documentation |
| `packaging/` | systemd / launchd / windows service artifacts |
| `scripts/` | license header check/fix |
| `Taskfile.yml` | Local tasks: build, test, lint, license, verify |
| `.github/workflows/ci.yml` | Multi-OS tests, license, govulncheck, proto, cross-build |

---

## Architecture

### Package layout

- All library code under `internal/`. `cmd/*` binaries stay thin.
- Organize files by semantic unit (type + close helpers), not one-type-per-file dogma.
- Every package has a `doc.go` (or package comment) and an `AGENTS.md`.

### Import graph

- **Acyclic**, leaf → application.
- Leaves import no higher-level project packages.
- If A and B would cycle, extract a shared leaf.

### Driver / selector pattern

For pluggable families (`process`, `engine`, …):

1. **Parent** `internal/<family>/` — interfaces + shared types; imports **no** driver.
2. **Driver** `internal/<family>/<driver>/` — one implementation; `var _ parent.Iface = …`; own tests.
3. **Selector** `internal/<family>/drivers/` — **only** package that imports parent + all drivers; explicit `Open` / `ByName` / `Default`.

Adding a backend = new subpackage + one line in `drivers`. Never bend the parent interface for one driver. **No `func init()`** registration.

### Wiring and globals

- **No `func init()`** anywhere (`gochecknoinits`).
- Constructor injection: `New(deps…)`.
- Accept interfaces where *consumed* (narrow); return concrete structs.
- No package-level mutable globals or singletons.
  - Exceptions: `slog.Default()` set once from `main`; build-time `version` vars via `-ldflags`.
  - **Do not** add package-level function variables solely so unit tests can paint residual lines.
- Functional options: `type Option func(*T)` with `With…` when optional config belongs on a type.

### Context

- `context.Context` is the **first** parameter of every function that does I/O, blocks, or can cancel.
- Propagate the caller’s context; do not invent `context.Background()` mid-stack (only at process ingress: `main`, accept loops).
- **Never store a `context.Context` in a struct field.** Hold `cancel` or a done channel if needed.
- Do not build contexts inside loops (`fatcontext`).

### Identifiers

- Record IDs: **prefixed lowercase Crockford-base32 ULIDs** via `internal/id` (e.g. `proc-…`, `group-…`).
- **Never UUIDs** for new IDs.

### Errors

- Wrap: `fmt.Errorf("pkg: op: %w", err)` — lowercase, no trailing punctuation, always `%w` when chaining.
- Sentinels: `var ErrXxx = errors.New(...)`; compare with `errors.Is` / `errors.As`.
- Domain codes and CLI exit map: `internal/domain`.
- **Never `return nil, nil`.** Do not log-and-return; return and let the caller decide (log only when deliberately swallowing).

### Logging

- `log/slog` only in app code.
- Never log secrets, tokens, or full env maps.

### Concurrency and shutdown

- Long-lived goroutines rooted from daemon lifecycle with cancel on shutdown.
- Graceful drain with timeouts; `time.Ticker` always `Stop`’d.
- Pure domain stays single-threaded by contract (no goroutines in `internal/domain`).

---

## Go style (condensed)

- `gofmt -s` clean; imports grouped stdlib → third-party → project.
- Exported identifiers: doc comment starting with the name; American English (`canceled`, `behavior`).
- Naming: no package stutter (`id.New` not `id.NewID` noise); acronyms uniform (`ID`, `URL`, `HTTP`, `JSON`, `ULID`).
- Receivers: short, consistent (`s *Server`).
- Use stdlib named constants (`http.MethodGet`, `http.StatusOK`).
- Tables and early returns over deep nesting; no speculative abstractions.
- Pre-PR: `gofmt`, `go vet`, `golangci-lint` clean. Do not disable linters to silence; fix or leave with clear reason.
- New packages: `doc.go`, tests, listed in `internal/AGENTS.md`.

---

## Testing and coverage

### Coverage floor: 80%

CI enforces **≥80% statement coverage per package** under `./internal/...` (generated `internal/api/gen/` excluded).

| Rule | Detail |
|------|--------|
| **Floor, not ladder** | Meet 80%. Do **not** grind packages to 90–100%. |
| **Assertions > paint** | A test that proves behavior beats a test that only executes a line. |
| **No coverage theater** | Forbidden: package-level func hooks, `SetXForTest`, swapping `crypto/rand.Reader`, or dead stubs **solely** to cover effectively unreachable branches. |
| **Plausible failures** | Prefer bad input, missing files, authz deny, cancel, corrupt state, missing sandbox binary. Skip “stdlib failed on infallible input.” |
| **Domain** | Pure and cheap to test thoroughly — high coverage is natural, still **not mandated above 80%**. |
| **Excluded** | Generated code; thin `cmd/` mains are not the coverage story. |

### Test tiers (exactly one tier per file)

| Tier | Location | Build tag | Default `go test ./...` | May touch |
|------|----------|-----------|-------------------------|-----------|
| **Unit** | `internal/.../*_test.go` | *(none)* | yes | fakes, `t.TempDir()`, hermetic only |
| **Integration** | `test/integration/` | `integration` | **no** | real daemon socket / engines as designed |
| **E2E** | `test/e2e/` | `e2e` | **no** | full binaries + MCP |

Build tags are load-bearing: integration/e2e must not compile in the default suite.

### Unit test rules

- Stdlib `testing` only — **no testify**, assert, require, or mock generators.
- `t.Parallel()` unless the test must avoid it (prefer designs that allow parallel).
- Hand-written fakes: embed interface, override used methods; panic on unexpected calls is fine.
- Speed: keep unit tests fast; no real network, no real cloud, no files outside `t.TempDir()`.
- Missing optional host tool in a unit test: do not skip away a required security property — use fakes or move to integration with explicit require env (`PMMCP_REQUIRE_SANDBOX`, etc.).

### Integration / e2e

- Black-box preferred.
- If a service is required for the job, **fail** when missing (`t.Fatal`) rather than `t.Skip` to fake green — unless the test is explicitly optional and documented.

---

## Dependencies

- Prefer the standard library and small, justified modules.
- **Heavy dependency = explicit product decision.** Example lesson: embedding full SOPS pulls multi-cloud KMS SDKs for one decrypt call — feature good, graph costly. Prefer lean approaches (exec host tool, narrower library, optional build tag) when they match the product.
- Do not drive-by bump dependencies inside feature PRs; isolate bumps; keep `go mod tidy` clean.
- New direct require: say why in the commit/PR body.

---

## Change recipes

### Add or change an MCP tool / IPC method

1. `internal/api` method + types + `AllMethods`.
2. Daemon handler + authz.
3. `cli.ToolMethod` + `ToolDescription` + CLI verb or omission.
4. Parity tests green; update `docs/` if user-visible.
5. Do not ship a tool that returns raw secret values to agents.

### Add a process or engine driver

1. New package under `internal/process/<name>/` or `internal/engine/<name>/`.
2. Implement parent interface; compile-time `var _` anchor.
3. Register in `…/drivers` only.
4. Unit tests with fakes; integration if talking to a real engine binary.
5. Parent package stays driver-free.

### New `internal/` package

1. `doc.go`, implementation, tests (≥80% floor).
2. `AGENTS.md` + `README.md` using the schema above.
3. Row in `internal/AGENTS.md` index.
4. Wire explicitly from daemon/`main` or selectors — no `init()`.

### Touch secrets, sandbox, authz, ipc peer filter, webhooks

1. Re-read **Security model** in this file.
2. Prefer fail-closed behavior.
3. Tests for deny paths matter more than happy-path paint.
4. Update `SECURITY.md` / `docs/security.md` / `docs/secrets.md` when the model changes.

### User-visible behavior

Update `docs/` (and README only if the top-level story changes). Code comments are not a substitute for docs.

---

## Workflow and definition of done

### Local gate (before you call a change done)

```bash
task fmt
task vet
task test
task lint      # golangci-lint
task license   # Apache headers on Go files
```

Full local proof: `task verify`.

Coverage check (Linux CI shape): `go test -cover ./internal/...` — every non-gen package ≥80%.

### Definition of done

- [ ] Behavior matches the product promises above
- [ ] Tests cover the **behavior** (not residual unreachable lines)
- [ ] `gofmt` / `vet` / lint clean
- [ ] New/changed Go files have license headers (`task license`)
- [ ] Catalog/docs/parity updated when tools or UX change
- [ ] Package `AGENTS.md` updated if surface or invariants changed
- [ ] No security regress (sandbox, secrets, peer cred, SSRF)
- [ ] No dependency bulk without justification

### Git

- Branch first; do not commit directly to `main` / `master` / `develop`.
- Atomic commits; imperative subjects.
- No force-push to shared branches; no `--no-verify` as habit.
- No AI co-author trailers unless the operator asks.

### Multi-agent safety

- Do not two-write the same files from concurrent agents.
- Partition by package path or use isolated worktrees.
- If you detect unexpected mid-edit churn, **stop** and reconcile — do not overwrite blindly.

---

## License

- Apache License 2.0 (`LICENSE.md`).
- Every first-party `.go` file carries the standard header.
- Check: `task license` / `scripts/license-headers.sh check`.
- Fix: `task license:fix`.
- Copyright holder in headers: **Steven Crothers** (see license script defaults).
- Generated code under `internal/api/gen/` is excluded from header stamping.

---

## Hard do-not (checklist)

- Do not hand-edit `internal/api/gen/**` — change `.proto` and `buf generate`.
- Do not chase 100% coverage or invent unnatural error paths for the meter.
- Do not add `func init()`.
- Do not use testify or UUID primary keys.
- Do not shell-inject process commands; keep argv explicit.
- Do not log or return raw secrets to agents.
- Do not auto-start the daemon from MCP/CLI on dial failure.
- Do not open a public control plane.
- Do not weaken sandbox or peer-cred checks to silence tests.
- Do not put supervision logic in the client package.
- Do not add a catalog tool without full parity (api + daemon + cli map + tests).
- Do not auto-bump unrelated dependencies in feature work.
- Do not force-push protected branches or commit to main directly.

---

## Further reading

| Need | Location |
|------|----------|
| Package index | `internal/AGENTS.md` |
| Package brief | `internal/<pkg>/AGENTS.md` |
| Human product docs | `docs/` |
| Security policy | `SECURITY.md` |
| Contributing (short) | `CONTRIBUTING.md` |
| Secrets guide | `docs/secrets.md` |
| MCP overview | `docs/mcp.md` |
| Harness integrations | `docs/integration/` |
| CI | `.github/workflows/ci.yml` |

When this file and a package brief disagree on **global** law, **this file wins**. When they disagree on **local symbols**, re-read the code and fix the brief.
