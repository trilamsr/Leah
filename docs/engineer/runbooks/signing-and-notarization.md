# Signing and notarization runbook

Covers Developer ID code-signing, Apple notarization, and stapling for distributing Leah.app outside the Mac App Store.

## Prerequisites

1. **Apple Developer Program membership** — enroll at https://developer.apple.com/programs/

2. **Developer ID Application certificate** — generate in Xcode:
   - Xcode → Settings → Accounts → select your Apple ID → Manage Certificates → + → Developer ID Application
   - The cert lands in your login Keychain automatically.

3. **notarytool credentials profile** — create once per machine:
   ```sh
   xcrun notarytool store-credentials leah-notary \
     --apple-id "$LEAH_APPLE_ID" \
     --team-id "$LEAH_TEAM_ID" \
     --password "$LEAH_APP_SPECIFIC_PASSWORD"
   ```
   - `LEAH_APPLE_ID`: your Apple ID email
   - `LEAH_TEAM_ID`: 10-char Team ID from https://developer.apple.com/account (top-right)
   - `LEAH_APP_SPECIFIC_PASSWORD`: generate at https://appleid.apple.com → App-Specific Passwords
   - `leah-notary` is the profile name; set `LEAH_KEYCHAIN_PROFILE=leah-notary` or any name you chose.

4. **Entitlements file** — `app/Leah/Sources/LeahApp/Leah.entitlements` must exist and declare `com.apple.security.cs.allow-jit` or other entitlements your app requires. Hardened Runtime (`--options runtime`) is required for notarization.

## Required env vars

| Variable | Purpose |
|---|---|
| `LEAH_IDENTITY` | Full codesign identity string — `"Developer ID Application: Maydow LLC (ABC123XYZ)"`. Find it: `security find-identity -v -p codesigning`. Preferred over constructing from `LEAH_TEAM_ID`. |
| `LEAH_TEAM_ID` | 10-char Apple Developer Team ID. Used as fallback identity (`"Developer ID Application: ${LEAH_TEAM_ID}"`) when `LEAH_IDENTITY` is unset; also required for `notarytool`. |
| `LEAH_APPLE_ID` | Apple ID email |
| `LEAH_APP_SPECIFIC_PASSWORD` | App-specific password for notarytool |
| `LEAH_KEYCHAIN_PROFILE` | Profile name passed to `notarytool --keychain-profile` |

Export these in your shell or a local `.env` file (never commit credentials).

> **Cert name format** — Apple Developer ID cert names always follow `"Developer ID Application: <Company Name> (<TEAM_ID>)"`. The Team ID alone (e.g. `"Developer ID Application: ABC123XYZ"`) is not a valid codesign identity and causes `codesign: No identity found`. Set `LEAH_IDENTITY` to the exact string shown by `security find-identity -v -p codesigning`.

## Invocation

```sh
# Build only (no signing):
make sign-and-notarize ARGS=--build-only

# Sign only (requires cert + env vars):
make sign-and-notarize ARGS=--sign

# Notarize only (app must already be signed):
make sign-and-notarize ARGS=--notarize

# Staple notarization ticket:
make sign-and-notarize ARGS=--staple

# Full release pipeline (build → sign → notarize → staple):
make sign-and-notarize ARGS=--all
```

Or invoke the script directly:
```sh
bash scripts/sign-and-notarize.sh --all
```

Override the default app path or entitlements path:
```sh
APP_PATH=/path/to/Leah.app ENTITLEMENTS=/path/to/Leah.entitlements \
  bash scripts/sign-and-notarize.sh --all
```

## What the script does

1. `--build-only` — runs `swift build -c release` in `app/Leah/`; produces `.build/release/Leah.app`.
2. `--sign` — `codesign --force --options runtime --timestamp --entitlements ... --sign "$LEAH_IDENTITY" Leah.app` where `LEAH_IDENTITY` defaults to `"Developer ID Application: ${LEAH_TEAM_ID}"` if unset.
3. `--notarize` — zips the app with `ditto`, submits to Apple notary via `xcrun notarytool submit --wait`.
4. `--staple` — `xcrun stapler staple Leah.app` attaches the notarization ticket so Gatekeeper passes offline.

## Troubleshooting

**`codesign: No identity found`** — either the cert is not in the login Keychain, or `LEAH_IDENTITY` does not match the cert's exact name. Run `security find-identity -v -p codesigning` and copy the full string (e.g. `"Developer ID Application: Maydow LLC (ABC123XYZ)"`) into `LEAH_IDENTITY`. Re-run the Xcode cert generation step if no Developer ID cert appears at all.

**`notarytool: invalid credentials`** — re-run `xcrun notarytool store-credentials` with correct values; confirm the profile name matches `LEAH_KEYCHAIN_PROFILE`.

**`notarytool: package invalid`** — ensure the app is signed before submitting; check `spctl -a -v Leah.app` output. Common cause: missing or incorrect entitlements.

**`stapler: ticket not found`** — notarization failed or was not awaited (`--wait` flag ensures this). Check notarytool output for the submission ID and retrieve logs: `xcrun notarytool log <submission-id> --keychain-profile leah-notary`.

**Gatekeeper rejects after distribution** — confirm staple succeeded (`xcrun stapler validate Leah.app`). Re-distribute the stapled copy.
