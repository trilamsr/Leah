#!/usr/bin/env bash
# Tests for scripts/check.sh parallel pipeline.
# Stubs PATH so go/golangci-lint/scripts resolve to controllable fakes.
set -uo pipefail

REPO_ROOT=$(cd "$(dirname "$0")/.." && pwd)
CHECK_SH="$REPO_ROOT/scripts/check.sh"

PASS=0
FAIL=0

run_case() {
  local name="$1"; shift
  local expected_exit="$1"; shift
  local stub_dir; stub_dir=$(mktemp -d)
  local out_file; out_file=$(mktemp)

  # Each remaining arg pair is: stub_name=exit_code[:stdout_msg]
  for spec in "$@"; do
    local sname="${spec%%=*}"
    local rest="${spec#*=}"
    local code="${rest%%:*}"
    local msg=""
    if [[ "$rest" == *:* ]]; then msg="${rest#*:}"; fi
    cat >"$stub_dir/$sname" <<EOF
#!/usr/bin/env bash
[ -n "$msg" ] && echo "$msg"
exit $code
EOF
    chmod +x "$stub_dir/$sname"
  done

  # Stub scripts/ helpers — point check.sh at a per-case stub scripts dir
  local stub_scripts; stub_scripts=$(mktemp -d)
  cp "$CHECK_SH" "$stub_scripts/check.sh"
  for helper in check-comment-density.sh check-no-bare-sleep.sh check-doc-links.sh check-pr-body-close-keywords.sh check-base-fresh.sh check-feature-completeness.sh; do
    if [ -x "$stub_dir/$helper" ]; then
      cp "$stub_dir/$helper" "$stub_scripts/$helper"
    else
      # default: pass
      printf '#!/usr/bin/env bash\nexit 0\n' >"$stub_scripts/$helper"
      chmod +x "$stub_scripts/$helper"
    fi
  done

  PATH="$stub_dir:/usr/bin:/bin" bash "$stub_scripts/check.sh" >"$out_file" 2>&1
  local actual_exit=$?

  if [ "$actual_exit" -eq "$expected_exit" ]; then
    echo "PASS: $name (exit=$actual_exit)"
    PASS=$((PASS+1))
  else
    echo "FAIL: $name (expected exit=$expected_exit, got=$actual_exit)"
    echo "----- output -----"
    cat "$out_file"
    echo "------------------"
    FAIL=$((FAIL+1))
  fi

  # Export last output for caller assertions
  LAST_OUT="$out_file"
}

assert_contains() {
  local label="$1" needle="$2"
  if grep -q -- "$needle" "$LAST_OUT"; then
    echo "  PASS: $label contains '$needle'"
    PASS=$((PASS+1))
  else
    echo "  FAIL: $label missing '$needle'"
    cat "$LAST_OUT"
    FAIL=$((FAIL+1))
  fi
}

# Case 1: build failure short-circuits — test/vet/lint stubs should NOT run.
run_case "build failure short-circuits" 1 \
  "go=1:build broken" \
  "golangci-lint=0"
assert_contains "build failure" "build broken"
if grep -q -- "--- vet ---" "$LAST_OUT"; then
  echo "  FAIL: parallel block ran despite build failure"
  FAIL=$((FAIL+1))
else
  echo "  PASS: parallel block did not run on build failure"
  PASS=$((PASS+1))
fi

# Case 1b: test failure short-circuits — same go stub exits 1, parallel
# block must not run.
GO_PASSBUILD_FAILTEST=$(mktemp -d)
cat >"$GO_PASSBUILD_FAILTEST/go" <<'EOF'
#!/usr/bin/env bash
case "$1" in
  build) exit 0 ;;
  test) echo "test broken"; exit 1 ;;
  *) exit 0 ;;
esac
EOF
chmod +x "$GO_PASSBUILD_FAILTEST/go"
stub_scripts=$(mktemp -d)
cp "$CHECK_SH" "$stub_scripts/check.sh"
for h in check-comment-density.sh check-no-bare-sleep.sh check-doc-links.sh check-pr-body-close-keywords.sh check-base-fresh.sh check-feature-completeness.sh; do
  printf '#!/usr/bin/env bash\nexit 0\n' >"$stub_scripts/$h"; chmod +x "$stub_scripts/$h"
done
out=$(PATH="$GO_PASSBUILD_FAILTEST:/usr/bin:/bin" bash "$stub_scripts/check.sh" 2>&1)
ec=$?
if [ "$ec" -eq 1 ] && echo "$out" | grep -q "test broken" && ! echo "$out" | grep -q -- "--- vet ---"; then
  echo "PASS: test failure short-circuits"
  PASS=$((PASS+1))
else
  echo "FAIL: test failure short-circuit (exit=$ec)"
  echo "$out"
  FAIL=$((FAIL+1))
fi

# Case 2: all stages green → exit 0.
# Need a `go` stub that succeeds for build AND test AND vet.
run_case "all stages pass" 0 \
  "go=0" \
  "golangci-lint=0"

# Case 3: single failing stage propagates non-zero exit and prints its log.
run_case "lint failure propagates" 1 \
  "go=0" \
  "golangci-lint=1:lint err xyz"
assert_contains "lint failure" "lint err xyz"

# Case 4: helper script failure propagates and log is printed.
SLEEP_STUB=$(mktemp)
cat >"$SLEEP_STUB" <<'EOF'
#!/usr/bin/env bash
echo "bare sleep found in foo_test.go"
exit 1
EOF
chmod +x "$SLEEP_STUB"
run_case "sleep-check failure propagates" 1 \
  "go=0" \
  "golangci-lint=0" \
  "check-no-bare-sleep.sh=1:bare sleep found in foo_test.go"
assert_contains "sleep failure" "bare sleep found"

# Case 5: parallel execution — vet+lint+helpers (post-test) should
# complete in ~max(sleep), not sum(sleep). Build+test are serial pre-block,
# so we make them no-op and time the parallel tail. 5 stages × 2s sleep
# = 10s serial vs ~2s parallel. Also stub check-base-fresh.sh (serial tail).
SLOW_GO=$(mktemp)
cat >"$SLOW_GO" <<'EOF'
#!/usr/bin/env bash
case "$1" in
  build|test) exit 0 ;;
  vet) sleep 2; exit 0 ;;
  *) exit 0 ;;
esac
EOF
chmod +x "$SLOW_GO"
SLOW_LINT=$(mktemp); cat >"$SLOW_LINT" <<'EOF'
#!/usr/bin/env bash
sleep 2; exit 0
EOF
chmod +x "$SLOW_LINT"

stub_dir=$(mktemp -d)
cp "$SLOW_GO" "$stub_dir/go"
cp "$SLOW_LINT" "$stub_dir/golangci-lint"
stub_scripts=$(mktemp -d)
cp "$CHECK_SH" "$stub_scripts/check.sh"
# Parallel-block helpers sleep 2s each (vet+lint+density+sleep+doclinks);
# serial-tail helpers (feature-completeness, base-fresh, pr-body) instant —
# we're isolating the parallel block's wall-clock, not the serial tail.
for helper in check-comment-density.sh check-no-bare-sleep.sh check-doc-links.sh; do
  cat >"$stub_scripts/$helper" <<'EOF'
#!/usr/bin/env bash
sleep 2; exit 0
EOF
  chmod +x "$stub_scripts/$helper"
done
for helper in check-pr-body-close-keywords.sh check-base-fresh.sh check-feature-completeness.sh; do
  printf '#!/usr/bin/env bash\nexit 0\n' >"$stub_scripts/$helper"
  chmod +x "$stub_scripts/$helper"
done

START=$(date +%s)
PATH="$stub_dir:/usr/bin:/bin" bash "$stub_scripts/check.sh" >/dev/null 2>&1
END=$(date +%s)
ELAPSED=$((END - START))
# 5 parallel stages × 2s ≈ 2s parallel; serial tail instant; 4s ceiling
# = 2s margin for CI scheduler noise. Serial would be ~10s.
if [ "$ELAPSED" -le 4 ]; then
  echo "PASS: parallel timing (elapsed=${ELAPSED}s, threshold<=4s)"
  PASS=$((PASS+1))
else
  echo "FAIL: stages ran serially (elapsed=${ELAPSED}s, expected<=4s)"
  FAIL=$((FAIL+1))
fi

echo
echo "Results: $PASS passed, $FAIL failed"
[ "$FAIL" -eq 0 ]
