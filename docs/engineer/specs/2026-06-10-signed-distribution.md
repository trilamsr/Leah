# Wave-8 S12 — Signed + notarized macOS binary distribution

**Status:** Draft
**Date:** 2026-06-10
**Scope:** macOS only. GitHub-Actions-built, Apple-signed + notarized, EdDSA-signed Sparkle appcast, Homebrew tap delivery.
**Explicitly NOT:** App Store (sandbox kills cross-app SQLite), Linux/Windows packaging, hosted Sparkle update server, MDM enrollment, kernel extensions.

## 1. Goal

Ship a macOS binary the first non-developer operator can install without `xattr -d com.apple.quarantine`, without Gatekeeper warnings, without disabling SIP, and without cloning the repo. Two install paths:

1. `brew tap trilamsr/leah && brew install leah` — preferred.
2. Direct download from a GitHub Release `.tar.gz` — fallback.

Either path produces a binary whose `codesign --verify --strict --verbose=2` returns `valid on disk` and `spctl --assess --type execute -vv` returns `accepted source=Notarized Developer ID`. (`--deep` omitted — deprecated for flat Go binaries with no nested bundles.)

Unlocks the "first non-dev operator" threshold the wave-8 brief flags as bumping S12 above the Discord/WhatsApp adapter waves.

## 2. Non-goals

- **App Store**: sandbox prohibits the cross-app SQLite reads (`~/Library/Messages/chat.db`, `~/Library/Photos/Photos.sqlite`, etc.) that V22/V23/V27 depend on. Direct + brew is the only viable channel.
- **Linux / Windows**: deferred. macOS is the only V1 target per W26 mirror loop + V20–V29 macOS adapter constraints.
- **Sparkle update server hosting**: appcast XML is signed + uploaded as a static GitHub-Pages or release asset; no separate server. Auto-update *triggering* (background daemon polls appcast, calls existing `leah self-upgrade`) is deferred to a follow-up wave (W141.5 if scheduled).
- **Hardened-runtime exceptions**: Leah does **not** need `com.apple.security.cs.allow-unsigned-executable-memory`, `…allow-jit`, `…disable-library-validation`. The default `--options runtime` profile suffices because the Go binary loads no third-party dylibs (only `libSystem.dylib`, dynamically linked as Go always does on darwin) and runs no JIT.

## 3. Cost

| Line item | Cost | Notes |
| --- | --- | --- |
| Apple Developer ID | $99 / yr | Operator-paid. Single seat. |
| GHA macOS minutes | ~$0 *while repo stays public* | Free macOS runners are conditional on public-repo status. If `trilamsr/Leah` ever flips private, recurring cost adds ~$0.08/min × ~6 min/release. |
| Sparkle EdDSA keypair | $0 | Generated locally via `generate_keys` (Sparkle binary). |
| Engineering setup | ~2 dev-days | One-time. |

Total recurring: **$99/yr**. Crossed when operator count > 1, which the wave-8 brief asserts is imminent.

## 4. Wave plan W141–W143 (file-disjoint)

| Wave | Scope | Files (disjoint) |
| --- | --- | --- |
| W141 | GHA release workflow + codesign + notarize + staple + operator install doc | `.github/workflows/release.yml`, `scripts/release/notarize.sh`, `scripts/release/verify-signed.sh`, `scripts/release/import-cert.sh`, `docs/operator/install-brew.md` |
| W142 | Sparkle EdDSA appcast generator + signature verification + local-self-update amendment | `scripts/release/appcast.sh`, `scripts/release/sign-appcast.sh`, `scripts/release/sign-sha256sums.sh`, `internal/selfupgrade/verify_appcast.go`, `internal/selfupgrade/verify_appcast_test.go`, `docs/engineer/specs/2026-06-10-local-self-update.md` (amend §2) |
| W143 | Brew tap formula + tap-update automation | `scripts/release/update-brew-formula.sh` (tap repo `trilamsr/homebrew-leah/Formula/leah.rb` lives in a separate repo, no overlap) |

All three are file-disjoint and can land in parallel (CLAUDE.md "file-disjoint code PRs parallelize up to 6"). The spec itself serializes per the spec-PR rule. **No shared helpers across waves:** `scripts/release/` is partitioned by filename per the wave-table above — W141 owns `notarize.sh`/`verify-signed.sh`/`import-cert.sh`, W142 owns `appcast.sh`/`sign-appcast.sh`/`sign-sha256sums.sh`, W143 owns `update-brew-formula.sh`. No cross-wave function imports; duplicate three similar lines if needed (CLAUDE.md "three similar lines beat a premature abstraction").

## 5. W141 — GHA workflow + codesign + notarize

### 5.1 `.github/workflows/release.yml` (sketch)

Triggered on tag push matching `v[0-9]+.[0-9]+.[0-9]+`.

```yaml
jobs:
  release:
    runs-on: macos-14
    timeout-minutes: 30
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version-file: 'go.mod' }
      - name: Build (reproducible flags from S10 M6)
        env: { CGO_ENABLED: '0', GOFLAGS: '-trimpath -mod=readonly' }
        run: |
          for cmd in leah leah-daemon leah-hud; do
            go build -ldflags="-s -w -buildid=" -o dist/$cmd ./cmd/$cmd
          done
      - name: Import Developer ID cert
        env: { CERT_P12_BASE64: ${{ secrets.APPLE_DEVID_P12 }}, CERT_PASSWORD: ${{ secrets.APPLE_DEVID_PASSWORD }} }
        run: scripts/release/import-cert.sh
      - name: Codesign
        env: { SIGN_IDENTITY: ${{ secrets.APPLE_SIGN_IDENTITY }} }
        run: |
          for cmd in leah leah-daemon leah-hud; do
            codesign --options runtime --timestamp --sign "$SIGN_IDENTITY" dist/$cmd
            codesign --verify --strict --verbose=2 dist/$cmd
          done
      - name: Notarize + staple
        env: { APPLE_ID: ${{ secrets.APPLE_ID }}, APPLE_PASSWORD: ${{ secrets.APPLE_APP_PASSWORD }}, TEAM_ID: ${{ secrets.APPLE_TEAM_ID }} }
        run: scripts/release/notarize.sh dist/leah dist/leah-daemon dist/leah-hud
      - name: Tarball + SHA256 + Sparkle EdDSA sign
        env: { SPARKLE_ED_PRIVATE_KEY: ${{ secrets.SPARKLE_ED_PRIVATE_KEY }} }
        run: |
          cd dist && tar czf leah-${GITHUB_REF_NAME}-darwin-arm64.tar.gz leah leah-daemon leah-hud
          shasum -a 256 *.tar.gz > SHA256SUMS
          ../scripts/release/sign-sha256sums.sh SHA256SUMS > SHA256SUMS.sig
      - name: Upload to Release
        uses: softprops/action-gh-release@v2
        with:
          files: |
            dist/*.tar.gz
            dist/SHA256SUMS
            dist/SHA256SUMS.sig
```

### 5.2 `scripts/release/notarize.sh`

1. `ditto -c -k --keepParent dist dist/leah-notarize.zip` — bundles `leah`, `leah-daemon`, `leah-hud` into a single container (notarytool requires a container, and all three binaries must be submitted together for the single staple pass below).
2. `xcrun notarytool submit dist/leah-notarize.zip --apple-id "$APPLE_ID" --password "$APPLE_PASSWORD" --team-id "$TEAM_ID" --wait` → fails the workflow if `status != Accepted`.
3. `xcrun stapler staple dist/leah dist/leah-daemon dist/leah-hud` — stapler is invoked per-binary on the original files; the zip from step 1 is discarded.
4. `spctl --assess --type execute -vv dist/leah dist/leah-daemon dist/leah-hud` final sanity check (expects `source=Notarized Developer ID` for each).

### 5.3 Hardened-runtime entitlements

None required (see §2 non-goals). The signed binary uses the default hardened-runtime profile. If a future feature requires JIT (e.g. local LLM via on-device V8/WASM), this section is the diff point — add a minimal `entitlements.plist` with only the needed exceptions.

### 5.4 Tests (W141)

- `actionlint .github/workflows/release.yml` clean (CI-side lint, no Apple secrets touched).
- `scripts/release/verify-signed.sh dist/leah` — exits 0 iff `codesign --verify --strict` + `spctl --assess --type execute` + `stapler validate` all pass. Run on a manual `workflow_dispatch` against the last tag for smoke regression.
- Manual: first tag pushed (`v0.0.1-signtest`) actually downloads + opens on a clean macOS VM with no quarantine override required.

## 6. W142 — Sparkle EdDSA appcast

### 6.1 Why a separate signature

Apple's notarization signs *the binary* against Apple's CA. The Sparkle appcast (XML feed of update versions) is **not** notarized — Sparkle defends update integrity with its own EdDSA signature, validated by the EdDSA public key compiled into the running Leah binary. Notarization + appcast EdDSA are independent trust paths and both required for auto-update to be safe.

### 6.2 Key handling

- Generate once: `./Sparkle/bin/generate_keys` → writes `Sparkle-private-key` to operator keychain, prints public key.
- Public key compiled into Leah via `-ldflags "-X internal/selfupgrade.appcastPubkey=<base64>"`.
- Private key stored as GHA secret `SPARKLE_ED_PRIVATE_KEY`. Never logged, never echoed — secrets used outside `secrets.*` interpolation pass through `echo "::add-mask::$VALUE"` on first shell read.
- Key rotation procedure: documented in spec §13. New pubkey ships in a Leah release; old pubkey accepted for one transitional-release window; then removed.

### 6.3 `internal/selfupgrade/verify_appcast.go`

```go
func VerifyAppcast(xml []byte, sig []byte, pubkey ed25519.PublicKey) error {
    if !ed25519.Verify(pubkey, xml, sig) {
        return ErrAppcastSigInvalid
    }
    return nil
}
```

Wired into the existing `self-upgrade` CLI from `2026-06-10-local-self-update.md` §6: when invoked with `--from-appcast <url>`, Leah fetches the XML + `.sig`, calls `VerifyAppcast`, then downloads the notarized tarball at the validated URL and verifies its SHA256 before atomic swap. Local-build self-upgrade (no appcast, local `go build` only) remains the default and unchanged.

**Trust-boundary change vs `2026-06-10-local-self-update.md`:** that spec asserts "operator's own clone + go toolchain — NO remote-fetched binaries." Appcast mode is the first sanctioned remote-binary path. W142 MUST land a paired amendment to `2026-06-10-local-self-update.md` §2 (non-goals) re-scoping that constraint to "default path only; appcast path is opt-in via `--from-appcast`, gated on EdDSA pubkey baked into the running binary." Without the paired amendment, the two specs disagree on the operator's threat model.

### 6.4 Tests (W142)

- `TestVerifyAppcast_GoodSig`, `TestVerifyAppcast_TamperedXML`, `TestVerifyAppcast_WrongKey`, `TestVerifyAppcast_TruncatedSig`. Fixtures generated from a test keypair checked in under `testdata/`. The production pubkey is **not** in the test set.
- TDD: W142 splits into **W142a** (failing tests + signature-only scaffolding for `verify_appcast.go`) and **W142b** (implementation that flips the tests green + appcast script wiring + paired amendment to `2026-06-10-local-self-update.md`). Two sequential PRs, no parallel landing.

## 7. W143 — Brew tap

### 7.1 Tap repo

Separate GitHub repo `trilamsr/homebrew-leah` (Homebrew convention: tap repos must be named `homebrew-<tap>`). Contains a single formula `Formula/leah.rb`:

```ruby
class Leah < Formula
  desc "Personal chief-of-staff for macOS"
  homepage "https://github.com/trilamsr/Leah"
  version "X.Y.Z"
  url "https://github.com/trilamsr/Leah/releases/download/vX.Y.Z/leah-vX.Y.Z-darwin-arm64.tar.gz"
  sha256 "<sha from release SHA256SUMS>"
  def install
    bin.install "leah", "leah-daemon", "leah-hud"
  end
  test do
    assert_match "leah", shell_output("#{bin}/leah --version")
  end
end
```

### 7.2 `scripts/release/update-brew-formula.sh`

Called as the last step of `release.yml`: clones the tap repo, rewrites `version`, `url`, `sha256`, opens a PR (using `${{ secrets.TAP_PUSH_TOKEN }}`). Operator merges manually — the human gate is intentional, because a compromised release.yml that overwrote the tap formula could otherwise reach every brew user.

### 7.3 Tests (W143)

- `brew install --build-from-source ./Formula/leah.rb` against a tagged release on a clean macOS VM.
- `brew audit --strict --online leah` clean.
- Manual smoke: `brew install leah && leah --version && spctl --assess --type execute -vv $(brew --prefix)/bin/leah` returns Notarized Developer ID.

## 8. Reproducible build cross-link (S10 M6)

`release.yml` reuses the build flags S10 M6 establishes (`CGO_ENABLED=0`, `-trimpath`, `-mod=readonly`, `-buildid=`, `-s -w`). No build-time `-X` injections — version + commit are derived at runtime from the embedded `go.mod` + `runtime/debug.ReadBuildInfo()` (preserved by `-trimpath`). Two operators independently checking out the same tag and running `go build` with these flags produce a byte-identical binary; appending Apple's signature is the only divergence between the unsigned local build and the signed release artifact. The S10 M6 `make verify-checksums` target lets a wary operator independently rebuild from source and compare `shasum -a 256` against the published `SHA256SUMS` *before signing* to detect a malicious GHA.

## 9. Privacy

Notarization sends Apple a SHA256 of every binary submitted, plus the developer team ID, plus a timestamp. It does **not** upload binary contents to Apple — `notarytool` uploads to Apple's ingest server, but the cryptographic check is on the hash + signature, not the executable bytes. This MUST be documented in `docs/operator/install-brew.md` (W141 scope) so operators understand the trust delta vs the local-build path.

`spctl` and Gatekeeper, on each operator's machine, perform an online ticket-validation check against Apple (CRL + revocation list lookup). This is a standard macOS behavior, not a Leah-introduced surveillance vector, but the install docs link to Apple's documented endpoints.

## 10. Operator override

`LEAH_DEV_BUILD=1` env var on the running daemon **disables appcast signature verification only.** A `WARN` line is emitted on every daemon start while the flag is set, and an audit row `Kind: "leah_dev_build_mode", reason: "explicit env"` is recorded. The flag deliberately does NOT gate startup on the prior upgrade method — that would refuse `brew install` (a valid dev path) and is the dev scenario this flag exists to support. Loudness, not refusal, is the safety property.

## 11. Failure modes

| Failure | Detection | Recovery |
| --- | --- | --- |
| Notarization rejected | `notarytool submit --wait` exits non-zero | `scripts/release/notarize.sh` greps `notarytool log $SUBMISSION_ID` and prints the issues; release.yml fails loudly. Operator fixes (typically a hardened-runtime entitlement mismatch or unsigned helper) and re-pushes the tag. No partial-release state: the GHA workflow is all-or-nothing because the upload-to-release step runs only after staple success. |
| Apple cert expired | `codesign --sign` exits non-zero | Operator regenerates Developer ID; rotates `APPLE_DEVID_P12` secret; re-runs workflow. |
| Sparkle private key compromise | Reported externally (operator-facing) | Rotate per §6.2: new release ships new pubkey + accepts old for one transitional-release window, then removes. |
| Tap formula push token revoked | brew users see stale version | Manual `update-brew-formula.sh` from local cwd until a new `TAP_PUSH_TOKEN` is provisioned. Direct-download path is unaffected. |
| `notarytool` outage at Apple | Workflow hangs at `--wait` | `--wait` has a built-in ~2h ceiling. CI job timeout of 30min in release.yml causes a fail-fast retry path; the tag is not re-cut, the workflow is re-run from the GHA UI. |

## 12. Test plan summary

W141: actionlint + verify-signed.sh + manual notarize-on-first-tag.
W142: 4 Go unit tests + TDD (failing tests first PR).
W143: `brew audit` + brew-install-on-clean-VM manual smoke.

Cross-cutting: every release MUST produce a `SHA256SUMS` file alongside the tarball, and a `SHA256SUMS.sig` (Sparkle EdDSA) covering it. Operators are expected to verify `shasum -a 256 -c SHA256SUMS` before extracting.

## 13. Key rotation procedure

Documented inline rather than scattered across runbooks:

1. Generate new keypair: `./Sparkle/bin/generate_keys --new`.
2. Add new pubkey to `internal/selfupgrade/appcast_keys.go` as `pubkeyV2`; keep `pubkeyV1` in the slice.
3. Cut release N: signature verified against either V1 or V2.
4. Cut release N+1: `pubkeyV1` removed; only V2 accepted.
5. Operators who skipped release N see a clear `ErrAppcastUnknownKey` and are directed to the direct-download fallback for that one release.

The transitional-release window (release N above) is mandatory — never delete a key without that one-release overlap, or operators on slow update cadences are locked out.

## 14. Open questions

- Direct vs DMG distribution: tarball chosen here for atomic-swap parity with the local-build path. A `.dmg` is operator-friendlier on first install but adds a notarize-the-DMG step. Deferred — revisit when `brew install` insufficiently covers the operator install path.
- ARM64 vs Universal2: V1 ships arm64 only — release.yml hardcodes `darwin-arm64` in the tarball filename (§5.1) and `bin.install` in the brew formula (§7.1). x86_64 added only if an operator surfaces; conversion is a `lipo` step in release.yml plus a `Hardware::CPU.intel?` branch in the formula. Flagged here so the assumption is explicit, not implicit.
- Helper-binary signing: `leah-hud` may grow Wails-bundled web assets per `2026-06-10-wails-decision.md`. If Wails is adopted post-v3, the `.app` bundle itself becomes the signing target, not the bare binary — at that point this spec gets a follow-up amendment (new section appended to §14). Until then, three flat signed binaries is the surface area.
