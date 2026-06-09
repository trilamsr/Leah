#!/usr/bin/env bash
# restic-init.sh — one-shot setup for leah's local + B2 restic repos.
#
# Prerequisites:
#   - brew install restic           (BSD-2, 0.19.0+)
#   - chmod 600 ~/.leah-restic-password   (operator pre-creates)
#   - export RESTIC_PASSWORD_FILE=~/.leah-restic-password
#
# Optional env (defaults shown):
#   LEAH_BACKUP_LOCAL_PATH=/Volumes/leah-backup
#   LEAH_BACKUP_B2_REPO=               (e.g. b2:my-bucket:leah-state)
#   B2_ACCOUNT_ID, B2_ACCOUNT_KEY      (only needed for B2 tier)
#
# Idempotent: "config file already exists" from `restic init` is treated
# as success.

set -euo pipefail

LOCAL="${LEAH_BACKUP_LOCAL_PATH:-/Volumes/leah-backup}"
B2="${LEAH_BACKUP_B2_REPO:-}"

if ! command -v restic >/dev/null 2>&1; then
  echo "restic-init: restic not on PATH — run 'brew install restic' first" >&2
  exit 1
fi

if [[ -z "${RESTIC_PASSWORD_FILE:-}" ]]; then
  echo "restic-init: RESTIC_PASSWORD_FILE not set" >&2
  echo "  create one with: openssl rand -hex 32 > ~/.leah-restic-password && chmod 600 ~/.leah-restic-password" >&2
  echo "  then: export RESTIC_PASSWORD_FILE=~/.leah-restic-password" >&2
  exit 1
fi

init_repo() {
  local repo="$1"
  echo "restic-init: initializing $repo"
  if restic -r "$repo" init 2>&1 | tee /tmp/restic-init.log; then
    return 0
  fi
  if grep -q "config file already exists" /tmp/restic-init.log; then
    echo "restic-init: $repo already initialized — ok"
    return 0
  fi
  echo "restic-init: FAILED to init $repo" >&2
  return 1
}

# Local tier — only init if the mount point exists.
if [[ -d "$LOCAL" ]]; then
  init_repo "$LOCAL"
else
  echo "restic-init: local mount $LOCAL not present — skipping local tier"
fi

# B2 tier — requires LEAH_BACKUP_B2_REPO + B2_ACCOUNT_ID + B2_ACCOUNT_KEY.
if [[ -n "$B2" ]]; then
  if [[ -z "${B2_ACCOUNT_ID:-}" || -z "${B2_ACCOUNT_KEY:-}" ]]; then
    echo "restic-init: B2 repo $B2 set but B2_ACCOUNT_ID/B2_ACCOUNT_KEY missing — skipping" >&2
  else
    init_repo "$B2"
  fi
else
  echo "restic-init: LEAH_BACKUP_B2_REPO unset — skipping B2 tier"
fi

echo "restic-init: done"
