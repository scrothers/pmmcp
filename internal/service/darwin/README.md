# darwin (service)

Installs a **LaunchAgent** so `pmmcpd` starts at user login on macOS and is kept alive if it exits.

## What Install does

Writes:

```
~/Library/LaunchAgents/com.scrothers.pmmcpd.plist
```

Contents include:

- **Label:** `com.scrothers.pmmcpd`
- **ProgramArguments:** `<pmmcpdPath> run`
- **RunAtLoad:** true (start at load/login)
- **KeepAlive:** true (restart if the process exits)

Install does **not** call `launchctl`. Loading is left to the operator (or a future CLI enhancement).

## Load after install

```bash
# usually via: pmmcp install-service (writes the plist)
launchctl bootstrap gui/"$(id -u)" ~/Library/LaunchAgents/com.scrothers.pmmcpd.plist
```

On older macOS toolchains, `launchctl load` against the same path may still work.

## Uninstall

Removes the plist if present (idempotent). Unload first if the agent is active:

```bash
launchctl bootout gui/"$(id -u)" ~/Library/LaunchAgents/com.scrothers.pmmcpd.plist
# then: pmmcp uninstall-service
```

Daemon state on disk is not deleted.

## API

```go
err:= darwin.Install(ctx, "/opt/homebrew/bin/pmmcpd")
path, err:= darwin.PlistPath
err = darwin.Uninstall(ctx)
```

## Static template

See [`packaging/launchd/com.scrothers.pmmcpd.plist`](../../../packaging/launchd/com.scrothers.pmmcpd.plist). The Go generator substitutes the real binary path.

## Design notes

- User-level only (LaunchAgent under the home directory), consistent with per-user tenancy and.
- Prefer an absolute `pmmcpd` path so login sessions do not depend on a custom `PATH`.

## Tests

```bash
go test./internal/service/darwin/...
```
