# Security Policy

## Supported versions

Development builds prior to 1.0 receive best-effort fixes on the default branch.

## Reporting a vulnerability

**Do not open a public issue for exploit details.**

Report privately via **[GitHub private vulnerability reporting](https://github.com/scrothers/pmmcp/security/advisories/new)** (preferred — it keeps the report, discussion, and fix coordinated in one place). If you cannot use GitHub, email **steven@scrothers.com** with `[pmmcp security]` in the subject.

Include: reproduction steps, affected OS, sandbox profile in effect, and `pmmcp`/`pmmcpd` versions (`pmmcp version`, `pmmcp daemon-info`).

You can expect an acknowledgment within **72 hours** and a status update within **7 days**. Please allow a reasonable disclosure window before publishing details; we credit reporters in release notes unless you prefer otherwise.

## Security model (summary)

Security model summary:

| Control | Behavior |
|---------|----------|
| **Same-user local daemon** | Control socket/pipe restricted to installing OS user (`0600` / owner ACL). Linux `SO_PEERCRED` rejects other UIDs. |
| **No public control plane** | No default TCP/HTTP API; private UDS/named pipe only. |
| **Strict sandbox default** | Managed children confined per OS (below). Fail closed if isolation cannot apply for strict/standard. |
| **Secrets** | Prefer env-files and `secret://` refs; never put raw secrets in MCP tool args. Log capture redacts sensitive `KEY=value` lines. |
| **Webhooks** | SSRF controls: block link-local/metadata; allowlist recommended. |
| **No MCP auto-start** | Missing daemon returns `daemon_unavailable` — no surprise process spawn from agents. |

### Child sandbox by OS

| OS | Enforcement |
|----|-------------|
| **Linux** | bubblewrap (`bwrap`) FS jail; tmpfs over secret trees (`~/.ssh`, …); Landlock ABI probe; fail closed without bwrap for strict |
| **macOS** | `sandbox-exec` seatbelt profile denying secret paths; fail closed without sandbox-exec for strict |
| **Windows** | Job Object with kill-on-job-close + tree terminate; fail closed if job assignment fails for strict |

Same-UID malware on the host can still talk to the daemon — sandboxing protects **children**, not the user from themselves.

### State and logs

- State directory mode `0700`.
- Process log files mode `0600`.
- Audit records mutations (start/stop/remove/secret set).

## Hardening checklist (operators)

1. Do not chmod the socket group/world writable.
2. Do not run `pmmcpd` as root unless you understand the blast radius.
3. Prefer `sandbox: strict` for untrusted agent workloads.
4. Keep SOPS/age keys outside the project when possible.
5. Review webhooks allowlists before enabling delivery.

## Tests of interest

- `internal/process/local` — `TestStrictDeniesDotSSH*` / Windows Job Object stop 
- `internal/ipc` — peer credential same-UID 
- `internal/webhook` — SSRF rejection 
- `internal/authz` — capability matrix 

Full quality gate: `task verify` locally and `.github/workflows/ci.yml` in CI.
