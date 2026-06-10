#!/usr/bin/env bash
source "$(dirname "$0")/_lib.sh"
smoke_log "attestation.Pool"
# Always-on (no live mode needed — fully hermetic)
go test -count=1 -run 'TestPool_' ./internal/attestation/... > /tmp/attest-smoke.log 2>&1 || {
  smoke_log "FAIL"; cat /tmp/attest-smoke.log; exit 1
}
smoke_log "PASS"
