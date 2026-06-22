#!/usr/bin/env bash
# Stream Leah app + daemon logs.
# Usage: tail-logs.sh [--duration Ns]    --duration exits after N seconds (testing)
#        tail-logs.sh --file             tail /tmp/leah-dev.log instead of log stream
set -euo pipefail

DURATION=""
USE_FILE=0

while [ $# -gt 0 ]; do
  case "$1" in
    --duration) DURATION="$2"; shift 2 ;;
    --file)     USE_FILE=1; shift ;;
    *)          shift ;;
  esac
done

if [ "$USE_FILE" -eq 1 ]; then
  LOG="/tmp/leah-dev.log"
  [ -f "$LOG" ] || { echo "log not yet created: $LOG"; exit 0; }
  if [ -n "$DURATION" ]; then
    timeout "$DURATION" tail -F "$LOG" || true
  else
    tail -F "$LOG"
  fi
  exit 0
fi

PRED='subsystem == "com.maydow.leah" OR processImagePath ENDSWITH "leah-daemon"'

if [ -n "$DURATION" ]; then
  timeout "$DURATION" log stream --predicate "$PRED" --style syslog 2>/dev/null || true
else
  log stream --predicate "$PRED" --style syslog
fi
