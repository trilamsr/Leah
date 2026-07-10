# Key Custody Research — Anthropic API + Sparkle EdDSA

**Date:** 2026-06-21
**Context:** Leah is a single-operator macOS personal-use product. Owner = user. No constrained-beta. Optimize for: best UX + cost-sensible.

---

## Part A — Anthropic API key UX

### A1. BYOK vs embedded vs subscription — recommendation

**Win for personal-use Leah: BYOK (user provides their own Anthropic key).**

| Model | Cost to author | Friction | Infra needed | Best fit |
|---|---|---|---|---|
| BYOK | $0 | High (account + key paste) | None | **Personal-use Leah ✅** |
| Embedded org-key | $$$/user (unbounded) | Zero | Proxy + rate-limit + per-user quota | Constrained beta only |
| Subscription (Leah Plus) | Net-positive at scale | Low | Stripe + auth + proxy + entitlement | Deferred multi-user |

Reasoning:
- Leah owner = the operator. The operator already has an Anthropic key (it's wired today in `internal/reasoner/anthropic.go` reading `ANTHROPIC_API_KEY`).
- Embedded org-key burns the author's money on every user's runtime; with Opus pricing at $15/M-in $75/M-out and a research bundle easily eating 50K tokens, a single curious user could rack $5–$50/day.
- A subscription tier requires Stripe + JWT + a proxy fleet — none of which is on the personal-use ship path.
- BYOK matches incumbents in the indie-mac segment: Raycast Pro explicitly says "you can BYOK - we currently support custom keys for OpenAI, Anthropic and Google" — even at a paid tier they let users opt into their own key (Raycast docs, 2026).

**Forward path:** start BYOK-only. Add Leah Plus subscription later if/when distribution warrants — the wizard step (A3) is designed to allow that addition without breaking existing users.

### A2. First-launch UX patterns — incumbent map

| Product | Pattern | Auth substrate | Key custody |
|---|---|---|---|
| Linear desktop | SSO (Google/email magic-link) | Linear backend | Server-side, opaque to user |
| Granola | Email magic-link | Granola backend | Granola holds their own LLM keys (subsidized) |
| Cursor | Sign-in + their key (with BYOK opt-in) | Cursor backend | Server-proxied; BYOK stored client-side |
| ChatGPT desktop | OAuth → OpenAI account | OpenAI account | Tied to ChatGPT subscription, not API |
| Raycast Pro | Subscription + their key (BYOK opt-in) | Raycast backend | Server-proxied; BYOK stored locally |

**Anthropic Console OAuth:** Anthropic Console (platform.claude.com) does NOT publicly expose OAuth-as-a-third-party-IdP for arbitrary apps to "Sign in with Anthropic" the way Google/GitHub do. The Admin API lets you programmatically manage keys (`/v1/organizations/api_keys`) but requires an admin key as auth — that doesn't help first-launch. **Verdict: no "Sign in with Anthropic" flow exists in 2026.** Treat key-paste as the only option until Anthropic ships OAuth.

**Map for personal-Leah (now):** paste-key wizard, matching the Raycast BYOK shape.
**Map for deferred multi-user-Leah:** email magic-link (Granola pattern) + optional BYOK toggle (Raycast pattern). Skip until subscription is on the roadmap.

### A3. Wizard step copy + flow

**Step 1 — Welcome:**
```
Welcome to Leah.
Leah uses Claude (by Anthropic) as its reasoning engine.
You'll need an Anthropic API key to continue.
[ I have a key ]   [ Get one (opens browser) ]
```

The "Get one" button opens `https://console.anthropic.com/settings/keys` in the default browser. No deep-link — Anthropic doesn't expose one.

**Step 2 — Paste:**
```
Paste your Anthropic API key
[ sk-ant-...                                      ]  [paste from clipboard]

Leah stores this in your macOS Keychain.
It never leaves your Mac except to call api.anthropic.com.
[ Verify & continue ]
```

On click: POST `https://api.anthropic.com/v1/messages` with a 1-token "ping" (`max_tokens: 1`, model `claude-haiku-4-5`). Confirms the key works and shows the user a tiny success before commit.

**Step 3 — Confirm:**
```
✓ Key verified (Workspace: Personal • Org: Acme Inc)
Spending limits and rate limits are controlled in your Anthropic Console.
[ Continue ]
```

Pull org+workspace name from the verify response header (`anthropic-organization-id`) for trust signal.

**Why not the option-rich "BYOK or Plus" copy:** premature. Adds a decision the user can't yet evaluate. Ship BYOK-only, add Plus as a toggle in Settings later — by then there will be feature differentiation to justify the choice.

### A4. Storage — macOS Keychain Services

**Recommended: `kSecClassGenericPassword` in the user's login Keychain**, accessed via the `Security.framework` C API (or a Go cgo wrapper like `keybase/go-keychain`).

| Option | Security | UX | Verdict |
|---|---|---|---|
| **macOS Keychain** | OS-grade, hardware-backed on T2/M-series, biometric-gated optional | "Allow once / Always" prompt first time | **Ship this** |
| `~/Library/Application Support/Leah/secrets.json` | File-perm only; readable by any process running as user | Zero UX | No — fails minimum bar |
| Env var (`ANTHROPIC_API_KEY`) | Bad — leaks into subprocess env, shell history | Bad for non-dev | Keep as escape-hatch for dev only |

**Schema:**
- Service: `com.maydow.leah.anthropic`
- Account: `default` (room for `staging`, `personal-2`, etc. later)
- Generic data: the key bytes
- Access control: `kSecAttrAccessibleWhenUnlocked` (no need for `AfterFirstUnlock` since Leah is a foreground tool, not a launchd daemon mid-boot)

**Backward-compat:** if `ANTHROPIC_API_KEY` env is set, prefer it (zero-friction for the operator's current dev loop). Wizard only runs when both env and Keychain are empty.

### A5. Key rotation flow

```
Settings → API Keys → Anthropic
─────────────────────────────────
Current key: sk-ant-...XYZ  (verified 2026-06-21)
Org: Acme Inc  •  Workspace: Personal
[ Test connection ]   [ Replace key ]   [ Remove ]
```

**Replace key** flow:
1. Modal: "Paste new key" (same field as Step 2).
2. Verify-call against new key BEFORE committing (1-token ping).
3. On success: atomically swap in Keychain. Show "Old key was kept for 60s in case you need to undo." Actually drop it after 60s — gives undo without persistent stale-key risk.
4. On failure: error + keep old key intact. Never leave the user keyless.
5. After commit: emit an `obs` event `auth.anthropic_key_rotated` so the audit-session skill can surface it.

**Edge case:** if the verify-call returns 401 the new key is bad — refuse and keep old. If it returns 429 (rate-limited) the new key is valid but the org is throttled — commit anyway and warn.

---

## Part B — Sparkle EdDSA key custody

### B1. Sparkle's built-in storage (verified from sparkle-project.org/documentation 2026)

> "It will generate a private key and **save it in your login Keychain on your Mac**. You don't need to do anything with it, but do keep it safe… You can use the `-x private-key-file` and `-f private-key-file` options to export and import the keys respectively when transferring keys to another Mac."

So Sparkle's default = same store we're already recommending for the Anthropic key. Good alignment.

### B2. Single-developer best practice

**Layered custody (highest leverage, lowest infra):**

1. **Primary:** Sparkle's default — private key lives in dev-Mac login Keychain (generated by `./bin/generate_keys`).
2. **Encrypted backup #1:** export with `generate_keys -x ~/leah-eddsa-private.pem`, store in **1Password** under a vault item titled `Leah Sparkle EdDSA — private`. 1Password syncs across all your devices and survives Mac wipe.
3. **Encrypted backup #2 (cold):** print the base64 of the exported key on paper, store in a physical safe. EdDSA private keys are ~64 bytes base64-encoded — fits on an index card. This is the "house burns down + 1Password compromised" recovery path.
4. **Public key:** committed to repo in `Info.plist` as `SUPublicEDKey`. Not a secret; safe to be public.

Avoid for single-dev:
- **YubiKey:** Sparkle's `sign_update` reads the key from Keychain or `-f file`; there's no PKCS#11 / hardware-token signing path. Wiring YubiKey in would mean wrapping the export → sign → wipe dance manually, which loses more reliability than it gains for one-person ops.
- **git-crypt:** repo lock-in; if you ever lose the gpg key the encrypted-in-repo copy is dead. 1Password is more recoverable.
- **AWS/Vercel Secrets Manager:** unjustified infra for a one-key, low-rotation workload. Adds an outage-domain (cloud provider down = can't ship).

### B3. Multi-machine signing (laptop + desktop)

Two viable patterns:

**Pattern A — sync via 1Password (recommended for single-dev):**
- Use 1Password CLI: `op read "op://Private/Leah Sparkle EdDSA/private" > /tmp/leah-eddsa.pem`
- Run `./bin/sign_update -f /tmp/leah-eddsa.pem MyApp-1.2.3.zip`
- `rm /tmp/leah-eddsa.pem` (or use `op run --env-file=...` so it never hits disk).
- Pro: same key on both machines, recoverable from 1Password on a fresh Mac.
- Con: key briefly in tmp; mitigate with `op run` env-injection.

**Pattern B — sign in CI (GitHub Actions):**
- Store the private key in GitHub Actions Secrets (`SPARKLE_EDDSA_PRIVATE_KEY`).
- CI job: `echo "$SPARKLE_EDDSA_PRIVATE_KEY" | ./bin/sign_update -f /dev/stdin dist/Leah-x.y.z.zip` then publishes to GitHub Releases.
- Pro: signing-key never lives on dev machines after initial provisioning; tagged release auto-signs.
- Con: GitHub gets read-access to your signing key (their secret-store is the trust boundary); a GitHub account compromise = update channel compromise.

**Recommendation: Pattern A now.** Pattern B is the right migration when (a) you have more than one releaser, or (b) releases are frequent enough that manual signing is the bottleneck. Neither is true for personal-use Leah today.

### B4. Backup + recovery

The catastrophic failure mode: **lost EdDSA private key** = every existing user's app refuses all future updates until they manually install a new build with a new `SUPublicEDKey` (which they must download out-of-band — Sparkle won't pull it).

**Mitigation tiers (do all three):**

1. **1Password vault item** (synced to all your devices + 1Password's cloud).
2. **Paper backup** in physical safe (offline; immune to 1Password account loss).
3. **Encrypted local file** on Time Machine backup (`~/leah-sparkle-backup.age` using `age -p` symmetric encryption with a high-entropy passphrase you also keep in 1Password).

**Rotation when key is suspected leaked:**
Sparkle 2 supports rotation: "if you both code-sign your application with Apple's Developer ID program and include a public EdDSA key for signing your update archive, Sparkle allows rotating keys by issuing a new update that changes either your Apple code signing certificate or your EdDSA keys (but not both)." So Developer ID + EdDSA is a load-bearing combo — **don't skip Developer ID notarization** even if Sparkle technically works without it. It's your second key (Apple's) that lets you rotate the first (EdDSA) without bricking users.

### B5. Appcast hosting

**Recommended: GitHub Releases + appcast.xml in the repo's `gh-pages` branch (or just a `docs/appcast.xml` file served via GitHub Pages).**

| Option | Cost | Setup | Verdict |
|---|---|---|---|
| **GitHub Releases + Pages appcast.xml** | $0 | 10 min | **Ship this** |
| Cloudflare R2 / S3 | ~$0–$1/mo | 1 hr | Overkill until traction |
| Vercel static | $0 (hobby) | 30 min | Fine but adds a vendor |

**Why GitHub Pages for the appcast specifically:**
- Sparkle 2.9 added appcast feed signing (`sparkle:hardwareRequirements`, signed feeds) — but GitHub Pages serves over HTTPS already, and EdDSA signs the binary itself, so feed-tampering only lets an attacker hide updates, not forge one.
- GitHub Releases gives free CDN-backed binary hosting with no bandwidth caps for OSS-style use.
- Single URL: `https://maydow.github.io/leah/appcast.xml`. Embed in `Info.plist` as `SUFeedURL`.

**Sparkle delta updates:** Sparkle supports binary delta updates that patch only changed files. **Skip for v1.** Generates an extra `.delta` artifact per release; appcast must list both full and delta enclosures. Worth it only when Leah's binary is large (>50 MB) and update frequency is high (>1/week). For a Go binary under 30 MB updated weekly-ish, full downloads are fine.

### B6. Update channels

**Single channel (stable only) for v1.** Skip beta infrastructure entirely.

Sparkle 2 supports channels (`sparkle:channel` element in the appcast) for beta/dev tracks, but for a one-operator product:
- The operator IS the beta tester (running from `make dev`).
- Friends/family on prod can take stable releases.
- Adding beta requires a UI toggle, channel-switcher in Settings, and at minimum a second appcast.xml.

When to add: when you have ≥10 external testers AND ≥1 risky release/quarter. Until then, the cost > benefit.

---

## Final recommendation (2 paragraphs)

**For the Anthropic API key:** ship BYOK with a three-step paste-verify-confirm wizard, storing the key in the user's macOS login Keychain under service `com.maydow.leah.anthropic`. Honor `ANTHROPIC_API_KEY` env when set so the operator's existing dev loop keeps working. Skip "Sign in with Anthropic" — Anthropic Console does not expose third-party OAuth in 2026 — and skip the option-rich "BYOK vs Plus" copy because there is no Plus tier yet. Build the rotation flow now (Settings → Replace key → verify-then-swap with a 60-second undo window) so it's there when the operator inevitably regenerates a key. The whole feature costs $0 of recurring infra and ~1 dev-day of work.

**For the Sparkle EdDSA key:** let Sparkle's default put the private key in the dev-Mac login Keychain, then layer three independent backups — 1Password vault item (synced + recoverable), encrypted Time Machine backup, paper copy in a physical safe. Sign from dev machines via `op read` injection rather than persisting the key on multiple Macs; defer GitHub Actions signing until release cadence makes manual signing painful. Host the appcast on GitHub Pages and the binaries on GitHub Releases — free, HTTPS, CDN-backed. Don't skip Apple Developer ID notarization even though Sparkle technically works without it: Developer ID is the second-key insurance policy that lets you rotate the EdDSA key if it ever leaks. Skip delta updates, beta channels, and signed appcasts until distribution actually demands them.

---

## Sources (2026 fetches)

- sparkle-project.org/documentation — EdDSA setup, key rotation, Keychain default
- github.com/sparkle-project/Sparkle releases (2.8.0, 2.9.0) — current channel/delta/feed-signing capabilities
- github.com/sparkle-project/Sparkle README 2.x — feature matrix (channels, delta, phased rollout)
- developer.apple.com/documentation/security/keychain_services — Keychain Services API
- developer.1password.com/docs/cli/secret-references — `op://vault/item/field` syntax + `op read`/`op run`
- developer.1password.com/docs/cli/secrets-reference-syntax — secret-reference URI format
- docs.anthropic.com/en/api/getting-started — Console URL, workspace/key model
- docs.anthropic.com/en/docs/about-claude/pricing — current model pricing for BYOK cost math
- raycast.com/pro — incumbent BYOK pattern reference ("BYOK… OpenAI, Anthropic and Google")
- granola.ai/security — incumbent backend-key-custody pattern reference
- help.openai.com ChatGPT mac FAQ — incumbent OAuth-to-account pattern reference

> All internal paths in this doc reflect the pre-2026-07-09 layout; current tree per `git ls-tree -d --name-only HEAD:internal/`.
