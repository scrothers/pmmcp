# linux (service)

Installs a **systemd user unit** so `pmmcpd` can run at login without leaving a terminal open.

## What Install does

Writes:

```
~/.config/systemd/user/pmmcpd.service
```

with `ExecStart=<pmmcpdPath> run`, `Restart=on-failure`, and `WantedBy=default.target`.

It does **not** run `systemctl`, enable the unit, or start the daemon. That keeps install pure, testable, and free of ambient side effects.

## Enable after install

```bash
# usually via: pmmcp install-service
systemctl --user daemon-reload
systemctl --user enable --now pmmcpd.service
```

### Lingering (servers / headless)

User units normally start at **graphical/login** session. For boot without an interactive login:

```bash
loginctl enable-linger "$USER"
```

`Install` never runs this; it is an operator choice.

## Uninstall

Removes `pmmcpd.service` from the user unit directory if present (idempotent). Stop/disable the unit yourself if it is still active:

```bash
systemctl --user disable --now pmmcpd.service
# then: pmmcp uninstall-service
```

State under the daemon `StateDir` is left alone.

## API

```go
err:= linux.Install(ctx, "/usr/local/bin/pmmcpd")
path, err:= linux.UnitPath
err = linux.Uninstall(ctx)
```

## Static template

A checked-in example lives at [`packaging/systemd/pmmcpd.service`](../../../packaging/systemd/pmmcpd.service). Runtime install uses the generated body in `linux.go` with the resolved binary path.

## Tests

```bash
go test./internal/service/linux/...
```

Tests set `HOME` to a temp directory and assert unit contents.
