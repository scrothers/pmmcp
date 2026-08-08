# service

Installs and uninstalls the **user-level** `pmmcpd` service definition for the current OS.

```go
err:= service.Install(ctx, "/path/to/pmmcpd")
err = service.Uninstall(ctx)
```

Install only **writes** unit/plist/task artifacts. It does not start the daemon or enable the unit. Uninstall removes those definitions (and does not wipe process state).

## CLI

```bash
pmmcp install-service
pmmcp uninstall-service
```

The client looks for `pmmcpd` next to the `pmmcp` binary when possible.

## Per platform

### Linux — systemd --user

Writes `~/.config/systemd/user/pmmcpd.service` with `ExecStart=<pmmcpd> run`.

```bash
systemctl --user daemon-reload
systemctl --user enable --now pmmcpd.service
# Optional on headless hosts:
loginctl enable-linger "$USER"
```

### macOS — LaunchAgent

Writes `~/Library/LaunchAgents/com.scrothers.pmmcpd.plist` with `RunAtLoad` and `KeepAlive`. Load with `launchctl` after install if desired.

### Windows — logon artifacts

Writes under `%LOCALAPPDATA%\pmmcp\`:

| File | Purpose |
|------|---------|
| `pmmcpd-start.bat` | Starts `pmmcpd run` |
| `pmmcpd-logon-task.xml` | Sample Task Scheduler definition (logon trigger) |
| `INSTALL.txt` | Manual registration notes |

Preferred path is a **per-user logon Scheduled Task** (no admin). A Windows Service under `services.msc` typically needs elevation and is not the default.

## Design notes

-: user-scoped durability across login/reboot; not a root system daemon by default.
- Packaging templates under `packaging/` mirror these units for distro/docs use.
- After install, `pmmcp doctor` can confirm the daemon is reachable.
