Start managed process "{{name}}" with pmmcp (project: {{project}}).

## Hard rules
- Use tool **pm_start**. Field **`command`** is a JSON **array of argv strings** — never a single shell string. Do not invent `sh -c` unless the user explicitly requires a shell.
- **Omit `sandbox`** so the strict default applies (fail-closed; no `~/.ssh`, credentials, or non-loopback egress). Do **not** set `sandbox` to `off`, `permissive`, or `standard` without explicit user approval.
- Secrets only via `env_files` / `secret://` refs — **never** secret values in `command` or other tool args.
- On **`daemon_unavailable`**: stop and tell the user to start `pmmcpd`. Do **not** fall back to `nohup`, `&`, or tmux.

## Steps
1. Call **pm_start** with `name`="{{name}}", `command`={{argv_json}} (set `project` if not current).
2. Call **pm_status** on the returned id/name — confirm `running` (or report `sandbox_failed` / `spawn_failed` / `name_conflict` clearly).
3. Before depending on a server: **pm_health_check** if health is configured, else **pm_ports** when a listen port is expected.
4. On failure: **pm_errors** then **pm_logs** — fix root cause before any restart.
