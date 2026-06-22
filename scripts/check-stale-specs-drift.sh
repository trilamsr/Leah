#!/usr/bin/env bash
# Fails when docs/engineer/runbooks/stale-specs.md disagrees with
# `audit-stale-specs.sh` on the STALE count. The runbook was hand-authored
# before the audit script existed (PR #322 vs #328) and quietly drifted;
# this gate keeps the regenerated runbook honest.

set -u

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
ROOT=$(cd "$SCRIPT_DIR/.." && pwd)
RUNBOOK="$ROOT/docs/engineer/runbooks/stale-specs.md"
AUDIT="$SCRIPT_DIR/audit-stale-specs.sh"

[ -r "$RUNBOOK" ] || { echo "missing runbook: $RUNBOOK" >&2; exit 1; }
[ -x "$AUDIT" ] || [ -r "$AUDIT" ] || { echo "missing audit script: $AUDIT" >&2; exit 1; }

SCRIPT_COUNT=$(bash "$AUDIT" --root "$ROOT" | grep -c '^STALE' || true)
RUNBOOK_COUNT=$(grep -oE '^## STALE \(([0-9]+)\)' "$RUNBOOK" | grep -oE '[0-9]+' | head -1 || true)

if [ -z "$RUNBOOK_COUNT" ]; then
  echo "runbook missing '## STALE (N)' header" >&2
  exit 1
fi

if [ "$SCRIPT_COUNT" != "$RUNBOOK_COUNT" ]; then
  echo "stale-specs runbook drift: script=$SCRIPT_COUNT runbook=$RUNBOOK_COUNT" >&2
  echo "regenerate: bash $AUDIT" >&2
  exit 1
fi

exit 0
