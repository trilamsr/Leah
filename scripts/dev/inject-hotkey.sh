#!/usr/bin/env bash
# Send a hotkey via System Events (requires Accessibility permission).
# Usage: inject-hotkey.sh [KEY]   KEY in {space, esc, return, tab}; default = space (⌥Space)
set -euo pipefail

KEY="${1:-space}"

case "$KEY" in
  space)  CODE=49;  MODS="{option down}" ;;
  esc)    CODE=53;  MODS="{}" ;;
  return) CODE=36;  MODS="{}" ;;
  tab)    CODE=48;  MODS="{}" ;;
  *)      echo "unknown key: $KEY (supported: space, esc, return, tab)" >&2; exit 1 ;;
esac

if [ "$MODS" = "{}" ]; then
  osascript -e "tell application \"System Events\" to key code $CODE"
else
  osascript -e "tell application \"System Events\" to key code $CODE using $MODS"
fi
