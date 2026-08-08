# Packaging

Everything needed to install `pmmcp`/`pmmcpd` outside of `go install` — OS service
definitions, Linux distro packages, and Windows task registration.

Two install paths exist, and they deliberately share behavior:

| Path | Binary location | Service definition | Who it's for |
|------|-----------------|--------------------|--------------|
| **`pmmcp install-service`** | wherever you put the binaries (e.g. `~/.local/bin`) | generated at runtime with the resolved path | building from source / `go install` |
| **Distro packages** (`.deb` / `.rpm` / `.apk` / Arch) | `/usr/bin` | shipped in the package (`/usr/lib/systemd/user/pmmcpd.service`) | `apt` / `dnf` / `apk` / `pacman` users |

Whichever path you take, **the daemon is never auto-started** — installing writes
definitions; *you* run the enable command. That is a [product
invariant](../README.md#%EF%B8%8F-the-five-things-that-never-compromise), not an oversight.

## Directory map

| Path | Contents |
|------|----------|
| [`systemd/`](systemd/) | User unit shipped in distro packages + headless-server (linger) notes |
| [`launchd/`](launchd/) | macOS LaunchAgent plist template + `launchctl` bootstrap guide |
| [`windows/`](windows/) | Logon Scheduled Task XML + PowerShell register/unregister scripts |
| [`nfpm/`](nfpm/) | Scriptlets for the `.deb`/`.rpm`/`.apk`/Arch packages built by GoReleaser |

Linux packages are built by [GoReleaser's nfpm integration](../.goreleaser.yaml)
on every `v*` tag and attached to the GitHub Release next to the tar/zip archives.

## Verifying what you download

Every release artifact — archives, packages, and `checksums.txt` — carries
[GitHub build provenance](https://docs.github.com/en/actions/security-for-github-actions/using-artifact-attestations)
(SLSA). Verify before installing:

```bash
# provenance: proves this exact file was built by this repo's release workflow
gh attestation verify pmmcp_1.0.0_linux_amd64.tar.gz --repo scrothers/pmmcp

# integrity: cross-check against the signed checksum manifest
sha256sum --check --ignore-missing checksums.txt
```

## Not here (yet)

Homebrew tap, AUR, Scoop, and winget are candidates once release cadence
stabilizes — each needs an external repo or manifest pipeline. If you package
pmmcp for a distro, open an issue; we'll link it and keep you in the loop on
breaking changes.
