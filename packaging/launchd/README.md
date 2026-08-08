# launchd (macOS)

`pmmcpd` runs as a **per-user LaunchAgent** — one daemon per macOS user, in the
user's own launchd domain. Never a system-wide LaunchDaemon: the same-user
socket is the trust boundary ([docs/security.md](../../docs/security.md)).

## Recommended: let pmmcp generate it

```bash
pmmcp install-service    # writes ~/Library/LaunchAgents/com.scrothers.pmmcpd.plist
                         # with your actual binary + log paths substituted
launchctl bootstrap gui/$(id -u) ~/Library/LaunchAgents/com.scrothers.pmmcpd.plist
pmmcp doctor
```

## Manual: use the template in this directory

[`com.scrothers.pmmcpd.plist`](com.scrothers.pmmcpd.plist) assumes
`/usr/local/bin/pmmcpd`. Edit paths first — launchd does **not** expand `~` or
environment variables — then:

```bash
cp com.scrothers.pmmcpd.plist ~/Library/LaunchAgents/
launchctl bootstrap gui/$(id -u) ~/Library/LaunchAgents/com.scrothers.pmmcpd.plist
```

## Operate

```bash
launchctl print gui/$(id -u)/com.scrothers.pmmcpd   # status, PID, last exit
launchctl kickstart -k gui/$(id -u)/com.scrothers.pmmcpd   # restart now
launchctl bootout gui/$(id -u)/com.scrothers.pmmcpd        # stop + unload
```

`bootstrap`/`bootout` are the modern (10.11+) interface; the legacy
`launchctl load/unload` forms still work but report errors less usefully.

## Behavior encoded in the plist

| Key | Choice | Why |
|-----|--------|-----|
| `KeepAlive.SuccessfulExit=false` | restart **only on failure** | a deliberate stop stays stopped |
| `ThrottleInterval=5` | ≥5s between respawns | no tight crash-loops |
| `ExitTimeOut=30` | 30s before SIGKILL | matches the daemon's graceful drain (and the systemd unit) |
| `LimitLoadToSessionType=[Aqua, Background]` | GUI **and** SSH sessions | headless Macs get a daemon too |
| `SoftResourceLimits.NumberOfFiles=65536` | raised fd limit | log capture + many children |
