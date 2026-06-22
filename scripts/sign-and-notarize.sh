#!/usr/bin/env bash
# Code-sign, notarize, and staple Leah.app for distribution.
#
# Env vars (required for --sign/--notarize):
#   LEAH_IDENTITY              Full codesign identity (e.g. "Developer ID Application: Maydow LLC (ABC123XYZ)")
#                              Find via: security find-identity -v -p codesigning
#   LEAH_TEAM_ID               Apple Developer Team ID (10-char alphanumeric)
#   LEAH_APPLE_ID              Apple ID email for notarization
#   LEAH_APP_SPECIFIC_PASSWORD App-specific password for notarytool
#   LEAH_KEYCHAIN_PROFILE      notarytool credentials profile name
#                              (set up once via: xcrun notarytool store-credentials)
#
# Usage: sign-and-notarize.sh [--build-only | --sign | --notarize | --staple | --all]
#        --build-only           swift build -c release only; no signing
#        --sign                 codesign the .app with Hardened Runtime
#        --notarize             submit to Apple notary service and wait
#        --staple               staple the notarization ticket to the .app
#        --all                  build + sign + notarize + staple in sequence
#
# Invoked by: make sign-and-notarize ARGS=--all
set -euo pipefail

APP_DIR="app/Leah"
APP_PATH="${APP_PATH:-$APP_DIR/.build/release/Leah.app}"
ENTITLEMENTS="${ENTITLEMENTS:-$APP_DIR/Sources/LeahApp/Leah.entitlements}"

usage() {
  echo "usage: sign-and-notarize.sh [--build-only | --sign | --notarize | --staple | --all]"
  echo ""
  echo "  --build-only  swift build -c release (no signing)"
  echo "  --sign        codesign with Hardened Runtime (requires LEAH_IDENTITY or LEAH_TEAM_ID)"
  echo "  --notarize    submit to Apple notary and wait (requires LEAH_KEYCHAIN_PROFILE)"
  echo "  --staple      staple notarization ticket to the .app"
  echo "  --all         build + sign + notarize + staple"
  echo ""
  echo "Env vars for --sign/--notarize:"
  echo "  LEAH_IDENTITY              Full codesign identity (e.g. \"Developer ID Application: Maydow LLC (ABC123XYZ)\")"
  echo "                             Find via: security find-identity -v -p codesigning"
  echo "  LEAH_TEAM_ID               Developer Team ID (used only when LEAH_IDENTITY is unset)"
  echo "  LEAH_APPLE_ID              Apple ID email"
  echo "  LEAH_APP_SPECIFIC_PASSWORD App-specific password"
  echo "  LEAH_KEYCHAIN_PROFILE      notarytool credentials profile name"
}

require_env() {
  local missing=()
  for var in LEAH_TEAM_ID LEAH_APPLE_ID LEAH_APP_SPECIFIC_PASSWORD; do
    if [ -z "${!var:-}" ]; then
      missing+=("$var")
    fi
  done
  if [ "${#missing[@]}" -gt 0 ]; then
    echo "error: missing required env vars: ${missing[*]}" >&2
    echo "       set LEAH_TEAM_ID, LEAH_APPLE_ID, LEAH_APP_SPECIFIC_PASSWORD before signing" >&2
    exit 1
  fi
}

require_keychain_profile() {
  if [ -z "${LEAH_KEYCHAIN_PROFILE:-}" ]; then
    echo "error: LEAH_KEYCHAIN_PROFILE must be set for --notarize" >&2
    echo "       create it once: xcrun notarytool store-credentials <profile-name>" >&2
    exit 1
  fi
}

cmd_build() {
  echo "==> swift build -c release"
  cd "$APP_DIR" && swift build -c release
}

cmd_sign() {
  require_env
  local identity="${LEAH_IDENTITY:-Developer ID Application: ${LEAH_TEAM_ID}}"
  echo "==> codesign $APP_PATH"
  codesign --force --options runtime --timestamp \
    --entitlements "$ENTITLEMENTS" \
    --sign "$identity" \
    "$APP_PATH"
  echo "signed: $APP_PATH"
}

cmd_notarize() {
  require_keychain_profile
  local zip="${APP_PATH%.app}.zip"
  echo "==> ditto $APP_PATH -> $zip"
  ditto -c -k --keepParent "$APP_PATH" "$zip"
  echo "==> xcrun notarytool submit $zip"
  xcrun notarytool submit "$zip" \
    --keychain-profile "$LEAH_KEYCHAIN_PROFILE" \
    --wait
}

cmd_staple() {
  echo "==> xcrun stapler staple $APP_PATH"
  xcrun stapler staple "$APP_PATH"
  echo "stapled: $APP_PATH"
}

if [ $# -eq 0 ]; then
  usage
  exit 0
fi

case "$1" in
  --build-only)
    cmd_build
    ;;
  --sign)
    cmd_sign
    ;;
  --notarize)
    cmd_notarize
    ;;
  --staple)
    cmd_staple
    ;;
  --all)
    cmd_build
    cmd_sign
    cmd_notarize
    cmd_staple
    ;;
  *)
    echo "error: unknown option: $1" >&2
    usage >&2
    exit 1
    ;;
esac
