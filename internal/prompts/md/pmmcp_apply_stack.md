Reconcile this project's declarative stack (`pmmcp.yaml`), profile: {{profile}}.

## Hard rules
- Declarative path **rejects** `sandbox: off` and shell-wrapped argv (`sh -c`, etc.). Fix the file — do not bypass with unsandboxed **pm_start**.
- Secrets in YAML only as refs / env-files — never raw values.
- On **`daemon_unavailable`**: stop; user must start `pmmcpd`.

## Steps
1. **pm_validate** the `pmmcp.yaml` (path or yaml body). On any error: fix and re-validate. Never apply an invalid document.
2. **pm_diff** desired vs running — review creates, updates, and removals.
3. If the diff prunes or restarts services the user may care about, confirm intent first; otherwise proceed when the goal is clear.
4. **pm_apply** — creates/starts services in the diff. There is **no** separate `start` flag; apply performs the reconciliation.
5. Confirm with **pm_list** (and **pm_group_status** / **pm_health_check** when groups or health are defined).
