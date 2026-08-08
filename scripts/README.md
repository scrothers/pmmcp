# scripts/

| Script | Purpose |
|--------|---------|
| `license-headers.sh` | Check or apply Apache-2.0 headers on first-party Go sources (`check` / `fix`) |

`task license` / `task license:fix` wrap `license-headers.sh`. CI runs the check job on every PR.

Full local completeness proof (fmt, license, unit/e2e/integration, build, cross-build) is `task verify` in the root `Taskfile.yml` — not a shell script here.
