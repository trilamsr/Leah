#!/usr/bin/env bash
# Verify each binary is codesigned, stapled, and accepted by Gatekeeper.
# Exits non-zero on the first failure so CI fails loudly.
# Usage: verify-signed.sh <bin1> [<bin2> ...]

set -euo pipefail

if [[ $# -lt 1 ]]; then
  echo "usage: $0 <binary> [<binary> ...]" >&2
  exit 2
fi

for bin in "$@"; do
  echo "==> $bin"
  codesign --verify --strict --verbose=2 "$bin"
  xcrun stapler validate "$bin"
  spctl --assess --type execute -vv "$bin"
done

echo "all binaries signed + notarized + stapled"
