# systemd (Linux)

`pmmcpd` runs as a **user** service — one daemon per OS user, no system-wide
control plane. That is the trust model, not a limitation
([Security](https://github.com/scrothers/pmmcp/wiki/Security)).

## Desktop / interactive use

If you installed a distro package (`.deb`/`.rpm`/`.apk`/Arch), the unit is
already at `/usr/lib/systemd/user/pmmcpd.service`:

```bash
systemctl --user enable --now pmmcpd.service
pmmcp doctor
```

If you built from source, generate a unit that points at *your* binary instead:

```bash
pmmcp install-service        # writes ~/.config/systemd/user/pmmcpd.service
systemctl --user enable --now pmmcpd.service
```

## Headless servers (no login session)

User managers normally start at login and stop at logout. For a daemon that
must run on a server without anyone logged in, enable **lingering** once:

```bash
sudo loginctl enable-linger <user>
```

That starts `<user>`'s systemd user manager at boot (with `XDG_RUNTIME_DIR`
provisioned, which the daemon's socket path depends on) and keeps it running
with no session. Then enable the unit as that user:

```bash
systemctl --user enable --now pmmcpd.service
```

> **Why not a system unit with `User=`?** A `pmmcpd@.service` system template
> would run the daemon outside a user manager, where `/run/user/<uid>` — home
> of the control socket — is not guaranteed to exist or survive. Lingering is
> the systemd-sanctioned way to get exactly the same result without those
> sharp edges.

## Useful commands

```bash
systemctl --user status pmmcpd     # unit + recent log lines
journalctl --user -u pmmcpd -f     # follow daemon logs
systemctl --user reload pmmcpd     # re-apply safe config subset (pmmcp reload)
systemctl --user disable --now pmmcpd
```

## Hardening note

The unit intentionally omits `NoNewPrivileges=`, `ProtectHome=`, and similar
directives. pmmcpd **is** the sandboxing layer: it confines children with
bubblewrap (user namespaces) and must reach the project trees it supervises.
Directives that break namespace creation would disable real child isolation to
gain cosmetic daemon isolation. The daemon's own exposure is bounded by the
same-user socket trust boundary instead.
