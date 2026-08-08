Diagnose process "{{name}}" with pmmcp. Prefer pmmcp tools over shell (`ps`, `kill`, `nohup`) unless the daemon is unreachable.

## Steps
1. **pm_status** (`name` or `id` = "{{name}}") — record status, exit code, sandbox, health, ports.
2. **pm_errors**, then **pm_logs** (raise `lines` if needed) — extract the failure signal from process output.
3. **pm_events** for this process — restarts, crashes, unhealthy transitions.
4. If still unclear: **pm_ports**, **pm_runtime_info**, **pm_audit_query** (who changed what).
5. Only after you understand the failure: fix config/code, then **pm_restart** (or **pm_stop** + corrected **pm_start**). Do **not** restart in a blind loop.

## Hard rules
- On **`daemon_unavailable`**: ask the user to start `pmmcpd`; do not shell around pmmcp.
- Do not relax sandbox or put secrets in tool args while debugging.
