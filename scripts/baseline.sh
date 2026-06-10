#!/usr/bin/env bash
set -euo pipefail
ts=$(date -u +%FT%TZ)
mkdir -p ~/.leah-state/
log=~/.leah-state/baseline-history.jsonl
out=/tmp/baseline-$$.txt

go test -race ./... > "$out.tests" 2>&1 || true
test_pass=$(grep -c '^ok' "$out.tests" 2>/dev/null || true)
test_fail=$(grep -c '^FAIL' "$out.tests" 2>/dev/null || true)
: "${test_pass:=0}"
: "${test_fail:=0}"

go test -bench=. -benchmem -count=1 ./internal/obs/... > "$out.bench" 2>&1 || true
allocs=$(awk '/^Benchmark/ {sum+=$5} END {print sum+0}' "$out.bench")
ns_op=$(awk '/^Benchmark/ {sum+=$3} END {print int(sum)}' "$out.bench")
: "${allocs:=0}"
: "${ns_op:=0}"

density_pass=1
./scripts/check-comment-density.sh > /dev/null 2>&1 || density_pass=0

printf '{"ts":"%s","test_pass":%d,"test_fail":%d,"bench_total_ns":%d,"bench_total_allocs":%d,"density_pass":%d}\n' \
  "$ts" "$test_pass" "$test_fail" "$ns_op" "$allocs" "$density_pass" >> "$log"

tail -1 "$log"
rm -f "$out".*
