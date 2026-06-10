#!/usr/bin/env bash
# Import a base64-encoded Developer ID .p12 into a one-shot keychain for this CI run.
# Requires env: CERT_P12_BASE64, CERT_PASSWORD.
# The keychain is deleted by the GHA runner teardown; nothing persists across runs.

set -euo pipefail

: "${CERT_P12_BASE64:?CERT_P12_BASE64 env required}"
: "${CERT_PASSWORD:?CERT_PASSWORD env required}"

KEYCHAIN="leah-release.keychain-db"
KEYCHAIN_PASSWORD="$(uuidgen)"
P12_PATH="$(mktemp -t leah-cert.XXXXXX).p12"
trap 'rm -f "$P12_PATH"' EXIT

echo "::add-mask::$KEYCHAIN_PASSWORD"

base64 --decode <<<"$CERT_P12_BASE64" > "$P12_PATH"

security create-keychain -p "$KEYCHAIN_PASSWORD" "$KEYCHAIN"
security set-keychain-settings -lut 21600 "$KEYCHAIN"
security unlock-keychain -p "$KEYCHAIN_PASSWORD" "$KEYCHAIN"

security import "$P12_PATH" \
  -k "$KEYCHAIN" \
  -P "$CERT_PASSWORD" \
  -T /usr/bin/codesign \
  -T /usr/bin/security

# Allow codesign to use the imported key without a UI prompt.
security set-key-partition-list \
  -S apple-tool:,apple:,codesign: \
  -s -k "$KEYCHAIN_PASSWORD" "$KEYCHAIN" >/dev/null

# Put the new keychain first in the search list so codesign finds the cert.
# shellcheck disable=SC2046 # word-splitting is intentional: each keychain path is a distinct arg.
security list-keychains -d user -s "$KEYCHAIN" $(security list-keychains -d user | tr -d '"')
