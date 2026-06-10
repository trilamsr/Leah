#!/usr/bin/env bash
# Fixture-based tests for scripts/check-base-fresh.sh.
# Each case spins up an isolated bare-repo "origin" + working clone to simulate
# the relevant base-staleness condition without touching the host repo.
set -uo pipefail

REPO_ROOT=$(cd "$(dirname "$0")/.." && pwd)
SCRIPT="$REPO_ROOT/scripts/check-base-fresh.sh"

if [[ ! -x "$SCRIPT" ]]; then
  echo "FAIL: $SCRIPT not executable" >&2
  exit 1
fi

PASS=0
FAIL=0

# fresh_fixture <case-name> — prints path to a working clone with origin/main wired up.
fresh_fixture() {
  local name=$1
  local root
  root=$(mktemp -d "${TMPDIR:-/tmp}/base-fresh-${name}.XXXXXX")
  (
    set -e
    git init --quiet --bare "$root/origin.git"
    git init --quiet -b main "$root/main-seed"
    cd "$root/main-seed"
    git config user.email t@t && git config user.name t
    echo seed > seed.txt
    git add seed.txt
    git commit --quiet -m "seed"
    git remote add origin "$root/origin.git"
    git push --quiet origin main
  )
  git clone --quiet "$root/origin.git" "$root/work"
  (cd "$root/work" && git config user.email t@t && git config user.name t)
  echo "$root"
}

# advance_origin <root> <n> — pushes n commits to origin/main from a side clone.
advance_origin() {
  local root=$1 n=$2 i
  local side="$root/advance"
  rm -rf "$side"
  git clone --quiet "$root/origin.git" "$side"
  (
    cd "$side"
    git config user.email t@t && git config user.name t
    for ((i=1;i<=n;i++)); do
      echo "main-$i" > "main-$i.txt"
      git add "main-$i.txt"
      git commit --quiet -m "main commit $i"
    done
    git push --quiet origin main
  )
}

assert() {
  local name=$1 expected=$2 actual=$3
  if [[ "$expected" == "$actual" ]]; then
    echo "PASS $name"
    PASS=$((PASS+1))
  else
    echo "FAIL $name (expected exit $expected, got $actual)"
    FAIL=$((FAIL+1))
  fi
}

# Case 1: happy — branch one commit ahead of fresh main → pass.
case_happy() {
  local root work
  root=$(fresh_fixture happy)
  work="$root/work"
  (
    cd "$work"
    git checkout --quiet -b feat/x
    echo a > a.txt && git add a.txt && git commit --quiet -m "add a"
  )
  (cd "$work" && "$SCRIPT") >/dev/null 2>&1
  assert happy 0 $?
  rm -rf "$root"
}

# Case 2: behind >10 — feature branch made when main was at seed, main has since
# advanced 11 commits → fail with "behind" message.
case_behind() {
  local root work out
  root=$(fresh_fixture behind)
  work="$root/work"
  (
    cd "$work"
    git checkout --quiet -b feat/x
    echo a > a.txt && git add a.txt && git commit --quiet -m "add a"
  )
  advance_origin "$root" 11
  (cd "$work" && git fetch --quiet origin main)
  out=$(cd "$work" && "$SCRIPT" 2>&1)
  local rc=$?
  assert behind-exit 1 $rc
  if grep -q "commits behind" <<<"$out"; then
    echo "PASS behind-msg"
    PASS=$((PASS+1))
  else
    echo "FAIL behind-msg (got: $out)"
    FAIL=$((FAIL+1))
  fi
  rm -rf "$root"
}

# Case 3: built on another open PR's branch. Simulate by making feat/x branch
# include a commit that is NOT in origin/main but IS reachable from another
# remote-tracking ref (another feature branch). The detector compares
# "ahead of merge-base" vs "commits not in origin/main" — when feat/x is
# based on feat/other, its commits not in origin/main exceed the diff
# (origin/main...HEAD) count, tripping the gate.
case_built_on_other() {
  local root work out
  root=$(fresh_fixture builton)
  work="$root/work"
  # Push feat/other to origin with one commit.
  (
    cd "$work"
    git checkout --quiet -b feat/other
    echo o > other.txt && git add other.txt && git commit --quiet -m "other"
    git push --quiet -u origin feat/other
    # feat/x branches off feat/other, adds one commit.
    git checkout --quiet -b feat/x
    echo x > x.txt && git add x.txt && git commit --quiet -m "x"
  )
  out=$(cd "$work" && "$SCRIPT" 2>&1)
  local rc=$?
  assert built-on-other-exit 1 $rc
  if grep -q "another open PR" <<<"$out"; then
    echo "PASS built-on-other-msg"
    PASS=$((PASS+1))
  else
    echo "FAIL built-on-other-msg (got: $out)"
    FAIL=$((FAIL+1))
  fi
  rm -rf "$root"
}

# Case 4: file-overlap — main added file F, this PR also touches F → fail.
case_file_overlap() {
  local root work out
  root=$(fresh_fixture overlap)
  work="$root/work"
  (
    cd "$work"
    git checkout --quiet -b feat/x
    echo a > shared.txt && git add shared.txt && git commit --quiet -m "PR adds shared.txt"
  )
  # Advance origin/main with the SAME filename.
  local side="$root/advance"
  rm -rf "$side"
  git clone --quiet "$root/origin.git" "$side"
  (
    cd "$side"
    git config user.email t@t && git config user.name t
    echo m > shared.txt
    git add shared.txt
    git commit --quiet -m "main also adds shared.txt"
    git push --quiet origin main
  )
  (cd "$work" && git fetch --quiet origin main)
  out=$(cd "$work" && "$SCRIPT" 2>&1)
  local rc=$?
  assert overlap-exit 1 $rc
  if grep -q "shared.txt" <<<"$out"; then
    echo "PASS overlap-msg"
    PASS=$((PASS+1))
  else
    echo "FAIL overlap-msg (got: $out)"
    FAIL=$((FAIL+1))
  fi
  rm -rf "$root"
}

# Case 5: on main — script must short-circuit success.
case_on_main() {
  local root work
  root=$(fresh_fixture onmain)
  work="$root/work"
  (cd "$work" && "$SCRIPT") >/dev/null 2>&1
  assert on-main 0 $?
  rm -rf "$root"
}

case_happy
case_behind
case_built_on_other
case_file_overlap
case_on_main

echo "---"
echo "passed: $PASS  failed: $FAIL"
[[ $FAIL -eq 0 ]]
