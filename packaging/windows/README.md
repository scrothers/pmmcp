# Windows

`pmmcpd` runs per user, started by a **logon Scheduled Task** at
`LeastPrivilege` — no Administrator, no system service, no elevation prompt.
The daemon confines children with a Job Object (`KILL_ON_JOB_CLOSE` + tree
terminate); the private named pipe is the same-user trust boundary.

## Recommended flow

```powershell
# 1. Write the start script + install notes (no elevation)
pmmcp install-service

# 2. Register the logon task from the checked-in template — and start it now
./Register-PmmcpdTask.ps1 -Start

# 3. Prove it
pmmcp doctor
```

`pmmcp install-service` writes artifacts under `%LOCALAPPDATA%\pmmcp\`:

| File | Purpose |
|------|---------|
| `pmmcpd-start.bat` | Starts `pmmcpd run` (the task's action) |
| `pmmcpd-logon-task.xml` | Generated copy of the task definition |
| `INSTALL.txt` | Elevation and registration notes |

## Files in this directory

| File | Purpose |
|------|---------|
| [`pmmcpd-logon-task.xml`](pmmcpd-logon-task.xml) | Task template: logon trigger, `LeastPrivilege`, no execution time limit, restart-on-failure (3× / 1 min), battery-safe |
| [`Register-PmmcpdTask.ps1`](Register-PmmcpdTask.ps1) | Registers the task (replaces an existing one); `-Start` launches immediately |
| [`Unregister-PmmcpdTask.ps1`](Unregister-PmmcpdTask.ps1) | Stops + unregisters the task; leaves state untouched |

Prefer plain `schtasks`? The template registers directly:

```bat
schtasks /Create /TN pmmcpd /XML pmmcpd-logon-task.xml
schtasks /Run /TN pmmcpd
```

Or build it in the Task Scheduler GUI: trigger **At log on**, action **Start a
program** → `%LOCALAPPDATA%\pmmcp\pmmcpd-start.bat`.

## Why not a Windows Service?

A system-wide Service requires Administrator to install and runs outside the
user's session — the wrong trust boundary for a **per-user** daemon whose
socket ACL, state directory, and audit identity are all user-scoped. The logon
task gives the same "starts at login, restarts on failure" behavior with zero
elevation. This is the same reasoning as user units on Linux and LaunchAgents
on macOS.

## Uninstall

```powershell
./Unregister-PmmcpdTask.ps1     # remove the task
pmmcp uninstall-service         # remove generated files under %LOCALAPPDATA%\pmmcp\
```

State (database, logs, secrets) is left untouched by both — see
[docs/operations.md](../../docs/operations.md) before deleting the state
directory shown by `pmmcp daemon-info`.
