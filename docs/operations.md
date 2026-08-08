# Operations runbook

Running pmmcp day to day: keeping the daemon healthy, reloading and restarting it, upgrading without losing state, backing up, running it for more than one user, and fixing the handful of things that actually go wrong. When a command doesn't do what you expected, start here.

---

## The daemon is not running

The single most common issue, because pmmcp **never** starts the daemon for you. Symptom: any command exits `3` with

```
pmmcp: daemon_unavailable: dial daemon: <detail>
```

or, over MCP, every tool returns a retryable `daemon_unavailable` result.

**Diagnose:**

```bash
pmmcp doctor
```

`doctor` prints where it's looking for the socket and whether the daemon answered — without dialing as a normal client, so it works as a first check even when the socket path is wrong.

**Fix, in order of likelihood:**

1. **It's genuinely not started.** Start it: `pmmcpd run` (foreground) or your service (`systemctl --user start pmmcpd.service`, `launchctl load …`, or your Windows task). See [Installation](install.md).
2. **The client is looking at the wrong socket.** If you overrode `ipc.endpoint` or `state_dir` for the daemon, the client must use the same values. Set `PMMCP_IPC_ENDPOINT` / `PMMCP_STATE_DIR` in the client's environment (and, for an agent, in the MCP server's `env` block). See [Configuration → Environment overrides](configuration.md#environment-overrides).
3. **The daemon crashed on startup.** Check its own log/journal (`journalctl --user -u pmmcpd.service` on Linux). A bad `daemon.toml` — an unknown key, `version` ≠ 1 — fails the daemon at load, loudly. Fix the config and restart.
4. **A different OS user's daemon.** The socket is per-user. A client run as `alice` can't reach `bob`'s daemon (by design). See [Multiple OS users](#multiple-os-users).

---

## Reloading and restarting

Two levers, for two kinds of change:

```bash
pmmcp reload      # re-apply the safe subset of config without a restart
```

`reload` (also `pm_daemon_reload`) re-reads `daemon.toml` and applies the settings that are safe to change live — log level, and similar. It does **not** move the socket or the state directory.

**Structural changes require a full restart:** the IPC endpoint, the state directory, anything that changes where the daemon lives. Restart the service (`systemctl --user restart pmmcpd.service`) or Ctrl-C and re-run `pmmcpd run`. On restart, [boot relaunch](supervision.md#boot-relaunch) brings back your durable (`enable`d) processes, so a daemon restart is not a stack outage — the supervised processes come back to their desired state.

`pmmcpd` shuts down gracefully on `SIGINT`/`SIGTERM`: it stops accepting connections and cleans up before exiting.

---

## Version mismatch

Symptom: exit `8`, `ipc_version_mismatch`. The client and daemon speak an incompatible IPC version — you upgraded one binary but not the other, or you have two builds on your `PATH`.

The client and daemon negotiate a version on connect and **fail closed** on an incompatible pair rather than talking past each other. The fix is always: **rebuild both `pmmcp` and `pmmcpd` from the same source and install them together.** Keep the pair matched (they're meant to be installed side by side — see [Installation](install.md)).

---

## Backup and upgrade

Everything durable lives in the **state directory** (`pmmcp daemon-info` prints its path): the SQLite database (`pmmcp.db` — processes, desired state, groups, profiles, events, audit), the `keyring`, and the `logs/` tree. Back that directory up and you've backed up pmmcp.

### Backing up

```bash
STATE=$(pmmcp daemon-info | ...)          # the state dir from daemon-info
# stop the daemon for a consistent copy of the DB, then:
cp -a "$STATE" "$STATE.backup-$(date +%F)"
```

Copy with the daemon stopped for a clean, consistent database snapshot. The keyring is `0600`; preserve permissions (`cp -a`).

### Upgrading

1. Build the new `pmmcp` and `pmmcpd` from the same source.
2. Stop the daemon (`systemctl --user stop pmmcpd.service`).
3. (Recommended) back up the state directory as above.
4. Replace both binaries.
5. Start the daemon.

On start, the daemon runs any needed **schema migrations** against its database automatically; a failed migration leaves the prior state intact and refuses to start rather than half-upgrading. Then boot relaunch restores your durable processes. Because both binaries are matched, there's no version-mismatch window — just don't run a new client against an old daemon (or vice-versa) in between.

### Removing everything

Uninstall the service (`pmmcp uninstall-service`), then delete the state directory. That erases processes, logs, events, audit, and stored secrets — irreversible, so back up first if you might want any of it.

---

## Multiple OS users

pmmcp is single-user by design: one daemon per OS user, each on its own private socket. To run it for several people on one machine, each person runs their own daemon under their own account — there is no shared instance.

The rules:

1. **One daemon per user.** Each user installs and enables the service as themselves (`pmmcp install-service`, then enable per platform).
2. **Never share a socket or state directory across users.** The socket is `0600` in a `0700` directory for a reason; do not `chmod` it group- or world-accessible.
3. **A client dials its own user's daemon.** Cross-user connections are refused by peer-credential checks — that's the [trust boundary](security.md#trust-model), not a bug to work around.
4. On Linux, if a user's daemon should survive logout, enable lingering once: `loginctl enable-linger <user>`.

| User | Socket | State |
|------|--------|-------|
| `alice` | `$XDG_RUNTIME_DIR/pmmcp/pmmcpd.sock` (as alice) | `~alice/.local/state/pmmcp` |
| `bob` | bob's runtime dir | `~bob/.local/state/pmmcp` |

---

## After an incident

When something misbehaved, the three streams reconstruct it (see [Logs & events](logs-and-events.md)):

```bash
pmmcp status  web     # where is it now — failed? crashed? exit code?
pmmcp errors  web     # why: the error lines from its output
pmmcp events  web     # what happened: crash, backoff, restart sequence
pmmcp audit   action=process.stop   # who: did someone stop it, or did it fall over?
```

The audit trail is the one that answers "did the agent do this?" — every mutation and every denial is attributable to a session.

---

## Troubleshooting quick reference

| Symptom | Likely cause | Fix |
|---------|--------------|-----|
| `daemon_unavailable` (exit 3) | daemon down, or client/daemon path mismatch | Start `pmmcpd`; align `PMMCP_IPC_ENDPOINT`/`PMMCP_STATE_DIR`. [Above](#the-daemon-is-not-running) |
| `ipc_version_mismatch` (exit 8) | mismatched binaries | Rebuild + install both from the same source |
| `sandbox_failed` (exit 7) on start | strict sandbox couldn't be applied (e.g. no `bwrap`) | Install the mechanism, or pick a supported profile. [Security](security.md#the-sandbox) |
| `name_conflict` (exit 6) | name already used in this project/profile | Different name, or remove/replace the old process |
| `not_found` (exit 4) | wrong name, or wrong project (cwd) | `pmmcp list --all`; check your working directory |
| `permission_denied` (exit 5) | missing capability (e.g. sandbox relax) or another session's process | [Security → Capabilities](security.md#capabilities-and-roles); use `pmmcp share` |
| Daemon won't start | bad `daemon.toml` (unknown key, `version`≠1) | Fix config; it fails loudly on the offending key |
| Process starts then dies immediately | crash on startup; check backoff | `pmmcp errors <name>`, `pmmcp events <name>` |
| Config change had no effect | needs a restart, not a reload | Restart the daemon for structural changes |

Every code above is explained in the [error reference](reference-errors.md).

---

## See also

- [Installation](install.md) — starting the daemon as a service
- [Configuration](configuration.md) — paths, the socket, and what `reload` covers
- [Security](security.md) — the trust boundary behind the multi-user rules
- [Logs, events & observability](logs-and-events.md) — the incident-reconstruction toolkit
