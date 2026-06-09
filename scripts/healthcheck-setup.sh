#!/usr/bin/env bash
#
# One-shot setup: register a healthchecks.io check + print the ping URL
# to export as LEAH_HEALTHCHECK_URL.
#
# Requires: HEALTHCHECKS_API_TOKEN (sign up at https://healthchecks.io)
#
# Usage: ./scripts/healthcheck-setup.sh

set -euo pipefail

if [ -z "${HEALTHCHECKS_API_TOKEN:-}" ]; then
  echo "HEALTHCHECKS_API_TOKEN not set" >&2
  echo "Sign up at https://healthchecks.io, create a project, copy the API key" >&2
  exit 1
fi

name="leah-$(hostname -s)"

response=$(curl -fsS https://healthchecks.io/api/v3/checks/ \
  -H "X-Api-Key: $HEALTHCHECKS_API_TOKEN" \
  -d "{\"name\":\"$name\",\"timeout\":600,\"grace\":120}")

ping_url=$(echo "$response" | grep -oE 'https://hc-ping\.com/[a-f0-9-]+')

if [ -z "$ping_url" ]; then
  echo "failed to extract ping URL from response:" >&2
  echo "$response" >&2
  exit 1
fi

echo "Healthcheck registered: $name"
echo ""
echo "Add to your shell rc:"
echo "  export LEAH_HEALTHCHECK_URL=$ping_url"
