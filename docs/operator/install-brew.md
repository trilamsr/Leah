# Install Leah on macOS

Two paths. Pick one. Both produce a binary Gatekeeper accepts without `xattr -d com.apple.quarantine`, without warnings, without disabling SIP.

## 1. Homebrew (recommended)

```sh
brew tap trilamsr/leah
brew install leah
```

Updates: `brew upgrade leah`.

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
