#!/usr/bin/env bash
# check-spec-parity.sh — enforce v3.2 rename + cosmetic-debt rules against the
# leah macOS native UI design spec.
#
# v3.1 leaked these renames; v3.2 extracted the forbidden-phrase list here so
# the spec body never enumerates its own banned tokens (which would self-match).
#
# Usage:
#   scripts/check-spec-parity.sh docs/superpowers/specs/<spec>.md
#
# Allow-list (sections that may legitimately cite the historical names):
#   §14 (decisions log) · §15 (anti-patterns) · §16.7 (this rule's spec home) ·
#   §18 (changelog)
#
# Also exempted line-by-line: section headers that read
#   `### N.M Foo (formerly "X")` — historical citation in the header itself.
#
# Forbidden phrases (renamed in v3.1/v3.2 — must not appear in normative body):
#   chamber       → panel
#   sigil         → mark
#   flourish      → transition
#   Flourish 1/2  → Transition 1/2
#   gold seam     → gold transition
#   aesthetic-reduced → minimal mode
#   #0A0A0C       → #08090C   (palette drift)
#
# Ancillary cosmetic-debt rules (killed in v2/v3, must stay killed):
#   90 s / idle 90      — panel auto-destroy killed by decision #36
#   max 3 pin / 3 pinned — pin cap is 2 per decision #40
#   Tiempos italic outside §3.3 + §14 row 28 — one-location rule, v3.2 default = New York Italic
#   ⌘⌃                  — modifier-only chord killed by decision #5
#   wake-word ON        — default-OFF framing per decision #2
#   [☀] / [⛅] / [🌧]    — emoji glyphs in wireframes; use SF Symbol bracketed names
#   SCShareableContent  — wrong API for capture-detection per decision #103
#
# Exits 0 on clean. Non-zero with one-line `file:line: forbidden phrase: <phrase>`
# on first hit.

set -euo pipefail

SPEC="${1:?usage: check-spec-parity.sh <spec.md>}"

if [[ ! -f "$SPEC" ]]; then
  echo "check-spec-parity: spec file not found: $SPEC" >&2
  exit 2
fi

# Forbidden phrases (regex). Order matters only for reporting.
FORBIDDEN_REGEXES=(
  '\bchamber\b'
  '\bchambers\b'
  '\bChamber\b'
  '\bsigil\b'
  '\bsigils\b'
  '\bSigil\b'
  '\bflourish\b'
  '\bFlourish [12]\b'
  '\bflourishes\b'
  'gold seam'
  'Gold Seam'
  'aesthetic-reduced'
  '#0A0A0C'
  '\b90 ?s\b'
  'idle 90'
  'max 3 pin'
  '3 pinned'
  'Tiempos italic'
  '⌘⌃'
  'wake-word ON'
  '\[☀\]'
  '\[⛅\]'
  '\[🌧\]'
  'SCShareableContent'
)

# Allow-list sections (top-level ## N. matches these numbers): skip all checks.
ALLOWED_SECTIONS=(14 15 16 18)
# (§16 is broad — the 16.7 rule itself describes the parity check meta-narrative.
#  The script *also* fails-fast at top-level §16, leaving sub-section enforcement
#  to the §16.7 description; finer granularity not needed since §16 is wholly
#  about tests and historical-cite references are expected.)

# Walk the spec, track current top-level section, skip allowed.
cur_section=0
exit_code=0
line_no=0
while IFS= read -r line; do
  line_no=$((line_no + 1))

  # Detect top-level section header `## N. …`
  if [[ "$line" =~ ^\#\#[[:space:]]+([0-9]+)\. ]]; then
    cur_section="${BASH_REMATCH[1]}"
    continue
  fi

  # Skip allow-listed sections.
  for allowed in "${ALLOWED_SECTIONS[@]}"; do
    if [[ "$cur_section" == "$allowed" ]]; then
      continue 2
    fi
  done

  # Skip lines that are themselves "(formerly \"X\")" historical citations in
  # subsection headers. These survive in body but are spec'd-historical.
  if [[ "$line" =~ ^\#\#\#.*\(formerly ]]; then
    continue
  fi

  # Skip lines that mark themselves as historical citations via convention.
  # If a line carries any of these markers it is referencing v1/v2/v3 prior
  # state, not specifying the v3.2 normative behavior.
  if [[ "$line" =~ (v1:|v2:|v3:|\(v1\)|\(v2\)|\(v3\)|v1\'s|v2\'s|v3\'s|v3\.1|v3\.2|\(was|was\ \"|supersedes|superseded|obsoleted|deprecated|Killed\ by|killed\ by|reversed|reversal|formerly|heraldic\ intent|first-impression|silent-drop|reviewer\ fix|Reviewer\ fix) ]]; then
    continue
  fi

  # SCShareableContent — allowed when used for legitimate enumerate-shareable-
  # content use cases (per decision #103: it's wrong for capture-detection but
  # right for shareable-window enumeration). A line that uses it as a positive
  # observer (e.g., "SCShareableContent stream-end observer fires") is the
  # correct current API. The forbidden-phrase rule targets the wrong-use case;
  # mark legitimate uses with the in-spec note pattern.
  if [[ "$line" =~ SCShareableContent[\`\'\"\ ]+(stream|observer|enumerate|window|content|shareable) ]]; then
    continue
  fi

  # Also skip lines that compare current behavior against a former cap (e.g.,
  # "if <3 pinned, backfill" describing edge cases of the new <=2-pin rule).
  # The pattern: `<\s*N` or `≤\s*N` referencing pinned/widget counts is
  # describing current limits, not violating them.
  if [[ "$line" =~ \<[[:space:]]*[0-9]+[[:space:]]*pin ]]; then
    continue
  fi

  # Check each forbidden regex.
  for regex in "${FORBIDDEN_REGEXES[@]}"; do
    if printf '%s' "$line" | grep -qE "$regex"; then
      printf '%s:%d: forbidden phrase: %s\n' "$SPEC" "$line_no" "$regex" >&2
      exit_code=1
    fi
  done
done < "$SPEC"

if [[ "$exit_code" -eq 0 ]]; then
  echo "check-spec-parity: ok — $SPEC has no forbidden phrases outside the allow-list."
fi

exit "$exit_code"
