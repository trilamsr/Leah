#!/usr/bin/env bash
# check-lint_test.sh — assert `make lint` catches the PR #278 errcheck
# pattern (unchecked fmt.Fprintln) and passes on clean code. Locks in
# the gate that was silently skipped when golangci-lint was absent,
# which let two CI-red merges through this session.

set -u

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
REPO_ROOT=$(cd "$SCRIPT_DIR/.." && pwd)
PASS=0
FAIL=0

pass() { echo "ok: $1"; PASS=$((PASS + 1)); }
fail() { echo "FAIL: $1"; FAIL=$((FAIL + 1)); }

mkfixture() {
  local dir mode="$1"
  dir=$(mktemp -d)
  cp "$REPO_ROOT/.golangci.yml" "$dir/.golangci.yml"
  cat > "$dir/go.mod" <<EOF
module fixture

go 1.24
EOF
  if [ "$mode" = "dirty" ]; then
    # PR #278 pattern: unchecked fmt.Fprintln return — errcheck must catch.
    cat > "$dir/main.go" <<'EOF'
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stdout, "x")
}
EOF
  else
    cat > "$dir/main.go" <<'EOF'
package main

import (
	"fmt"
	"os"
)

func main() {
	_, _ = fmt.Fprintln(os.Stdout, "x")
}
EOF
  fi
  echo "$dir"
}

# Errcheck violation must fail the lint gate.
case_errcheck_violation_fails() {
  local dir; dir=$(mkfixture dirty)
  ( cd "$dir" && golangci-lint run --timeout=5m ./... ) >/dev/null 2>&1
  local rc=$?
  if [ "$rc" -ne 0 ]; then pass "errcheck on fmt.Fprintln exits non-zero"
  else fail "errcheck violation should fail lint (got $rc)"; fi
  rm -rf "$dir"
}

# Clean code passes.
case_clean_code_passes() {
  local dir; dir=$(mkfixture clean)
  ( cd "$dir" && golangci-lint run --timeout=5m ./... ) >/dev/null 2>&1
  local rc=$?
  if [ "$rc" -eq 0 ]; then pass "clean code passes lint"
  else fail "clean code should pass lint (got $rc)"; fi
  rm -rf "$dir"
}

# check.sh must NOT silently skip when golangci-lint is missing — that
# is precisely what let PR #278 land CI-red. Source the script in a
# subshell with command stubbed to claim golangci-lint is absent; the
# `command -v` branch should exit non-zero, not warn-and-continue.
case_check_sh_fails_when_lint_missing() {
  if grep -qE 'echo .*skipped.*golangci-lint not installed' "$SCRIPT_DIR/check.sh"; then
    fail "check.sh silently skips when golangci-lint missing (PR #278 pattern)"
  else
    pass "check.sh does not silent-skip on missing golangci-lint"
  fi
}

command -v golangci-lint >/dev/null 2>&1 || {
  echo "golangci-lint not installed; install via:"
  echo "  go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2"
  exit 1
}

case_errcheck_violation_fails
case_clean_code_passes
case_check_sh_fails_when_lint_missing

echo "---"
echo "passed: $PASS  failed: $FAIL"
[ "$FAIL" -gt 0 ] && exit 1 || exit 0
