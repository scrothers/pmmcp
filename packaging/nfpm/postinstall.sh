#!/bin/sh
# pmmcp package postinstall.
#
# Deliberately does NOT start anything: pmmcpd is a per-user daemon and
# "never auto-started" is a product invariant. We only tell the human how.
set -e

cat <<'EOF'
pmmcp installed.

Each user runs their own daemon (no system-wide service):

  systemctl --user enable --now pmmcpd.service   # start now + at every login
  pmmcp doctor                                   # verify the daemon answers

Headless server (no login session)? Enable lingering once:

  sudo loginctl enable-linger <user>

Strict sandboxing needs bubblewrap (bwrap) — install it via your package
manager or strict processes will fail closed rather than run unsandboxed.

Docs: https://github.com/scrothers/pmmcp/blob/main/docs/quickstart.md
EOF

exit 0
