#!/usr/bin/env bash
# One-time EdDSA keypair generation for Sparkle automatic updates.
#
# WHY two search paths: SPM builds drop the tool under .build/; Xcode
# DerivedData is the Xcode-native path. We try both so the script works
# regardless of which build system was used.
#
# After running:
#   1. Embed the printed SUPublicEDKey value in app/Leah/Sources/LeahApp/Info.plist.
#   2. Back up the private key to all three custody locations per docs/runbooks/sparkle-key-custody.md.
set -euo pipefail

find_tool() {
    local name="$1"
    # SPM .build path
    local spm
    spm="$(find "$(git rev-parse --show-toplevel)/app/Leah/.build" -name "$name" -type f -perm -u+x 2>/dev/null | head -1)"
    if [ -n "$spm" ]; then echo "$spm"; return; fi
    # Xcode DerivedData fallback
    local xcode
    xcode="$(find "${HOME}/Library/Developer/Xcode/DerivedData" -name "$name" -type f -perm -u+x 2>/dev/null | head -1)"
    if [ -n "$xcode" ]; then echo "$xcode"; return; fi
    echo ""
}

GEN_KEYS="$(find_tool generate_keys)"
if [ -z "$GEN_KEYS" ]; then
    cat >&2 <<'EOF'
generate_keys not found.
Build the app first: cd app/Leah && swift build
Or download Sparkle from https://github.com/sparkle-project/Sparkle/releases and set SPARKLE_DIR.
EOF
    exit 1
fi

"$GEN_KEYS"

echo
echo "---"
echo "Next steps:"
echo "  1. Copy the SUPublicEDKey value above into app/Leah/Sources/LeahApp/Info.plist."
echo "  2. Back up the private key per docs/runbooks/sparkle-key-custody.md:"
echo "     - 1Password vault item (Leah EdDSA private key)"
echo "     - age-encrypted file on Time Machine volume"
echo "     - BIP39 mnemonic paper printout stored in a safe"
