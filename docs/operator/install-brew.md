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

## What notarization tells Apple

Apple receives, per release:

- the SHA256 of each binary submitted,
- the developer team ID,
- the submission timestamp.

Apple does **not** receive the binary contents. The notarization check is on hash + signature, not the executable bytes themselves.

On every launch, macOS performs a ticket-validation lookup against Apple (Gatekeeper / `spctl`). This is standard macOS behavior, not Leah-introduced. Apple documents the endpoints at <https://support.apple.com/en-us/HT202491>.

If you want the local-build trust path (no remote-fetched binaries, no Apple notarization roundtrip), use `git clone` + `go build` per the developer install path instead.
