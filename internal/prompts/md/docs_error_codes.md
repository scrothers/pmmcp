# pmmcp error codes

| code | exit | retry? | meaning / agent action |
|------|------|:------:|------------------------|
| daemon_unavailable | 3 | yes | Cannot reach pmmcpd. Tell the user to start it. Do **not** fall back to nohup/shell. |
| invalid_argument | 2 | no | Bad/missing fields (e.g. empty `command`). Fix the tool call. |
| not_found | 4 | no | Unknown name/id in this project — check **pm_list** / project scope. |
| permission_denied | 5 | no | Missing capability (e.g. sandbox relax). Surface; do not bypass. |
| conflict / name_conflict / already_exists | 6 | no | State clash — rename, remove, or replace as appropriate. |
| sandbox_failed | 7 | no | Isolation could not apply; process did **not** start (fail-closed). |
| ipc_version_mismatch | 8 | no | Client/daemon skew — rebuild both from the same source. |
| spawn_failed | 1 | no | Exec failed (missing binary, bad cwd). Fix `command[0]` / paths. |
| internal | 1 | maybe | Unexpected daemon error — report context; do not thrash retries. |

Validation policy codes on **pm_validate** (not the same as daemon codes): `sandbox_required`, `argv_shell_risk`, `path_outside_project`, `port_privileged`, `webhook_url_denied` — fix the YAML; do not work around.
