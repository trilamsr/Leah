#!/usr/bin/env bash
set -euo pipefail
# Launches a REPL with project context. Pass 'swift' or 'go'; default prints menu.
case "${1:-}" in
  swift)
    cd "$(git -C "$(dirname "$0")" rev-parse --show-toplevel)/app/Leah"
    exec swift repl
    ;;
  go)
    if command -v gore &>/dev/null; then
      exec gore
    else
      echo "gore not installed. Install via: go install github.com/x-motemen/gore/cmd/gore@latest"
      echo "Then run: gore"
      exit 1
    fi
    ;;
  *)
    echo "Usage: repl.sh [swift|go]"
    echo "  swift  — launch Swift REPL in app/Leah"
    echo "  go     — launch gore Go REPL (install: go install github.com/x-motemen/gore/cmd/gore@latest)"
    ;;
esac
