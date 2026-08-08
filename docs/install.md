# Installation

This guide gets you from a clone to a running daemon you can talk to. It assumes you've read [Concepts](concepts.md) — in particular, that `pmmcpd` (the daemon) and `pmmcp` (the client) are two separate binaries, and that **the daemon is never started for you.**

By the end you'll have both binaries on your `PATH`, the daemon running as a user service, and a green `pmmcp doctor`.

---

## Prerequisites

- **Go 1.25 or newer** to build from source (the module targets `go 1.25`).
- **Linux, macOS, or Windows.** All three are first-class. On Linux, strict sandboxing uses [`bubblewrap`](https://github.com/containers/bubblewrap) (`bwrap`) — install it (`dnf install bubblewrap`, `apt install bubblewrap`, …) if you want strict isolation for host processes. Without it, strict processes **fail to start** rather than run unsandboxed (that's the point — see [Security](security.md)).
- **Optional:** [Podman](https://podman.io) (preferred) or Docker, only if you'll run container-runtime processes (sidecars). Not needed for host processes.

---

## Build the binaries

```bash
git clone https://github.com/scrothers/pmmcp
cd pmmcp

go build -o bin/pmmcp  ./cmd/pmmcp
go build -o bin/pmmcpd ./cmd/pmmcpd
```

Put them somewhere on your `PATH`:

```bash
install -m 0755 bin/pmmcp bin/pmmcpd ~/.local/bin/   # or /usr/local/bin as root
```

Confirm:

```bash
pmmcp version      # -> pmmcp <version>
pmmcpd version     # -> pmmcpd <version>
```

> Keep the two binaries next to each other. `pmmcp install-service` locates the daemon by looking for `pmmcpd` beside the `pmmcp` executable first (then on `PATH`), so a matched pair installs cleanly.

---

## Run the daemon

You have two options: run it in the foreground (good for a first look), or install it as a user service (what you want long-term).

### Option A — foreground (to try it)

```bash
pmmcpd run
# -> pmmcpd listening on <endpoint> (state <dir>)
```

`pmmcpd run` runs until you stop it with Ctrl-C (`SIGINT`) or `SIGTERM`, which trigger a graceful shutdown. In a second terminal, run the client. This is the simplest way to see pmmcp work; for anything lasting, use the service.

> `pmmcpd` takes no config flags. It reads its configuration from the standard search path, or from `PMMCP_CONFIG` if set. See [Configuration](configuration.md).

### Option B — install as a user service (recommended)

```bash
pmmcp install-service
```

This **writes a service definition and nothing else** — it does not start, enable, or elevate anything. It runs entirely as your user, no root required. What it writes, per platform:

| OS | File written | Then start it with |
|----|--------------|--------------------|
| **Linux** | `~/.config/systemd/user/pmmcpd.service` (`ExecStart=<pmmcpd> run`, `Restart=on-failure`) | `systemctl --user enable --now pmmcpd.service` |
| **macOS** | `~/Library/LaunchAgents/com.scrothers.pmmcpd.plist` (`RunAtLoad`, `KeepAlive`) | `launchctl load ~/Library/LaunchAgents/com.scrothers.pmmcpd.plist` |
| **Windows** | `%LOCALAPPDATA%\pmmcp\pmmcpd-start.bat`, `pmmcpd-logon-task.xml`, `INSTALL.txt` | `schtasks /Create /TN pmmcpd /XML "%LOCALAPPDATA%\pmmcp\pmmcpd-logon-task.xml"` (or import the XML in Task Scheduler) |

The final "start it" step is deliberately yours to run. pmmcp writes the definition so you can review it; **you** decide to launch a long-lived daemon. `install-service` prints the exact enable command for your platform.

Point it at a specific daemon binary with `--bin`:

```bash
pmmcp install-service --bin /usr/local/bin/pmmcpd
```

On Linux, if you want the daemon to survive logout (not just your login session), enable lingering once:

```bash
loginctl enable-linger "$USER"
```

---

## Confirm it's working

```bash
pmmcp doctor
```

`doctor` probes the daemon **without dialing it as a normal client** and prints a report: the resolved config, the IPC endpoint, and whether the daemon answered a version handshake. If the daemon is up and compatible, `doctor` exits `0`. If it's down, `doctor` still prints the report (so you can see *where* it's looking) and then exits `3` (`daemon_unavailable`) with a hint to start it.

A healthy first run looks like:

```bash
pmmcp doctor          # exit 0, report shows the daemon answering
pmmcp whoami          # your OS user, role, capabilities, session
pmmcp daemon-info     # daemon version, uptime, paths
```

If `doctor` reports the daemon down, see [Operations → The daemon is not running](operations.md#the-daemon-is-not-running).

---

## Uninstall

Remove the service definition (this does not stop a running daemon — stop it first with your platform's tool):

```bash
# stop first, e.g. on Linux:
systemctl --user disable --now pmmcpd.service

pmmcp uninstall-service
```

`uninstall-service` removes the definition file(s) it created. Your state directory — the database, logs, and any secrets — is left untouched. To remove that too, delete the state directory shown by `pmmcp daemon-info` (see [Operations → Backup & upgrade](operations.md#backup-and-upgrade) first).

---

## Next

- **Run your first process** → [Quickstart](quickstart.md)
- **Tune the daemon** → [Configuration](configuration.md)
- **Wire it to an agent** → [Agent & MCP integration](mcp.md) · [Harness guides](integration/README.md)
