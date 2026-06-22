#!/usr/bin/env bash
# Sign a notarized .zip, publish a GitHub release, and emit the appcast <item>.
#
# Usage: publish-release.sh VERSION ZIPPATH
#   VERSION   SemVer string, e.g. 1.0.0
#   ZIPPATH   Absolute path to the notarized Leah-<VERSION>.zip
#
# Stdout emits the <item> XML fragment — append it inside <channel> in
# docs/appcast.xml on the gh-pages branch, then push.
set -euo pipefail

VERSION="${1:?usage: publish-release.sh VERSION ZIPPATH}"
ZIP="${2:?missing ZIPPATH}"

find_tool() {
    local name="$1"
    local spm
    spm="$(find "$(git rev-parse --show-toplevel)/app/Leah/.build" -name "$name" -type f -perm -u+x 2>/dev/null | head -1)"
    if [ -n "$spm" ]; then echo "$spm"; return; fi
    local xcode
    xcode="$(find "${HOME}/Library/Developer/Xcode/DerivedData" -name "$name" -type f -perm -u+x 2>/dev/null | head -1)"
    if [ -n "$xcode" ]; then echo "$xcode"; return; fi
    echo ""
}

SIGN_UPDATE="$(find_tool sign_update)"
if [ -z "$SIGN_UPDATE" ]; then
    echo "sign_update not found. Build the app first: cd app/Leah && swift build" >&2
    exit 1
fi

# sign_update emits: sparkle:edSignature="..." length="..."
SIG_LINE="$("$SIGN_UPDATE" "$ZIP")"
LENGTH=$(stat -f%z "$ZIP")
DL_URL="https://github.com/maydow/leah/releases/download/v${VERSION}/$(basename "$ZIP")"

gh release create "v${VERSION}" "$ZIP" \
    --title "Leah v${VERSION}" \
    --notes "Release v${VERSION}"

cat <<EOF
<item>
  <title>Leah ${VERSION}</title>
  <pubDate>$(date -u +"%a, %d %b %Y %H:%M:%S +0000")</pubDate>
  <enclosure
    url="${DL_URL}"
    sparkle:version="${VERSION}"
    length="${LENGTH}"
    type="application/octet-stream"
    ${SIG_LINE} />
</item>
EOF
