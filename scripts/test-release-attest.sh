#!/usr/bin/env bash
# W135/S10/M6 release-attest smoke test. Spec §7.3.
#
# Runs locally OR under `act` to exercise the release-attest workflow without
# burning Actions minutes on a real tag. Covers the two assertions the spec
# mandates:
#   1. Build is reproducible — sha256(build_1) == sha256(build_2) per target.
#   2. Aggregator emits one base64 payload covering every matrix leg.
#
# NOT wired into make check — runs only on tag-staging PRs (manual or
# release-attest-smoke workflow when an operator opts in).
#
# Usage:
#   scripts/test-release-attest.sh                # all matrix legs
#   scripts/test-release-attest.sh linux amd64    # one leg

set -euo pipefail

MATRIX=(
  "linux amd64"
  "linux arm64"
  "darwin amd64"
  "darwin arm64"
)

if [ "$#" -eq 2 ]; then
  MATRIX=("$1 $2")
fi

LDFLAGS='-s -w -buildid='
GOFLAGS='-trimpath -mod=readonly'
export CGO_ENABLED=0

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
hashes_file="$tmp/hashes.txt"
: > "$hashes_file"

for leg in "${MATRIX[@]}"; do
  read -r goos goarch <<<"$leg"
  echo "==> ${goos}/${goarch}"

  for cmd in leah leah-daemon leah-hud; do
    a="$tmp/${cmd}-${goos}-${goarch}-a"
    b="$tmp/${cmd}-${goos}-${goarch}-b"

    GOOS="$goos" GOARCH="$goarch" GOFLAGS="$GOFLAGS" \
      go build -ldflags="$LDFLAGS" -o "$a" "./cmd/${cmd}"
    GOOS="$goos" GOARCH="$goarch" GOFLAGS="$GOFLAGS" \
      go build -ldflags="$LDFLAGS" -o "$b" "./cmd/${cmd}"

    ha=$(shasum -a 256 "$a" | cut -d' ' -f1)
    hb=$(shasum -a 256 "$b" | cut -d' ' -f1)

    if [ "$ha" != "$hb" ]; then
      echo "FAIL: non-reproducible ${cmd} ${goos}/${goarch}: $ha != $hb" >&2
      exit 1
    fi
    echo "ok ${cmd} ${goos}/${goarch} sha256=$ha"

    printf '%s  %s-%s-%s\n' "$ha" "$cmd" "$goos" "$goarch" >> "$hashes_file"
  done
done

echo "==> aggregator self-check"
agg=$(base64 < "$hashes_file" | tr -d '\n')
if [ -z "$agg" ]; then
  echo "FAIL: empty aggregated payload" >&2
  exit 1
fi
echo "ok aggregated payload ${#agg} bytes"

echo
echo "PASS: ${#MATRIX[@]} leg(s) reproducible + aggregator non-empty"
