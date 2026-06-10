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

# Reviewer 🔴: method receiver with placeholder panic was bypassing the
# public-func regex (`^func [A-Z]` required cap-letter directly after func).
# Canonical PR#146 case is `func (d *Decoder) SendAudio(...)`.
run_case "feat branch + method receiver panic(not implemented) fails" '
  git checkout -q -b feat/method-panic
  cat > recv.go <<EOF
package x
type R struct{}
func (r *R) PublicMethod() { panic("not implemented") }
EOF
  git add recv.go
  git commit -q -m recv
' 1

# Reviewer 🔴 (TODO variant): method receiver with TODO in decl bypasses the
# `^func [A-Z]` anchor because the line starts with `func (`.
run_case "feat branch + method receiver TODO in decl fails" '
  git checkout -q -b feat/method-todo
  cat > rtodo.go <<EOF
package x
type R struct{}
func (r *R) PublicMethod() { // TODO finish
  return
}
EOF
  git add rtodo.go
  git commit -q -m rtodo
' 1

# Reviewer 🟡a: "not yet implemented" — common stdlib idiom — was bypassing.
run_case "feat branch + panic(not yet implemented) fails" '
  git checkout -q -b feat/notyet
  cat > a.go <<EOF
package x
func A() { panic("not yet implemented") }
EOF
  git add a.go
  git commit -q -m a
' 1

# Reviewer 🟡b: underscore variant "not_implemented" was bypassing.
run_case "feat branch + panic(not_implemented) fails" '
  git checkout -q -b feat/underscore
  cat > b.go <<EOF
package x
func B() { panic("not_implemented") }
EOF
  git add b.go
  git commit -q -m b
' 1

# Reviewer 🟡c: wrapped error panic(errors.New("TODO")) was bypassing.
run_case "feat branch + panic(errors.New TODO) fails" '
  git checkout -q -b feat/errors-new
  cat > c.go <<EOF
package x
import "errors"
func C() { panic(errors.New("TODO")) }
EOF
  git add c.go
  git commit -q -m c
' 1

# Reviewer 🟡c2: panic(fmt.Errorf("not implemented")) was bypassing.
run_case "feat branch + panic(fmt.Errorf not implemented) fails" '
  git checkout -q -b feat/fmt-errorf
  cat > d.go <<EOF
package x
import "fmt"
func D() { panic(fmt.Errorf("not implemented")) }
EOF
  git add d.go
  git commit -q -m d
' 1

# Reviewer 🟡d: multi-line public func signature with placeholder panic
# was bypassing line-based grep.
run_case "feat branch + multiline public func panic fails" '
  git checkout -q -b feat/multiline
  cat > ml.go <<EOF
package x
func Multi(
    a int,
    b int,
) error {
    panic("not implemented")
}
EOF
  git add ml.go
  git commit -q -m ml
' 1

# Reviewer #4: return errors.New("TODO") in non-test files is a placeholder.
run_case "feat branch + return errors.New(TODO) fails" '
  git checkout -q -b feat/return-todo
  cat > rt.go <<EOF
package x
import "errors"
func RT() error { return errors.New("TODO") }
EOF
  git add rt.go
  git commit -q -m rt
' 1

# Wrapped honest-sentinel pattern must still pass — panic(SomeNamedErr) where
# the named error references "not implemented" only in its message string.
run_case "feat branch + named-sentinel wrap still passes" '
  git checkout -q -b feat/honest-wrap
  cat > hw.go <<EOF
package x
import "errors"
var ErrNotShipped = errors.New("feature not yet implemented — owner T-1")
func (R) Send() { panic(ErrNotShipped) }
type R struct{}
EOF
  git add hw.go
  git commit -q -m hw
' 0

# Allow returning a named sentinel error that mentions TODO/not-implemented
# in its message string (honest deferral, not a placeholder return).
run_case "feat branch + return named-sentinel passes" '
  git checkout -q -b feat/return-sentinel
  cat > rs.go <<EOF
package x
import "errors"
var ErrPending = errors.New("not yet implemented")
func RS() error { return ErrPending }
EOF
  git add rs.go
  git commit -q -m rs
' 0

echo "---"
echo "pass=$pass fail=$fail"
[ "$fail" -eq 0 ]
