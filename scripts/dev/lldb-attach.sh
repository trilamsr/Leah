#!/usr/bin/env bash
set -euo pipefail
# Attaches lldb to a running process by name. Prints process status; run further lldb commands manually.
# Usage: lldb-attach.sh <process-name>
if [ $# -lt 1 ]; then
  echo "usage: lldb-attach.sh <process-name>" >&2
  exit 1
fi
PID=$(pgrep -f "$1" | head -1)
[ -z "$PID" ] && { echo "no process matching $1"; exit 1; }
lldb -p "$PID" -o "process status"
