#!/usr/bin/env bash
# Type text into the focused window via System Events (requires Accessibility permission).
# Usage: inject-text.sh "text to type"
set -euo pipefail

TEXT="${1:-}"
if [ -z "$TEXT" ]; then
  echo "usage: inject-text.sh \"text to type\"" >&2
  exit 1
fi

# Use keystroke for arbitrary strings; single-quotes inside TEXT are escaped.
ESCAPED="${TEXT//\'/\'}"
osascript -e "tell application \"System Events\" to keystroke \"$ESCAPED\""
