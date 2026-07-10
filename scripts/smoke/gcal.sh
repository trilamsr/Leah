#!/usr/bin/env bash
source "$(dirname "$0")/_lib.sh"
smoke_log "gcal.ListToday"
mode=$(smoke_mode)
if [ "$mode" = "1" ]; then
  test -r ~/.leah-state/secrets/gcal-token.json || { smoke_log "no gcal token"; exit 1; }
  out=$(go run ./cmd/leah brief 2>&1 || true)
  echo "$out" | grep -qiE "today|event|nothing|unavailable" || { smoke_log "FAIL: $out"; exit 1; }
  smoke_log "PASS live"
  exit 0
fi
go test -count=1 -run 'TestListTodayTable' ./internal/actions/adapters/gcal/... > /tmp/gcal-smoke.log 2>&1 || {
  smoke_log "FAIL mock"; cat /tmp/gcal-smoke.log; exit 1
}
smoke_log "PASS mock"
