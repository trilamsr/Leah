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

# _test.go excluded so a thick test file can't masquerade as a shipped
# surface; shell scripts have no such convention so all .sh files count.
extra_loc() {
  local files="$1" file abs total=0 n
  while IFS= read -r file; do
    [ -z "$file" ] && continue
    case "$file" in *_test.go) continue ;; esac
    abs="$ROOT/$file"
    [ -f "$abs" ] || continue
    n=$(wc -l < "$abs" 2>/dev/null | tr -d ' ')
    total=$((total + ${n:-0}))
  done <<EOF
$files
EOF
  echo "$total"
}

# Slugs often diverge from filenames (local-self-update vs self_upgrade.go);
# the `leah <subcommand>` mention in prose is more reliable than slug tokens.
resolve_cmd_files() {
  local spec="$1" tokens token snake f out=""
  tokens=$(grep -oE '`?leah[ -][a-z][a-z0-9_-]+' "$spec" 2>/dev/null \
    | sed -E 's/^`?leah[ -]//' \
    | awk '!seen[$0]++')
  while IFS= read -r token; do
    [ -z "$token" ] && continue
    snake=$(echo "$token" | tr '-' '_')
    for f in "$ROOT/cmd/leah/$snake".go "$ROOT/cmd/leah/$snake"_*.go; do
      [ -f "$f" ] || continue
      out="$out${f#$ROOT/}
"
    done
  done <<EOF
$tokens
EOF
  printf '%s' "$out" | awk 'NF && !seen[$0]++'
}

# Some specs (signed-distribution) ship entirely as shell with no internal/
# surface; literal path match avoids false positives on unrelated scripts.
resolve_script_files() {
  local spec="$1"
  grep -oE 'scripts/[a-zA-Z0-9_./-]+\.sh' "$spec" 2>/dev/null \
    | awk '!seen[$0]++'
}

classify() {
  local spec="$1"
  local fname stem pkg pkg_abs loc cmd_files script_files extra total reason
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

  if [ -d "$pkg_abs" ]; then
    loc=$(nontest_loc "$pkg_abs")
    reason="$pkg ($loc LOC)"
  else
    loc=0
    reason=""
  fi

  # Fold cmd/ and scripts/ LOC in before STALE — some specs ship there only.
  cmd_files=$(resolve_cmd_files "$spec")
  script_files=$(resolve_script_files "$spec")
  extra=$(extra_loc "$(printf '%s\n%s' "$cmd_files" "$script_files")")
  total=$((loc + extra))

  if [ -n "$cmd_files$script_files" ]; then
    if [ -n "$reason" ]; then
      reason="$reason + cmd/scripts ($extra LOC)"
    else
      reason="cmd/scripts ($extra LOC)"
    fi
  fi

  if [ "$total" -eq 0 ]; then
    printf 'STALE\t%s\tno pkg dir (%s)\n' "$fname" "$pkg"
    return
  fi

  if [ "$total" -gt 100 ]; then
    printf 'SHIPPED\t%s\t%s\n' "$fname" "$reason"
  else
    printf 'PARTIAL\t%s\t%s stub/thin\n' "$fname" "$reason"
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
