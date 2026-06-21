#!/usr/bin/env bash
# publish-tarball_test.sh — assert publish-tarball.sh builds the source tarball
# the brew formula's `url` expects (leah-vX.Y.Z-src.tar.gz + sidecar SHA256),
# excludes development noise (.claude/, node_modules/, dist/), and refuses
# bad tags.
#
# MAY-268: today no release publishes leah-v0.0.1-mvp5-src.tar.gz, so
# `brew install leah` fails to resolve. This gate locks the contract so the
# release workflow can never regress past a tag push without the artifact.

set -u

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
PUB="$SCRIPT_DIR/publish-tarball.sh"
PASS=0
FAIL=0

pass() { echo "ok: $1"; PASS=$((PASS + 1)); }
fail() { echo "FAIL: $1"; FAIL=$((FAIL + 1)); }

# Build a minimal git repo fixture: a few real files + the noise dirs that
# must be excluded from the published tarball.
make_fixture() {
  local dir=$1
  (
    cd "$dir"
    git init -q
    git config user.email t@t
    git config user.name t
    mkdir -p cmd/leah internal/foo .claude/worktrees node_modules/x dist Formula .github/workflows
    echo "package main"  > cmd/leah/main.go
    echo "package foo"   > internal/foo/foo.go
    echo "noise"         > .claude/worktrees/x
    echo "noise"         > node_modules/x/pkg.json
    echo "noise"         > dist/leah
    echo "class Leah"    > Formula/leah.rb
    echo "name: release" > .github/workflows/release.yml
    echo "module example.com/leah" > go.mod
    git add -A
    git commit -q -m init
    # Annotated tag — `git archive` formats accept either, but matching the
    # real release contract (annotated tags get release notes) keeps the
    # fixture honest.
    git tag -a v0.0.1-test -m "test tag"
  )
}

run_case_missing_tag_rejected() {
  "$PUB" >/dev/null 2>&1
  local rc=$?
  if [ "$rc" -eq 2 ]; then
    pass "missing TAG exits 2"
  else
    fail "missing TAG should exit 2 (got $rc)"
  fi
}

run_case_bad_tag_rejected() {
  TAG="not-a-version" "$PUB" --dry-run >/dev/null 2>&1
  local rc=$?
  if [ "$rc" -eq 2 ]; then
    pass "non-semver TAG exits 2"
  else
    fail "non-semver TAG should exit 2 (got $rc)"
  fi
}

run_case_builds_named_tarball() {
  local work
  work=$(mktemp -d)
  make_fixture "$work"
  local out
  out=$(mktemp -d)
  (
    cd "$work"
    TAG=v0.0.1-test OUT_DIR="$out" "$PUB" >/dev/null 2>&1
  )
  if [ -f "$out/leah-v0.0.1-test-src.tar.gz" ]; then
    pass "produces leah-vX.Y.Z-src.tar.gz matching brew formula URL"
  else
    fail "expected $out/leah-v0.0.1-test-src.tar.gz to exist"
    ls -la "$out" 2>&1 | head
  fi
  rm -rf "$work" "$out"
}

run_case_emits_sha256_sidecar() {
  local work out
  work=$(mktemp -d); out=$(mktemp -d)
  make_fixture "$work"
  ( cd "$work" && TAG=v0.0.1-test OUT_DIR="$out" "$PUB" >/dev/null 2>&1 )
  if [ -f "$out/leah-v0.0.1-test-src.tar.gz.sha256" ]; then
    pass "writes SHA256 sidecar for brew formula sha256 stanza"
  else
    fail "expected $out/leah-v0.0.1-test-src.tar.gz.sha256 to exist"
  fi
  rm -rf "$work" "$out"
}

run_case_excludes_noise() {
  local work out extracted
  work=$(mktemp -d); out=$(mktemp -d); extracted=$(mktemp -d)
  make_fixture "$work"
  ( cd "$work" && TAG=v0.0.1-test OUT_DIR="$out" "$PUB" >/dev/null 2>&1 )
  ( cd "$extracted" && tar xzf "$out/leah-v0.0.1-test-src.tar.gz" )
  local bad=0
  # Mirror the scrub set in publish-tarball.sh — if it grows, this list must
  # grow too. Formula/ and .github/ are tracked-but-excluded; the others come
  # from .gitignore but are scrubbed defensively.
  for noise in .claude node_modules dist Formula .github; do
    if find "$extracted" -name "$noise" -type d 2>/dev/null | grep -q .; then
      fail "tarball must not contain $noise/"
      bad=1
    fi
  done
  if [ "$bad" -eq 0 ]; then
    pass "tarball excludes .claude/ node_modules/ dist/ Formula/ .github/"
  fi
  # Sanity: real source IS present.
  if find "$extracted" -name "main.go" 2>/dev/null | grep -q .; then
    pass "tarball includes cmd/leah/main.go"
  else
    fail "tarball missing cmd/leah/main.go"
  fi
  rm -rf "$work" "$out" "$extracted"
}

run_case_dry_run_skips_upload() {
  local work out
  work=$(mktemp -d); out=$(mktemp -d)
  make_fixture "$work"
  # Force the upload path that would hit gh by setting UPLOAD=1; --dry-run
  # must override and NOT invoke gh (we can detect by absence of error
  # since the fixture has no gh auth).
  local logf
  logf=$(mktemp)
  ( cd "$work" && TAG=v0.0.1-test OUT_DIR="$out" UPLOAD=1 "$PUB" --dry-run >"$logf" 2>&1 )
  local rc=$?
  if [ "$rc" -eq 0 ] && grep -q "dry-run" "$logf"; then
    pass "--dry-run skips gh upload and exits 0"
  else
    fail "--dry-run should exit 0 with dry-run marker (rc=$rc)"
    cat "$logf"
  fi
  rm -rf "$work" "$out" "$logf"
}

run_case_missing_tag_rejected
run_case_bad_tag_rejected
run_case_builds_named_tarball
run_case_emits_sha256_sidecar
run_case_excludes_noise
run_case_dry_run_skips_upload

echo "---"
echo "passed: $PASS  failed: $FAIL"
if [ "$FAIL" -gt 0 ]; then
  exit 1
fi
