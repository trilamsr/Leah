#!/usr/bin/env bash
set -euo pipefail
mode="${LEAH_SMOKE_LIVE:-0}"
echo "smoke mode: $([ "$mode" = "1" ] && echo LIVE || echo MOCK)"

if [ ! -d scripts/smoke ]; then
  echo "no smoke tests yet (scripts/smoke/)"
  exit 0
fi

# Check for any executable smoke scripts (excluding _lib.sh and *_test.sh harnesses)
has_smokes=0
for s in scripts/smoke/*.sh; do
  [ -x "$s" ] || continue
  case "$(basename "$s")" in _*.sh|*_test.sh) continue ;; esac
  has_smokes=1
  break
done

if [ "$has_smokes" -eq 0 ]; then
  echo "no smoke tests yet (scripts/smoke/)"
  exit 0
fi

fail=0
for s in scripts/smoke/*.sh; do
  [ -x "$s" ] || continue
  case "$(basename "$s")" in _*.sh|*_test.sh) continue ;; esac
  name=$(basename "$s" .sh)
  printf '%s: ' "$name"
  if "$s"; then
    echo "PASS"
  else
    echo "FAIL"
    fail=$((fail+1))
  fi
done

[ "$fail" -eq 0 ] && exit 0 || exit 1
