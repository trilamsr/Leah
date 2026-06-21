# Install Leah on macOS

Two paths. Pick one. Both produce a binary Gatekeeper accepts without `xattr -d com.apple.quarantine`, without warnings, without disabling SIP.

## 1. Homebrew (recommended)

```sh
brew tap trilamsr/leah
brew install leah
```

Updates: `brew upgrade leah`.

The tap repo (`trilamsr/homebrew-leah`) is out of band — it is a separate GitHub
repo, not this one. `Formula/leah.rb` here is the source of truth; copy it to that
tap repo's `Formula/leah.rb` at release time. Creating the tap repo is a one-time
operator step (`gh repo create trilamsr/homebrew-leah --public`).

## Rollback a bad upgrade

### A. Homebrew install

`brew switch` is removed — pin a specific version instead. From the tap, the
current formula is the only pinned point unless older formula revisions are kept,
so the reliable path is reinstall-from-source at a known tag:

```sh
brew uninstall leah
git clone --branch v0.0.1-mvp5 --depth 1 https://github.com/trilamsr/Leah ~/leah-rollback
cd ~/leah-rollback && make install
```

To stay on a known-good release and block surprise upgrades:

```sh
brew pin leah        # brew upgrade now skips leah until `brew unpin leah`
```

### B. Source / self-upgrade install

`scripts/upgrade.sh` keeps the previous build via a `leah-current` ↔ `leah-previous`
symlink pair. Roll back without rebuilding:

```sh
scripts/upgrade.sh rollback-symlink \
  ~/.leah-state/bin/leah-current ~/.leah-state/bin/leah-previous
scripts/upgrade.sh rollback-symlink \
  ~/.leah-state/bin/leah-daemon-current ~/.leah-state/bin/leah-daemon-previous
```

This swaps current ↔ previous atomically (two-step rename). Confirm with
`leah version`, then restart the daemon. Only one prior build is retained — a
second rollback returns you to the build you just left.

## 2. Direct download

Pick the latest `vX.Y.Z` from [GitHub Releases](https://github.com/trilamsr/Leah/releases):

```sh
TAG=vX.Y.Z
curl -LO "https://github.com/trilamsr/Leah/releases/download/$TAG/leah-$TAG-darwin-arm64.tar.gz"
curl -LO "https://github.com/trilamsr/Leah/releases/download/$TAG/SHA256SUMS"
shasum -a 256 -c SHA256SUMS
tar xzf "leah-$TAG-darwin-arm64.tar.gz"
mv leah leah-daemon leah-hud /usr/local/bin/
```

Always run `shasum -a 256 -c SHA256SUMS` before extracting. If it fails, do not run the binary — open an issue.

## Verify the binary is signed + notarized

```sh
spctl --assess --type execute -vv "$(command -v leah)"
```

Expected: `accepted source=Notarized Developer ID`.

## Two SHA256SUMS files — which to use when

Each release ships two checksum files:

- `SHA256SUMS` — covers the signed + stapled tarball you download above. Use this for normal install verification.
- `SHA256SUMS.unsigned` — covers a pre-signing tarball built from the same source tree. Use this for `make verify-checksums TAG=vX.Y.Z`, which rebuilds locally and confirms the GHA-published bytes match a clean rebuild.

The signed file's hash cannot be reproduced locally — `codesign` rewrites Mach-O bytes with a per-run timestamp, so signed binaries built from identical source diverge. The reproducibility gate fixes its checkpoint at the unsigned tarball; everything downstream of that (signing, notarization, stapling) is verified by `spctl` against Apple, not by hash.

## What notarization sends to Apple

Apple receives, per release:

- the binaries themselves — `notarytool` uploads each submission as a zip; Apple scans the executable bytes for known malware,
- the developer team ID,
- the submission timestamp.

Apple holds the uploaded binaries transiently to run the scan; no operator data outside the binary itself is transmitted. Once the submission completes, Apple issues a stapled ticket and the upload is no longer referenced at install time.

On every launch, macOS performs a ticket-validation lookup against Apple (Gatekeeper / `spctl`). This is standard macOS behavior, not Leah-introduced. Apple documents the endpoints at <https://support.apple.com/en-us/102445>.

If you want the local-build trust path (no remote-fetched binaries, no Apple notarization roundtrip), use `git clone` + `go build` per the developer install path instead.
