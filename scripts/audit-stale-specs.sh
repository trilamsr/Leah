#!/usr/bin/env bash
# Mechanically reproduces the docs/engineer/runbooks/stale-specs.md
# SHIPPED / PARTIAL / STALE classification so a re-audit doesn't need a
# human pass. Advisory — always exits 0.
#
# Usage: audit-stale-specs.sh [--root <repo-root>]
#
# Output per spec, tab-separated, sorted by status then filename:
#   <STATUS>\t<spec-filename>\t<reason>

set -u

ROOT=""
while [ $# -gt 0 ]; do
  case "$1" in
    --root) ROOT="$2"; shift 2 ;;
    *) shift ;;
  esac
done

if [ -z "$ROOT" ]; then
  SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
  ROOT=$(cd "$SCRIPT_DIR/.." && pwd)
fi

SPECS_DIR="$ROOT/docs/engineer/specs"
[ -d "$SPECS_DIR" ] || { echo "no specs dir: $SPECS_DIR" >&2; exit 0; }

# Resolve a spec to a package directory under the repo root. Priority:
#   1. filename stem → internal/adapters/<x>  (if `-adapter` suffix)
#   2. filename stem → internal/<x>
#   3. spec body → first internal/<pkg> whose two-segment dir exists
#   4. fallback: internal/<stem>  (drives STALE for placeholder specs)
# Filename-first beats body-first because spec prose often cross-refs
# sibling packages before naming its own — first-mention picks the wrong
# row. The body scan still rescues specs whose package name diverges
# from the filename (knowledge-graph.md → internal/knowledge).
resolve_pkg() {
  local spec="$1" stem="$2" base guess hits hit
  base="${stem%-adapter}"
  if [ "$base" != "$stem" ] && [ -d "$ROOT/internal/adapters/$base" ]; then
    echo "internal/adapters/$base"; return
  fi
  guess="internal/$stem"
  if [ -d "$ROOT/$guess" ]; then echo "$guess"; return; fi

  # Filename guess missed disk; check the body for a real package mention.
  # Restrict to top-level (two-segment) paths and preserve document
  # order — first mention is the spec's own package, not a cross-ref.
  hits=$(grep -oE 'internal/[a-zA-Z0-9_-]+' "$spec" 2>/dev/null | awk '!seen[$0]++' || true)
  while IFS= read -r hit; do
    [ -z "$hit" ] && continue
    if [ -d "$ROOT/$hit" ]; then echo "$hit"; return; fi
  done <<EOF
$hits
EOF

  # Nothing on disk — return the filename-derived guess; STALE fires.
  echo "$guess"
}

# Non-test Go LOC under a directory. Returns 0 if dir missing.
nontest_loc() {
  local dir="$1"
  [ -d "$dir" ] || { echo 0; return; }
  find "$dir" -type f -name '*.go' ! -name '*_test.go' -print0 2>/dev/null \
    | xargs -0 cat 2>/dev/null \
    | wc -l \
    | tr -d ' '
}

classify() {
  local spec="$1"
  local fname stem pkg pkg_abs loc
  fname=$(basename "$spec")
  # Strip YYYY-MM-DD- prefix; everything after becomes the package stem.
  stem=$(echo "${fname%.md}" | sed -E 's/^[0-9]{4}-[0-9]{2}-[0-9]{2}-//')

  pkg=$(resolve_pkg "$spec" "$stem")
  pkg_abs="$ROOT/$pkg"

  # Placeholder pattern from the runbook — internal/foo is the canonical
  # "spec never picked a real package" tell.
  if [ "$pkg" = "internal/foo" ]; then
    printf 'STALE\t%s\tplaceholder pkg %s\n' "$fname" "$pkg"
    return
  fi

  if [ ! -d "$pkg_abs" ]; then
    printf 'STALE\t%s\tno pkg dir (%s)\n' "$fname" "$pkg"
    return
  fi

  loc=$(nontest_loc "$pkg_abs")
  if [ "$loc" -gt 100 ]; then
    printf 'SHIPPED\t%s\t%s (%s LOC)\n' "$fname" "$pkg" "$loc"
  else
    printf 'PARTIAL\t%s\t%s (%s LOC; stub/thin)\n' "$fname" "$pkg" "$loc"
  fi
}

# Stream all specs through the classifier, then sort by status order
# (SHIPPED, PARTIAL, STALE) and filename for stable diffs.
{
  for spec in "$SPECS_DIR"/*.md; do
    [ -f "$spec" ] || continue
    classify "$spec"
  done
} | awk -F'\t' '
  BEGIN { order["SHIPPED"]=1; order["PARTIAL"]=2; order["STALE"]=3 }
  { printf "%d\t%s\n", order[$1], $0 }
' | sort -k1,1n -k3,3 | cut -f2-

exit 0
