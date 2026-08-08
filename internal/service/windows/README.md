# windows (service)

Installs **per-user startup artifacts** for `pmmcpd` on Windows. The preferred path is a **logon Scheduled Task** owned by the current user — no Administrator elevation.

## What Install writes

Under `%LOCALAPPDATA%\pmmcp\` (or `HOME\AppData\Local\pmmcp` if `LOCALAPPDATA` is unset):

| File | Purpose |
|------|---------|
| `pmmcpd-start.bat` | Starts `"<pmmcpdPath>" run` |
| `pmmcpd-logon-task.xml` | Sample Task Scheduler definition (logon trigger → the bat) |
| `INSTALL.txt` | Human steps + elevation notes |

Install does **not** register the task or create a Windows Service.

## Recommended registration (no elevation)

```bat
schtasks /Create /TN pmmcpd /XML "%LOCALAPPDATA%\pmmcp\pmmcpd-logon-task.xml"
```

Or Task Scheduler GUI: trigger **At log on** (your user), action **Start a program** → `pmmcpd-start.bat`.

## Elevation

A system-wide Windows Service typically **requires Administrator**. That is intentionally not the default pmmcp install path).

## Uninstall

Removes the three artifact files and best-effort deletes the empty directory. If you registered a task, delete it separately:

```bat
schtasks /Delete /TN pmmcpd /F
```

## API

```go
err:= windows.Install(ctx, `C:\Tools\pmmcpd.exe`)
dir, err:= windows.InstallDir
err = windows.Uninstall(ctx)
```

## How it is invoked

`pmmcp install-service` → `service.Install` → `windows.Install` when `GOOS=windows`. The CLI resolves a sibling `pmmcpd` binary when possible.

## Tests

```bash
go test./internal/service/windows/...
```

Tests override `LOCALAPPDATA` to a temp directory and assert bat/XML/readme contents.
