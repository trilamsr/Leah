#!/usr/bin/env bash
# V6/W91 placeholder-detection gate: catch `panic("not implemented")` /
# TODO-in-public-func smell in NEW feat/ branches. Honest "explicitly deferred"
# panics with a named sentinel error (see ErrUplinkNotShipped in
# internal/voice/listener/openai_realtime_decoder.go) pass.
set -euo pipefail

branch=$(git rev-parse --abbrev-ref HEAD)
case "$branch" in
  feat/*) ;;
  *) exit 0 ;;
esac

failed=0
# bash 3.2 (macOS default) has no mapfile, so stream via while-read.
while IFS= read -r f; do
  [ -z "$f" ] && continue
  [ -f "$f" ] || continue

  if grep -nE 'panic\("(not implemented|TODO|todo|unimplemented)"\)' "$f"; then
    echo "ERROR: $f contains panic with raw placeholder string" >&2
    echo "Fix: replace with named sentinel error (see ErrUplinkNotShipped pattern in internal/voice/listener/openai_realtime_decoder.go)" >&2
    failed=1
  fi

  if grep -nE '^func [A-Z][[:alnum:]_]+.*\{' "$f" | grep -qE 'TODO|FIXME|XXX'; then
    echo "ERROR: $f public function has TODO/FIXME/XXX placeholder" >&2
    failed=1
  fi
done < <(git diff --name-only origin/main...HEAD -- '*.go' | grep -vE '_test\.go$' || true)

exit $failed
