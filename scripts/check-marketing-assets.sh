#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
MISSING=0
for f in docs/assets/marketing/hero-{01-summon,02-ambient,03-focus,04-dashboard}.png \
         docs/assets/marketing/mark.{svg,pdf} \
         docs/assets/marketing/mark-{18,24,56,96}.png; do
  if [ ! -f "$ROOT/$f" ] || [ ! -s "$ROOT/$f" ]; then
    echo "missing or empty: $f"
    MISSING=1
  fi
done
exit $MISSING
