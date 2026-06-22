#!/usr/bin/env bash
set -euo pipefail
THIS="$(cd "$(dirname "$0")" && pwd)"

if [ -z "${ANTHROPIC_API_KEY:-}" ]; then
  echo "skip: ANTHROPIC_API_KEY not set"
  exit 0
fi

OUT="$("$THIS/phase1-e2e.sh" 2>&1)"
if echo "$OUT" | grep -q "phase1 e2e ok"; then
  echo "ok"
else
  echo "FAIL: did not see 'phase1 e2e ok'"
  echo "$OUT"
  exit 1
fi
