#!/usr/bin/env bash
# sign.sh — assemble + dual-sign a leah plugin bundle.
#
# Outputs <plugin>.leahplugin/ with Contents/{Info.plist,manifest.json,MacOS/<bin>,Resources/}
# and writes Contents/_CodeSignature plus Contents/manifest.json.sig (EdDSA).
# Apple Developer ID covers on-disk tamper; EdDSA covers the plugin-key channel verified
# by internal/attest.VerifyPlugin (§7.7).
#
# Usage:
#   scripts/plugin/sign.sh \
#       --src plugins/weather-pro \
#       --binary plugins/weather-pro/weather-pro \
#       --identity "Developer ID Application: Maydow (TEAMID)" \
#       --ed25519-key ~/.config/leah/plugin-signing.ed25519 \
#       --out dist/weather-pro.leahplugin
#
# All flags required. Exits 1 on missing tools or signing failure — caller (release pipeline)
# treats nonzero as ship-blocking, matching §7.7 unsigned-load refusal.

set -euo pipefail

SRC=""
BINARY=""
IDENTITY=""
ED25519_KEY=""
OUT=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --src)         SRC="$2"; shift 2 ;;
    --binary)      BINARY="$2"; shift 2 ;;
    --identity)    IDENTITY="$2"; shift 2 ;;
    --ed25519-key) ED25519_KEY="$2"; shift 2 ;;
    --out)         OUT="$2"; shift 2 ;;
    *) echo "unknown flag: $1" >&2; exit 1 ;;
  esac
done

for v in SRC BINARY IDENTITY ED25519_KEY OUT; do
  if [[ -z "${!v}" ]]; then
    echo "sign.sh: --${v,,} required" >&2
    exit 1
  fi
done

command -v codesign >/dev/null || { echo "codesign not found (Xcode CLT required)" >&2; exit 1; }
command -v openssl  >/dev/null || { echo "openssl not found"  >&2; exit 1; }

[[ -f "$SRC/manifest.json" ]] || { echo "missing $SRC/manifest.json" >&2; exit 1; }
[[ -f "$SRC/Info.plist"    ]] || { echo "missing $SRC/Info.plist"    >&2; exit 1; }
[[ -x "$BINARY"            ]] || { echo "missing or non-exec binary $BINARY" >&2; exit 1; }
[[ -f "$ED25519_KEY"       ]] || { echo "missing ed25519 key $ED25519_KEY" >&2; exit 1; }

rm -rf "$OUT"
mkdir -p "$OUT/Contents/MacOS" "$OUT/Contents/Resources"
cp "$SRC/Info.plist"    "$OUT/Contents/Info.plist"
cp "$SRC/manifest.json" "$OUT/Contents/manifest.json"
if [[ -d "$SRC/Resources" ]]; then
  cp -R "$SRC/Resources/." "$OUT/Contents/Resources/"
fi
cp "$BINARY" "$OUT/Contents/MacOS/$(basename "$BINARY")"
chmod +x "$OUT/Contents/MacOS/$(basename "$BINARY")"

# EdDSA channel: detached signature over manifest.json — internal/attest verifies this against
# the operator-trusted plugin-signing pubkey before allowing load.
openssl pkeyutl -sign -inkey "$ED25519_KEY" -rawin \
  -in "$OUT/Contents/manifest.json" \
  -out "$OUT/Contents/manifest.json.sig"

# Developer ID channel: covers binary + bundled assets via codesign's deep mode.
codesign --force --options runtime --timestamp --deep \
  --sign "$IDENTITY" "$OUT"

codesign --verify --deep --strict --verbose=2 "$OUT" >&2

echo "signed: $OUT"
