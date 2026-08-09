Import "{{path}}" into a pmmcp declare draft (Procfile; Compose import is planned but not yet supported).

## Steps
1. **pm_import** with `path`="{{path}}".
2. **Review the draft and warnings before writing anything.** Common hazards:
   - shell-wrapped commands → rewrite as real argv arrays
   - secrets in env → convert to `secret://` refs or env-files
3. Merge into `pmmcp.yaml`, then **pm_validate**. Fix every error.
4. **pm_diff**, then **pm_apply** only when validation is clean and the user intent is clear.

## Hard rules
- Never **pm_apply** an unreviewed import.
- Do not keep shell argv or `sandbox: off` just because the import produced them.
- On **`daemon_unavailable`**: stop; user starts `pmmcpd`.
