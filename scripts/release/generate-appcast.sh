#!/usr/bin/env bash
# generate-appcast.sh — emit a Sparkle appcast.xml on stdout for every signed
# .zip in a release directory. Each .zip MUST have a sibling `<zip>.sig` file
# whose contents are the literal `sparkle:edSignature="..." length="..."`
# fragment that `sign_update` prints; this script does not invoke sign_update
# itself so it stays runnable on machines without the EdDSA private key (e.g.
# CI smoke tests, the reviewer's worktree).
#
# Usage:
#   generate-appcast.sh RELEASE_DIR VERSION BASE_URL > appcast.xml
#   generate-appcast.sh --help
#
# Env:
#   LEAH_APPCAST_CHANNEL  Default "stable". Set to "rollback" when promoting
#                         a previous build as the rollback channel item.

set -euo pipefail

usage() {
  cat <<'USAGE'
usage: generate-appcast.sh RELEASE_DIR VERSION BASE_URL > appcast.xml

  RELEASE_DIR  Directory containing Leah-<VERSION>.zip + sibling .sig files.
  VERSION      SemVer string (e.g. 1.2.3) — used as the appcast item version.
  BASE_URL     Base URL (no trailing slash) where the .zip is hosted.

env:
  LEAH_APPCAST_CHANNEL  "stable" (default) or "rollback" — emitted as the
                        <sparkle:channel> element. The rollback channel is
                        used by Settings → Advanced → Use rollback channel.
USAGE
}

if [ "${1:-}" = "--help" ] || [ "${1:-}" = "-h" ]; then
  usage
  exit 0
fi

if [ "$#" -lt 3 ]; then
  usage >&2
  exit 2
fi

RELEASE_DIR="$1"
VERSION="$2"
BASE_URL="${3%/}"
CHANNEL="${LEAH_APPCAST_CHANNEL:-stable}"

if [ ! -d "$RELEASE_DIR" ]; then
  echo "error: release dir not found: $RELEASE_DIR" >&2
  exit 1
fi

# rfc822 in UTC — Sparkle parses with NSDateFormatter en_US_POSIX %a, %d %b %Y %H:%M:%S %z.
pubdate="$(LC_ALL=C TZ=UTC date '+%a, %d %b %Y %H:%M:%S +0000')"

# Locate the .zip for VERSION. Naming convention matches publish-release.sh.
zip_path="$RELEASE_DIR/Leah-${VERSION}.zip"
if [ ! -f "$zip_path" ]; then
  echo "error: zip not found: $zip_path" >&2
  exit 1
fi

sig_path="${zip_path}.sig"
if [ ! -f "$sig_path" ]; then
  # Fail closed — an appcast with an unsigned <item> would still pass syntax
  # but Sparkle would refuse install, and Settings → Advanced rollback would
  # never converge. Surface the missing-sig early.
  echo "error: missing sidecar signature: $sig_path" >&2
  exit 1
fi

sig_fragment="$(tr -d '\r\n' < "$sig_path")"
if [ -z "$sig_fragment" ]; then
  echo "error: empty signature file: $sig_path" >&2
  exit 1
fi

zip_name="$(basename "$zip_path")"
dl_url="${BASE_URL}/${zip_name}"

# XML attribute escape — matches AppcastTemplate.swift's xmlAttr.
xml_escape() {
  printf '%s' "$1" | sed -e 's/&/\&amp;/g' -e 's/</\&lt;/g' -e 's/"/\&quot;/g'
}

esc_url="$(xml_escape "$dl_url")"
esc_ver="$(xml_escape "$VERSION")"
esc_channel="$(xml_escape "$CHANNEL")"

cat <<XML
<?xml version="1.0" encoding="utf-8"?>
<rss version="2.0" xmlns:sparkle="http://www.andymatuschak.org/xml-namespaces/sparkle">
  <channel>
    <title>Leah</title>
    <link>https://maydow.github.io/leah/appcast.xml</link>
    <description>Leah update feed</description>
    <language>en</language>
    <item>
      <title>Leah ${esc_ver}</title>
      <pubDate>${pubdate}</pubDate>
      <sparkle:channel>${esc_channel}</sparkle:channel>
      <enclosure
        url="${esc_url}"
        sparkle:version="${esc_ver}"
        ${sig_fragment}
        type="application/octet-stream" />
    </item>
  </channel>
</rss>
XML
