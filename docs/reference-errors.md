# Error reference

Every failure pmmcp reports carries a **code**. The code drives the CLI's [exit status](cli.md#exit-codes), tells an agent whether a retry is worthwhile, and points you at the fix. This page is the lookup table.

An error prints as `pmmcp: <code>: <message>` on the CLI, and surfaces to an agent as an error tool-result whose text is `<code>: <message>: <detail>`. You can also read this reference from a running system as the MCP resource `pmmcp://docs/error-codes`.

---

## Error codes

These are the codes the daemon emits. "Retryable" means the same request may succeed later without you changing anything (it's advisory — set per situation, not fixed per code); the value shown is the typical case.

| Code | Exit | Typically retryable | What it means & what to do |
|------|------|:---:|-----------------------------|
| `daemon_unavailable` | 3 | yes | The client couldn't reach the daemon. Start `pmmcpd` (or the service); if you overrode paths, make the client's `PMMCP_IPC_ENDPOINT`/`PMMCP_STATE_DIR` match. → [Operations](operations.md#the-daemon-is-not-running) |
| `ipc_version_mismatch` | 8 | no | Client and daemon are incompatible versions. Rebuild both from the same source. → [Operations](operations.md#version-mismatch) |
| `invalid_argument` | 2 | no | A required field is missing or malformed (empty argv, bad flag, a value where none is allowed). Fix the command. |
| `not_found` | 4 | no | No process/group/profile by that name or id in this project. Check `pmmcp list`; you may be in the wrong project (cwd). |
| `permission_denied` | 5 | no | The action needs a capability the caller doesn't hold — e.g. relaxing the sandbox below the default without `sandbox:relax`, or acting on another session's process without a share. → [Security](security.md#capabilities-and-roles) |
| `conflict` | 6 | no | The operation conflicts with current state. |
| `name_conflict` | 6 | no | A process with that name already exists in the project/profile. Use a different name, or `--replace` / remove the old one first. |
| `already_exists` | 6 | no | The resource is already present. |
| `sandbox_failed` | 7 | no | The requested sandbox could not be applied, so the process did **not** start (fail-closed). Install the mechanism (`bwrap` on Linux), or choose a profile the host supports. → [Security](security.md#the-sandbox) |
| `spawn_failed` | 1 | no | The child couldn't be launched — bad executable path, missing binary, permission on the program itself. Check `command[0]` and `cwd`. |
| `failed_precondition` | 1 | sometimes | The system isn't in a state where this makes sense (e.g. acting on a process mid-transition). Re-check status and retry. |
| `unimplemented` | 1 | no | The operation isn't available in this build. |
| `internal` | 1 | sometimes | An unexpected daemon-side error. Check the daemon log; file a bug with the audit/event context. |

Anything without an explicit mapping exits `1`. A `nil` error exits `0`.

---

## Validation errors (`pmmcp.yaml`)

`pmmcp validate` / `pm_validate` reject a declaration by **policy code**. These are not daemon error codes — they're the specific reasons a `pmmcp.yaml` was refused, reported all at once so you can fix everything in one pass. See [Declarative](declarative.md#what-gets-rejected).

| Policy code | Rejected because |
|-------------|------------------|
| `sandbox_required` | A service (or the defaults) set `sandbox: off` — the declarative path won't accept an unsandboxed service. |
| `argv_shell_risk` | An `argv` invokes a shell (`bash -c`, `sh -c`, `powershell -Command`, `cmd /c`, …). Declarative services must be real argv, not shell strings. |
| `path_outside_project` | A watch path (or similar) escapes the project root. |
| `port_privileged` | A host-facing port below 1024. |
| `webhook_url_denied` | A webhook URL points at a blocked target (loopback, private/link-local, cloud metadata, or a bad scheme). |

Structural problems are reported too: a missing/`unsupported apiVersion` (must be `pmmcp.dev/v1alpha1`), a missing/`unsupported kind` (must be `Project`), a missing `argv`, an unknown `depends_on` target, or — with strict parsing — any **unknown field**.

---

## Reading errors as an agent

- **`daemon_unavailable` is the one to handle gracefully.** It's retryable and it means "ask the human to start the daemon," not "try a different tool." Don't fall back to shelling out.
- **`permission_denied` and `sandbox_failed` are policy, not bugs.** They mean pmmcp refused to do something unsafe. Surface the reason; don't try to route around it.
- **`invalid_argument` is your input.** Re-read the tool's parameters (argv as a `command` array, `name` required, no secret values in args) and correct the call.

Full model behind these: [Security](security.md). Where they show up operationally: [Operations](operations.md).
