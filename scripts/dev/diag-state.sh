#!/usr/bin/env bash
set -euo pipefail
# Sends a diag.state IPC frame to leah-daemon and pretty-prints the JSON response.
SOCK="${LEAH_SOCK:-$HOME/Library/Caches/Leah/leah.sock}"
[ ! -S "$SOCK" ] && { echo "daemon socket not found at $SOCK"; exit 1; }
go run "$(dirname "$0")/../smoke/diag-state.go" "$SOCK" | jq .
