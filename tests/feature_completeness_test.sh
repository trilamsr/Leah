#!/usr/bin/env bash
# Tests for scripts/check-feature-completeness.sh — the V6/W91 placeholder-detection gate.
# WHY: the gate is the only thing standing between feat/ PRs and "honest placeholder" review;
# regressions here ship `panic("not implemented")` straight to main.
set -euo pipefail

REPO_ROOT=$(cd "$(dirname "$0")/.." && pwd)
GATE="$REPO_ROOT/scripts/check-feature-completeness.sh"

pass=0
fail=0

run_case() {
  local name=$1 setup=$2 want_exit=$3
  local tmp
  tmp=$(mktemp -d)
  pushd "$tmp" >/dev/null

  git init -q
  git config user.email t@t.t
  git config user.name t
  git checkout -q -b main
  echo "// seed" > seed.go
  git add seed.go
  git commit -q -m seed
  git remote add origin "$tmp/.git" 2>/dev/null || true
  git update-ref refs/remotes/origin/main HEAD

  eval "$setup"

  set +e
  bash "$GATE" >/tmp/gate.out 2>&1
  local got=$?
  set -e

  if [ "$got" -eq "$want_exit" ]; then
    echo "PASS: $name (exit=$got)"
    pass=$((pass+1))
  else
    echo "FAIL: $name (exit=$got want=$want_exit)"
    cat /tmp/gate.out
    fail=$((fail+1))
  fi

  popd >/dev/null
  rm -rf "$tmp"
}

# Non-feat branch skips even if the file would otherwise trip the rule.
run_case "non-feat branch skips" '
  git checkout -q -b chore/foo
  cat > bad.go <<EOF
package x
func Bad() { panic("not implemented") }
EOF
  git add bad.go
  git commit -q -m bad
' 0

# Raw placeholder panic in a non-test feat/ file must fail.
run_case "feat branch + panic(not implemented) fails" '
  git checkout -q -b feat/bad
  cat > bad.go <<EOF
package x
func Bad() { panic("not implemented") }
EOF
  git add bad.go
  git commit -q -m bad
' 1

# Honest deferred-feature panic w/ named sentinel must pass.
run_case "feat branch + panic(ErrUplinkNotShipped) passes" '
  git checkout -q -b feat/honest
  cat > honest.go <<EOF
package x
import "errors"
var ErrUplinkNotShipped = errors.New("audio uplink not implemented — W14b owns it")
func Send() { panic(ErrUplinkNotShipped) }
EOF
  git add honest.go
  git commit -q -m honest
' 0

# TODO in a public func declaration line fails.
run_case "feat branch + TODO in public func decl fails" '
  git checkout -q -b feat/todo
  cat > todo.go <<EOF
package x
func Public() { // TODO finish this
  return
}
EOF
  git add todo.go
  git commit -q -m todo
' 1

# TODO in a test file is allowed (test files excluded from scan).
run_case "feat branch + TODO in _test.go passes" '
  git checkout -q -b feat/test-todo
  cat > foo_test.go <<EOF
package x
func TestPublic(t *testing.T) { // TODO finish this
  return
}
EOF
  git add foo_test.go
  git commit -q -m test-todo
' 0

echo "---"
echo "pass=$pass fail=$fail"
[ "$fail" -eq 0 ]
