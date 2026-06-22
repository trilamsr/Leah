#!/usr/bin/env bash
# Fixtures three specs (SHIPPED / PARTIAL / STALE) and confirms the audit
# script tags each one correctly. Mirrors check-handoff-continuity_test.sh.

set -u

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
GATE="$SCRIPT_DIR/audit-stale-specs.sh"
PASS=0
FAIL=0

pass() { echo "ok: $1"; PASS=$((PASS + 1)); }
fail() { echo "FAIL: $1"; FAIL=$((FAIL + 1)); }

# Build a synthetic repo: a specs/ dir + an internal/ tree with the
# package surfaces that drive SHIPPED / PARTIAL / STALE classification.
mkfixture() {
  local dir
  dir=$(mktemp -d)
  mkdir -p "$dir/docs/engineer/specs"
  mkdir -p "$dir/internal/shipped"
  mkdir -p "$dir/internal/partial"
  mkdir -p "$dir/cmd/leah"
  mkdir -p "$dir/scripts/release"
  # Misnamed-slug pair: filename slug `notifier` resolves to a thin
  # `internal/notifier` stub, but the spec body cites `internal/notify`
  # first — body-mention fallback should pick the heavier pkg.
  mkdir -p "$dir/internal/notifier"
  mkdir -p "$dir/internal/notify"

  # SHIPPED: >100 LOC of non-test Go.
  {
    echo "package shipped"
    for _ in $(seq 1 120); do echo "// line of real code"; done
  } > "$dir/internal/shipped/shipped.go"

  # PARTIAL: package dir exists but <100 LOC.
  printf 'package partial\n\n// only a stub\n' > "$dir/internal/partial/partial.go"

  # Body-fallback target: slug-derived dir is a 1-line stub, body-derived
  # dir holds the real impl (>100 LOC). Fixture exercises the override.
  printf 'package notifier\n\n// thin shim — real impl is in notify\n' > "$dir/internal/notifier/notifier.go"
  {
    echo "package notify"
    for _ in $(seq 1 120); do echo "// real impl line"; done
  } > "$dir/internal/notify/notify.go"

  # STALE spec points at a placeholder pkg that doesn't exist.
  printf '# shipped spec\n\nImplemented in `internal/shipped`.\n' \
    > "$dir/docs/engineer/specs/2026-06-10-shipped.md"
  printf '# partial spec\n\nStub at `internal/partial`.\n' \
    > "$dir/docs/engineer/specs/2026-06-10-partial.md"
  printf '# stale spec\n\nPlaceholder reference to `internal/foo`.\n' \
    > "$dir/docs/engineer/specs/2026-06-10-stale.md"
  # Slug `notifier` lexically resolves to the stub; body names notify first.
  printf '# notifier spec\n\nReal surface is `internal/notify`; thin shim lives at `internal/notifier`.\n' \
    > "$dir/docs/engineer/specs/2026-06-10-notifier.md"

  # SHIPPED via cmd/leah/: slug "local-self-update" diverges from the
  # actual file name "self_upgrade.go". Spec body names the subcommand
  # `leah self-upgrade`, which is what should drive resolution.
  {
    echo "package main"
    for _ in $(seq 1 120); do echo "// cmd surface line"; done
  } > "$dir/cmd/leah/self_upgrade.go"
  printf '# local-self-update\n\nCLI: `leah self-upgrade` swaps the binary.\n' \
    > "$dir/docs/engineer/specs/2026-06-10-local-self-update.md"

  # SHIPPED via scripts/release/: no `internal/` surface at all. Spec
  # body must drive the lookup via the explicit shell-script path.
  {
    echo "#!/usr/bin/env bash"
    for _ in $(seq 1 120); do echo "# notarize step"; done
  } > "$dir/scripts/release/notarize.sh"
  printf '# signed-distribution\n\nRelease: `scripts/release/notarize.sh` runs notarytool.\n' \
    > "$dir/docs/engineer/specs/2026-06-10-signed-distribution.md"

  echo "$dir"
}

run_and_assert() {
  local dir; dir=$(mkfixture)
  local out; out=$("$GATE" --root "$dir" 2>/dev/null)
  local rc=$?

  if [ "$rc" -ne 0 ]; then
    fail "script exited $rc (expected 0)"
    rm -rf "$dir"; return
  fi

  echo "$out" | grep -q $'^SHIPPED\t2026-06-10-shipped.md\t' \
    && pass "SHIPPED row present" \
    || fail "missing SHIPPED row; got: $out"

  echo "$out" | grep -q $'^PARTIAL\t2026-06-10-partial.md\t' \
    && pass "PARTIAL row present" \
    || fail "missing PARTIAL row; got: $out"

  echo "$out" | grep -q $'^STALE\t2026-06-10-stale.md\t' \
    && pass "STALE row present" \
    || fail "missing STALE row; got: $out"

  echo "$out" | grep -q $'^SHIPPED\t2026-06-10-local-self-update.md\t' \
    && pass "cmd/leah surface resolved as SHIPPED" \
    || fail "missing SHIPPED row for cmd/leah surface; got: $out"

  echo "$out" | grep -q $'^SHIPPED\t2026-06-10-signed-distribution.md\t' \
    && pass "scripts/release surface resolved as SHIPPED" \
    || fail "missing SHIPPED row for scripts/release surface; got: $out"

  # Body-fallback override: filename slug `notifier` lexically hits the
  # stub, but the spec body cites `internal/notify` first — that pkg has
  # the real impl, so the row must read SHIPPED internal/notify.
  echo "$out" | grep -q $'^SHIPPED\t2026-06-10-notifier.md\tinternal/notify ' \
    && pass "body-fallback override picked internal/notify" \
    || fail "expected SHIPPED internal/notify for notifier.md; got: $out"

  rm -rf "$dir"
}

run_and_assert

echo "---"
echo "passed: $PASS  failed: $FAIL"
[ "$FAIL" -gt 0 ] && exit 1 || exit 0
