#!/usr/bin/env bash
source "$(dirname "$0")/_lib.sh"
smoke_log "gmail.ListUnread"
mode=$(smoke_mode)

if [ "$mode" = "1" ]; then
  # Live mode: requires real token at ~/.leah-state/secrets/gmail-token.json
  test -r ~/.leah-state/secrets/gmail-token.json || {
    smoke_log "no gmail token; install via 'leah connect gmail' first"; exit 1
  }
  out=$(go run ./cmd/leah ask "list 3 unread mail subjects" 2>&1 || true)
  echo "$out" | grep -qE 'subject|unread|no mail' || {
    smoke_log "FAIL: unexpected output: $out"; exit 1
  }
  smoke_log "PASS live"
  exit 0
fi

# Mock mode: run hermetic gmail-adapter test as a smoke proxy
go test -count=1 -run 'TestListUnread' ./internal/actions/adapters/gmail/... > /tmp/gmail-smoke.log 2>&1 || {
  smoke_log "FAIL mock"; cat /tmp/gmail-smoke.log; exit 1
}
smoke_log "PASS mock"
