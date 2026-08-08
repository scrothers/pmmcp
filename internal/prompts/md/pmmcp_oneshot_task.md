Run a one-shot task (build, migrate, test) — not a long-lived service.

## Hard rules
- Prefer **pm_run** over **pm_start** so the job is oneshot and not boot-relaunched as a durable process.
- Field **`command`** is a JSON **argv array**, never a shell string.
- Omit `sandbox` (strict default). No secret values in tool args.
- On **`daemon_unavailable`**: stop; user starts `pmmcpd` — do not shell-background the task.

## Steps
1. **pm_run** with `command`={{argv_json}}, `wait`=true, `timeout_sec`={{timeout}}.
2. Read `status`, `exit_code`, and `timed_out` from the result.
3. On non-zero exit or timeout: **pm_errors** / **pm_logs** for that process id, then fix and re-run only if appropriate.
