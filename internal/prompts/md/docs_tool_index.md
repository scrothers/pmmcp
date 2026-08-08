# pmmcp tool index

Use **tools/list** for the full catalog. Core tools:

| Need | Tools |
|------|--------|
| Lifecycle | pm_start, pm_stop, pm_restart, pm_list, pm_status, pm_remove, pm_run, pm_wait, pm_enable, pm_disable, pm_health_check |
| Logs | pm_logs, pm_grep, pm_errors |
| Declare | pm_validate → pm_diff → pm_apply; pm_declare_show; pm_import |
| Groups / profiles | pm_group_*, pm_profile_* |
| Events / audit | pm_events, pm_audit_query, pm_metrics_snapshot |
| Secrets | pm_secret_list, pm_secret_ref_check, pm_secret_set |
| Meta | pm_whoami, pm_daemon_info, pm_project_current |

## Always
- `command` / argv = **string array**, not a shell string.
- Default sandbox is **strict** (fail-closed); do not relax without user approval.
- Secrets by **reference**, never raw values in tool args.
- **daemon_unavailable** → user must start pmmcpd; do not shell around it.
