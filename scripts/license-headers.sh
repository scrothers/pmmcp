#!/usr/bin/env bash
# Check or apply Apache-2.0 license headers on every first-party Go source file.
#
# Usage:
#   scripts/license-headers.sh check   # CI / pre-PR (default)
#   scripts/license-headers.sh fix     # add missing headers in place
#
# Skips generated code under internal/api/gen/. Pin the tool version with
# ADDLICENSE_VERSION (default v1.2.0) so local and CI stay aligned.
set -euo pipefail
cd "$(dirname "$0")/.."

ADDLICENSE_VERSION="${ADDLICENSE_VERSION:-v1.2.0}"
COPYRIGHT_HOLDER="${COPYRIGHT_HOLDER:-Steven Crothers}"
COPYRIGHT_YEAR="${COPYRIGHT_YEAR:-2026}"
MODE="${1:-check}"

case "$MODE" in
  check | fix) ;;
  -h | --help | help)
    sed -n '2,12p' "$0"
    exit 0
    ;;
  *)
    echo "usage: $0 check|fix" >&2
    exit 2
    ;;
esac

mapfile -t files < <(
  find . \
    \( -path './.git' -o -path './bin' -o -path './internal/api/gen' \) -prune \
    -o -name '*.go' -type f -print \
    | sed 's|^\./||' \
    | sort
)

if ((${#files[@]} == 0)); then
  echo "license-headers: no Go files found" >&2
  exit 1
fi

args=(
  -c "$COPYRIGHT_HOLDER"
  -l apache
  -y "$COPYRIGHT_YEAR"
)
if [[ "$MODE" == "check" ]]; then
  args+=(-check)
fi

echo "license-headers: $MODE (${#files[@]} Go files, addlicense@${ADDLICENSE_VERSION})"
go run "github.com/google/addlicense@${ADDLICENSE_VERSION}" "${args[@]}" "${files[@]}"
echo "license-headers: ok"
