# Release runbook — signed macOS builds

Engineer-side guide for the `release` workflow in `.github/workflows/release.yml`. The workflow fires on any `v[0-9]+.[0-9]+.[0-9]+` tag push and produces a signed, notarized, stapled `darwin-arm64` tarball plus two SHA256SUMS files.

## Required GitHub Actions secrets

All six must be set under `Settings → Secrets and variables → Actions` on the `trilamsr/Leah` repo before tagging a release. Missing any secret fails the run at the corresponding step.

| Secret                   | Source                                                                                                      | Used by step          |
|--------------------------|-------------------------------------------------------------------------------------------------------------|-----------------------|
| `APPLE_DEVID_P12`        | Base64 of the Developer ID Application `.p12` export: `base64 -i devid.p12 \| pbcopy`.                       | Import Developer ID cert |
| `APPLE_DEVID_PASSWORD`   | Password set when exporting the `.p12` from Keychain Access.                                                | Import Developer ID cert |
| `APPLE_SIGN_IDENTITY`    | Common Name of the cert as `codesign -s` expects it, e.g. `Developer ID Application: Tri Lam (TEAMID123)`.  | Codesign              |
| `APPLE_ID`               | Apple Developer account email.                                                                              | Notarize + staple     |
| `APPLE_APP_PASSWORD`     | App-specific password generated at <https://appleid.apple.com> → App-Specific Passwords. Not the AppleID password. | Notarize + staple     |
| `APPLE_TEAM_ID`          | 10-char Team ID from <https://developer.apple.com/account>.                                                 | Notarize + staple     |

## Cutting a release

```sh
git tag vX.Y.Z
git push origin vX.Y.Z
```

The workflow then:

1. Builds `leah`, `leah-daemon`, `leah-hud` with reproducible flags (`-trimpath`, `-mod=readonly`, `-buildid=`).
2. Tars + hashes the **unsigned** binaries → `SHA256SUMS.unsigned` (the reproducibility checkpoint).
3. Imports the `.p12` into a one-shot keychain, codesigns each binary with hardened runtime + secure timestamp.
4. Submits a single zip of all three binaries to `notarytool`, waits for `Accepted`, staples each.
5. Tars + hashes the **signed** binaries → `SHA256SUMS`.
6. Builds a **source tarball** (`leah-vX.Y.Z-src.tar.gz` + `.sha256` sidecar) via `scripts/release/publish-tarball.sh`. The brew formula's `url` stanza references this artifact — without it, `brew install leah` 404s.
7. Uploads tarballs + both SHA256SUMS files to the GitHub Release.

### Source tarball contract

`Formula/leah.rb` pins `url "https://github.com/trilamsr/Leah/releases/download/vX.Y.Z/leah-vX.Y.Z-src.tar.gz"`. The release workflow produces that exact filename plus a `.sha256` sidecar the formula's `sha256` stanza references.

Why a custom-named source tarball rather than GitHub's auto-generated `archive/refs/tags/<tag>.tar.gz`:

- the auto-archive's hash shifts whenever a tracked file is added (e.g. a new agent worktree marker), making the formula `sha256` stanza fragile;
- the published artifact excludes development noise (`.claude/`, `node_modules/`, `dist/`, `Formula/`, `.github/`) so the brew install path doesn't ship CI scripts to operators;
- the sidecar `.sha256` lets the formula bump be a one-line PR per release.

Dry-run the artifact locally before tagging:

```sh
TAG=vX.Y.Z scripts/release/publish-tarball.sh --dry-run
```

After publishing, bump `Formula/leah.rb` with the new `url`, `version`, and the sha256 from `dist/leah-vX.Y.Z-src.tar.gz.sha256`, then copy the formula into the `trilamsr/homebrew-leah` tap repo.

## Post-release verification

From any clean macOS clone of the repo:

```sh
make verify-checksums TAG=vX.Y.Z
```

This rebuilds from source and diffs the published `SHA256SUMS.unsigned` against the local rebuild. See `docs/operator/install-brew.md` for the operator-facing explanation of why a second checksum file exists.
