#!/usr/bin/env bash
# WHY: guard regression — APPROVE comment older than HEAD commit timestamp must block merge.
set -u

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GUARD="$SCRIPT_DIR/check-amend-after-approve.sh"

PASS=0
FAIL=0

run() {
  local name="$1"; shift
  local want_code="$1"; shift
  local want_substr="$1"; shift
  local got
  got="$("$@" 2>&1)"
  local code=$?
  if [ "$code" -ne "$want_code" ]; then
    echo "FAIL $name: exit $code want $want_code; out=$got"
    FAIL=$((FAIL+1)); return
  fi
  if [ -n "$want_substr" ] && ! printf '%s' "$got" | grep -qF "$want_substr"; then
    echo "FAIL $name: missing substr '$want_substr'; out=$got"
    FAIL=$((FAIL+1)); return
  fi
  echo "PASS $name"
  PASS=$((PASS+1))
}

# WHY: stub gh so tests are hermetic. $GH_FIXTURE_DIR holds canned responses per invocation.
STUB_DIR="$(mktemp -d)"
trap 'rm -rf "$STUB_DIR"' EXIT
cat >"$STUB_DIR/gh" <<'STUB'
#!/usr/bin/env bash
# WHY: mimic gh's -q (jq filter) and --json (passthrough) so guard exercises real code path.
sub="$1"; shift
filter=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    -q|--jq) filter="$2"; shift 2 ;;
    --json) shift 2 ;;
    --repo) shift 2 ;;
    *) shift ;;
  esac
done
case "$sub" in
  pr)   payload="$(cat "$GH_FIXTURE_DIR/pr-view.json")" ;;
  api)  payload="$(cat "$GH_FIXTURE_DIR/commit.json")" ;;
  repo) echo "stub-owner/stub-repo"; exit 0 ;;
  *)    echo "stub gh: unhandled $sub" >&2; exit 2 ;;
esac
if [ -n "$filter" ]; then
  printf '%s' "$payload" | jq -r "$filter"
else
  printf '%s' "$payload"
fi
STUB
chmod +x "$STUB_DIR/gh"
export PATH="$STUB_DIR:$PATH"

mk_fixture() {
  local dir="$1" approve_ts="$2" head_ts="$3" head_sha="$4"
  mkdir -p "$dir"
  cat >"$dir/pr-view.json" <<EOF
{
  "headRefOid": "$head_sha",
  "comments": [
    {"createdAt": "2026-06-20T10:00:00Z", "body": "nit: spacing"},
    {"createdAt": "$approve_ts", "body": "REVIEWER APPROVE — agent-id a0123456789abcdef"}
  ]
}
EOF
  cat >"$dir/commit.json" <<EOF
{"commit": {"author": {"date": "$head_ts"}}}
EOF
}

# Test 1: head committed AFTER approve → block (exit 1, stale-sha message)
FIX1="$(mktemp -d)"
mk_fixture "$FIX1" "2026-06-21T10:00:00Z" "2026-06-21T11:00:00Z" "deadbeef1"
GH_FIXTURE_DIR="$FIX1" run "blocks_stale_approve" 1 "approve-was-for-prior-sha" "$GUARD" 123

# Test 2: head committed BEFORE approve → pass (exit 0)
FIX2="$(mktemp -d)"
mk_fixture "$FIX2" "2026-06-21T12:00:00Z" "2026-06-21T11:00:00Z" "cafef00d2"
GH_FIXTURE_DIR="$FIX2" run "allows_fresh_approve" 0 "" "$GUARD" 124

# Test 3: equal timestamps → pass (commit at/before approve is fine)
FIX3="$(mktemp -d)"
mk_fixture "$FIX3" "2026-06-21T11:00:00Z" "2026-06-21T11:00:00Z" "1234abcd3"
GH_FIXTURE_DIR="$FIX3" run "allows_equal_timestamp" 0 "" "$GUARD" 125

# Test 4: no APPROVE comment at all → pass (nothing to invalidate; merge gate is elsewhere)
FIX4="$(mktemp -d)"
mkdir -p "$FIX4"
cat >"$FIX4/pr-view.json" <<'EOF'
{
  "headRefOid": "noappr0004",
  "comments": [
    {"createdAt": "2026-06-20T10:00:00Z", "body": "nit"}
  ]
}
EOF
cat >"$FIX4/commit.json" <<'EOF'
{"commit": {"author": {"date": "2026-06-21T11:00:00Z"}}}
EOF
GH_FIXTURE_DIR="$FIX4" run "no_approve_present_passes" 0 "" "$GUARD" 126

# Test 5: missing PR arg → exit 2 (usage)
run "usage_when_no_arg" 2 "usage" "$GUARD"

echo "---"
echo "PASS=$PASS FAIL=$FAIL"
[ "$FAIL" -eq 0 ]
