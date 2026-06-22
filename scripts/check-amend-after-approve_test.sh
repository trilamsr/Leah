#!/usr/bin/env bash
# WHY: guard regression — APPROVE older than HEAD blocks; quoted/non-reviewer APPROVE
# must NOT count; mixed ISO-8601 zone formats must compare chronologically (not lexically).
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

# WHY: single $WORK root → one trap cleans everything (stub + all fixtures).
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT
STUB_DIR="$WORK/stub"
mkdir -p "$STUB_DIR"

cat >"$STUB_DIR/gh" <<'STUB'
#!/usr/bin/env bash
# WHY: mimic gh's -q (jq filter) and --json (passthrough) so the guard exercises real code.
sub="$1"; shift
filter=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    -q|--jq) filter="$2"; shift 2 ;;
    --json)  shift 2 ;;
    --repo)  shift 2 ;;
    *)       shift ;;
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

next_fix() { mktemp -d -p "$WORK" fix.XXXXXX; }

# WHY: default reviewer login matches CLAUDE.md canonical agent-id shape.
mk_fixture() {
  local dir="$1" approve_ts="$2" head_ts="$3" head_sha="$4"
  local approve_body="${5:-REVIEWER APPROVE - verdict clean}"
  local login="${6:-cavecrew-reviewer-test-a1}"
  jq -n --arg sha "$head_sha" --arg ats "$approve_ts" --arg body "$approve_body" --arg login "$login" '{
    headRefOid: $sha,
    comments: [
      {author: {login: "tri"},    createdAt: "2026-06-20T10:00:00Z", body: "nit: spacing"},
      {author: {login: $login},   createdAt: $ats,                    body: $body}
    ]
  }' >"$dir/pr-view.json"
  jq -n --arg ts "$head_ts" '{commit: {author: {date: $ts}}}' >"$dir/commit.json"
}

# T1: head committed AFTER approve → block
FIX="$(next_fix)"; mk_fixture "$FIX" "2026-06-21T10:00:00Z" "2026-06-21T11:00:00Z" "deadbeef1"
GH_FIXTURE_DIR="$FIX" run "blocks_stale_approve" 1 "approve-was-for-prior-sha" "$GUARD" 123

# T2: head committed BEFORE approve → pass
FIX="$(next_fix)"; mk_fixture "$FIX" "2026-06-21T12:00:00Z" "2026-06-21T11:00:00Z" "cafef00d2"
GH_FIXTURE_DIR="$FIX" run "allows_fresh_approve" 0 "" "$GUARD" 124

# T3: equal timestamps → pass
FIX="$(next_fix)"; mk_fixture "$FIX" "2026-06-21T11:00:00Z" "2026-06-21T11:00:00Z" "1234abcd3"
GH_FIXTURE_DIR="$FIX" run "allows_equal_timestamp" 0 "" "$GUARD" 125

# T4: no APPROVE → pass
FIX="$(next_fix)"
jq -n '{headRefOid: "noappr0004", comments: [{author: {login: "tri"}, createdAt: "2026-06-20T10:00:00Z", body: "nit"}]}' >"$FIX/pr-view.json"
jq -n '{commit: {author: {date: "2026-06-21T11:00:00Z"}}}' >"$FIX/commit.json"
GH_FIXTURE_DIR="$FIX" run "no_approve_present_passes" 0 "" "$GUARD" 126

# T5: usage
run "usage_when_no_arg" 2 "usage" "$GUARD"

# T6: non-reviewer login posting "REVIEWER APPROVE" → ignored. Even though head > comment-ts,
# guard treats as no APPROVE and exits 0.
FIX="$(next_fix)"; mk_fixture "$FIX" "2026-06-21T10:00:00Z" "2026-06-21T11:00:00Z" "selflogin6" "REVIEWER APPROVE - self" "tri"
GH_FIXTURE_DIR="$FIX" run "rejects_non_reviewer_login" 0 "" "$GUARD" 127

# T7: quoted "> REVIEWER APPROVE" → ignored
FIX="$(next_fix)"; mk_fixture "$FIX" "2026-06-21T10:00:00Z" "2026-06-21T11:00:00Z" "quoted0007" "> REVIEWER APPROVE - quoted prior" "cavecrew-reviewer-test-a1"
GH_FIXTURE_DIR="$FIX" run "rejects_quoted_approve" 0 "" "$GUARD" 128

# T8: mixed zone formats — approve uses +00:00 EARLIER wall-clock than head Z → block.
# Lexical raw-string compare would invert ('+' < 'Z'); epoch compare is correct.
FIX="$(next_fix)"; mk_fixture "$FIX" "2026-06-21T10:00:00+00:00" "2026-06-21T11:00:00Z" "isoformat8"
GH_FIXTURE_DIR="$FIX" run "blocks_stale_approve_mixed_zone_format" 1 "approve-was-for-prior-sha" "$GUARD" 129

# T9: mixed zone formats — approve LATER → pass
FIX="$(next_fix)"; mk_fixture "$FIX" "2026-06-21T12:00:00+00:00" "2026-06-21T11:00:00Z" "isofmt9pass"
GH_FIXTURE_DIR="$FIX" run "allows_fresh_approve_mixed_zone_format" 0 "" "$GUARD" 130

# T10: quoted bypass attempt — author writes "> REVIEWER APPROVE\n thx" AFTER head; the
# only VALID approve (older, from reviewer) predates head → block. The quoted comment must
# not refresh the timestamp.
FIX="$(next_fix)"
jq -n '{
  headRefOid: "twoappr010",
  comments: [
    {author: {login: "cavecrew-reviewer-test-a1"}, createdAt: "2026-06-21T10:00:00Z", body: "REVIEWER APPROVE"},
    {author: {login: "tri"},                       createdAt: "2026-06-21T12:00:00Z", body: "> REVIEWER APPROVE\nthx"}
  ]
}' >"$FIX/pr-view.json"
jq -n '{commit: {author: {date: "2026-06-21T11:00:00Z"}}}' >"$FIX/commit.json"
GH_FIXTURE_DIR="$FIX" run "quoted_does_not_refresh_valid_approve" 1 "approve-was-for-prior-sha" "$GUARD" 131

echo "---"
echo "PASS=$PASS FAIL=$FAIL"
[ "$FAIL" -eq 0 ]
