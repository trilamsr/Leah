#!/usr/bin/env bash
# check-doc-links.sh — fail closed on broken intra-repo doc links in
# markdown link form `[label](docs/path)`.
#
# Scans every tracked `*.md` file under docs/ + CLAUDE.md for inline
# markdown-link bodies pointing at intra-repo prefixes (docs/, internal/,
# cmd/, scripts/) and asserts each cited path exists at HEAD. Catches the
# broken-forward-link pattern.
#
# Ported from regatta scripts/check-doc-links.sh (wave 9-G4, 2026-06-09).
# Allowlist trimmed: omits Makefile.d/ (leah has no Makefile.d) and
# contracts/schemas/ (regatta-specific).
#
# Exit:
#   0 — all referenced paths exist (or skipped per allowlist).
#   1 — one or more broken refs; details on stderr.

set -uo pipefail

ROOT=${ROOT:-.}

docs_list=$(cd "$ROOT" && git ls-files -- 'docs/' 'CLAUDE.md' 2>/dev/null | grep -E '\.md$' || true)

if [ -z "$docs_list" ]; then
  echo "check-doc-links: no docs found to scan" >&2
  exit 0
fi

broken=0
total=0
file_count=0

while IFS= read -r doc; do
  [ -z "$doc" ] && continue
  [ -f "$ROOT/$doc" ] || continue
  file_count=$((file_count + 1))
  while IFS= read -r cand; do
    [ -z "$cand" ] && continue
    case "$cand" in
      http*|mailto:*|'#'*|'./')   continue ;;
      *' '*)                       continue ;;
      docs/*|internal/*|cmd/*|scripts/*) ;;
      *) continue ;;
    esac
    total=$((total + 1))
    # Strip fragment + line-anchor so `path#section` / `path:42` resolve.
    stripped=${cand%%#*}
    stripped=${stripped%%:*}
    case "$stripped" in
      scripts/testdata/*) continue ;;
    esac
    if [ ! -e "$ROOT/$stripped" ]; then
      printf '%s: broken doc link: %s\n' "$doc" "$stripped" >&2
      broken=$((broken + 1))
    fi
  done < <(grep -oE '\]\([^) ]+\)' "$ROOT/$doc" | sed -E 's/^\]\(//; s/\)$//')
done <<<"$docs_list"

if [ "$broken" -gt 0 ]; then
  echo "check-doc-links: $broken broken doc link(s) across $file_count file(s) (scanned $total candidates)" >&2
  exit 1
fi

echo "check-doc-links: ok ($file_count files, $total candidate refs, 0 broken)"
