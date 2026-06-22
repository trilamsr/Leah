#!/usr/bin/env bash
# Capture a screenshot to PATH (default /tmp/leah-screenshot.png).
# Usage: screenshot.sh [PATH]
# Pass --window WINDOWID to capture a specific window.
set -euo pipefail

OUT="${1:-/tmp/leah-screenshot.png}"
WINDOW_ID=""

while [ $# -gt 0 ]; do
  case "$1" in
    --window) WINDOW_ID="$2"; shift 2 ;;
    *)        OUT="$1"; shift ;;
  esac
done

if [ -n "$WINDOW_ID" ]; then
  screencapture -x -o -l "$WINDOW_ID" "$OUT"
else
  screencapture -x -o "$OUT"
fi

echo "saved: $OUT"
sips -g pixelWidth -g pixelHeight "$OUT" 2>/dev/null || true
