#!/usr/bin/env bash
# Notarize + staple darwin binaries via Apple notarytool.
# Usage: notarize.sh <bin1> [<bin2> ...]
# Requires env: APPLE_ID, APPLE_PASSWORD, TEAM_ID.

set -euo pipefail

if [[ $# -lt 1 ]]; then
  echo "usage: $0 <binary> [<binary> ...]" >&2
  exit 2
fi

: "${APPLE_ID:?APPLE_ID env required}"
: "${APPLE_PASSWORD:?APPLE_PASSWORD env required}"
: "${TEAM_ID:?TEAM_ID env required}"

# notarytool requires a container; bundle all targets into one zip so a single
# submission covers every binary in this release.
ZIP="$(mktemp -t leah-notarize.XXXXXX).zip"
STAGE="$(mktemp -d -t leah-notarize-stage.XXXXXX)"
trap 'rm -rf "$ZIP" "$STAGE"' EXIT

# Stage every arg into one directory so ditto's --keepParent zips them together
# instead of capturing only dirname($1)'s siblings.
for bin in "$@"; do
  cp "$bin" "$STAGE/"
done
ditto -c -k --keepParent "$STAGE" "$ZIP"

echo "submitting $ZIP to notarytool..."
SUBMIT_LOG="$(mktemp -t leah-notarytool.XXXXXX)"
if ! xcrun notarytool submit "$ZIP" \
    --apple-id "$APPLE_ID" \
    --password "$APPLE_PASSWORD" \
    --team-id "$TEAM_ID" \
    --wait \
    --output-format plist > "$SUBMIT_LOG"; then
  echo "notarytool submit failed; log:" >&2
  cat "$SUBMIT_LOG" >&2
  exit 1
fi

# Extract id + status from the plist output; surface logs on any non-Accepted state.
SUBMISSION_ID="$(/usr/libexec/PlistBuddy -c 'Print :id' "$SUBMIT_LOG" 2>/dev/null || true)"
STATUS="$(/usr/libexec/PlistBuddy -c 'Print :status' "$SUBMIT_LOG" 2>/dev/null || true)"

if [[ "$STATUS" != "Accepted" ]]; then
  echo "notarization status: $STATUS (id=$SUBMISSION_ID)" >&2
  if [[ -n "$SUBMISSION_ID" ]]; then
    xcrun notarytool log "$SUBMISSION_ID" \
      --apple-id "$APPLE_ID" --password "$APPLE_PASSWORD" --team-id "$TEAM_ID" >&2 || true
  fi
  exit 1
fi

echo "notarization Accepted (id=$SUBMISSION_ID); stapling..."
for bin in "$@"; do
  xcrun stapler staple "$bin"
done
