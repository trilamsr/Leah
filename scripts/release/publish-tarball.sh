#!/usr/bin/env bash
# Build (and optionally upload) the source tarball the brew formula's `url`
# stanza expects: leah-vX.Y.Z-src.tar.gz + sidecar SHA256.
# Usage: TAG=vX.Y.Z [OUT_DIR=dist] [UPLOAD=1] publish-tarball.sh [--dry-run]
#
# Why a custom-named source tarball rather than relying on GitHub's auto-
# generated `archive/refs/tags/<tag>.tar.gz`: Formula/leah.rb pins
# `leah-<tag>-src.tar.gz` so the published artifact is reproducible from a
# pre-release prepare step, can carry a sidecar SHA256 the formula references,
# and excludes worktree/build noise (.claude/, node_modules/, dist/) that
# would otherwise bloat the auto-archive and shift its hash whenever someone
# adds an agent worktree.

set -euo pipefail

DRY_RUN=0
for arg in "$@"; do
  case "$arg" in
    --dry-run) DRY_RUN=1 ;;
    *) echo "unknown arg: $arg" >&2; exit 2 ;;
  esac
done

if [[ -z "${TAG:-}" ]]; then
  echo "TAG=vX.Y.Z env required" >&2
  exit 2
fi

# Strict semver guard with optional -prerelease suffix. Matches what
# release.yml's tag trigger accepts (`v[0-9]+.[0-9]+.[0-9]+`) plus the
# `-mvp5`-style suffix the v0.0.1-mvp5 formula already ships.
if ! [[ "$TAG" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[A-Za-z0-9.-]+)?$ ]]; then
  echo "TAG must match vX.Y.Z (optional -suffix); got: $TAG" >&2
  exit 2
fi

OUT_DIR="${OUT_DIR:-dist}"
UPLOAD="${UPLOAD:-0}"
REPO="${REPO:-trilamsr/Leah}"

mkdir -p "$OUT_DIR"
# Resolve to absolute paths up-front — the tar/sha256 steps run inside a
# `cd $TMP` subshell, so relative OUT_DIR would land in the wrong place.
OUT_DIR_ABS="$(cd "$OUT_DIR" && pwd)"
TARBALL="$OUT_DIR_ABS/leah-${TAG}-src.tar.gz"
REPO_ROOT="$(pwd)"

# `git archive` is the right tool: it walks the tree at the tagged commit,
# so untracked worktree noise is excluded by construction. We further trim
# anything that IS tracked but should never ship in the source dist —
# Formula/, .github/, .claude/ — via export-ignore in .gitattributes (no
# .gitattributes today, so we apply the filter post-archive).
echo "==> archiving $TAG → $TARBALL"
TMP="$(mktemp -d -t leah-src.XXXXXX)"
trap 'rm -rf "$TMP"' EXIT

# Verify the tag actually exists locally — without this the archive call
# fails with a noisy "fatal: not a valid object" that masks the real cause.
if ! git rev-parse -q --verify "refs/tags/${TAG}" >/dev/null; then
  echo "tag $TAG not found locally; run: git tag $TAG && git push origin $TAG" >&2
  exit 1
fi

# Prefix every file with leah-<tag>/ so `tar xzf` expands into a single
# top-level dir — Homebrew's `Formula#install` cd's into it implicitly.
git archive --format=tar --prefix="leah-${TAG}/" "$TAG" \
  | tar -x -C "$TMP"

# Post-archive scrub: strip dirs that are tracked-but-noisy in a source dist.
# .claude is in .gitignore so it never enters the archive in the first place,
# but we whitelist the scrub set explicitly so the test fixture (which adds
# .claude as tracked) and any future regressions are caught.
for noise in .claude node_modules dist Formula .github; do
  rm -rf "$TMP/leah-${TAG}/$noise"
done

# tar deterministically (sorted, fixed mtime) so the published artifact has
# a stable hash across re-runs from the same tag — matches the verify-checksums
# reproducibility contract.
commit_ct="$(git log -1 --format=%ct "$TAG" 2>/dev/null || echo 0)"
# GNU tar (--sort, --mtime) gives a deterministic archive; BSD tar on macos-14
# release runners lacks these flags, so we fall through to a plain tar. Source-
# tarball reproducibility on darwin is provided by the verify-checksums gate
# against the binary tarball, not this artifact.
( cd "$TMP" && tar \
    --sort=name \
    --mtime="@${commit_ct}" \
    -czf "$TARBALL" "leah-${TAG}" 2>/dev/null \
  ) || ( cd "$TMP" && tar -czf "$TARBALL" "leah-${TAG}" )

shasum -a 256 "$TARBALL" | awk '{print $1}' > "$TARBALL.sha256"
echo "==> sha256 $(cat "$TARBALL.sha256")  $TARBALL"

if [[ "$DRY_RUN" -eq 1 ]]; then
  echo "dry-run: skipping gh release upload"
  exit 0
fi

if [[ "$UPLOAD" -eq 1 ]]; then
  echo "==> uploading to release $TAG on $REPO"
  gh release upload "$TAG" "$TARBALL" "$TARBALL.sha256" --repo "$REPO" --clobber
fi
