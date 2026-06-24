# Leah Phase 5 — Distribution + ambient awareness layer

**Version:** v1.0 (2026-06-23). Authoritative for Phase 5 dispatch.
**Predecessor:** `docs/superpowers/specs/2026-06-22-leah-phase4-design.md` v1.0 — Phase 4 closes with v1.1 (voice frontier, multi-device sync, learn-recommend pass-2, camera+vision, multi-agent A2A, attestation, plugin SDK, privacy budget, watchdog supervisor).
**Phase boundary:** Phase 5 PRs do not start until Phase 4 (v1.1 public launch per predecessor §0) ships and runs on the operator's machine for ≥ 14 days. Phase 5 ends with v1.2 public launch.

> **Spec parity:** this file is checked by `scripts/check-spec-parity.sh`. Forbidden phrases (renamed terms, killed cosmetics) are not used in normative body.

---

## 0. Executive summary

Phase 4 made Leah multi-modal and multi-agent on one operator's hardware; Phase 5 makes the SDK that Phase 4 shipped useful (third-party plugins can be found, installed, and trusted from a marketplace), shrinks the voice runtime so always-on listening fits on an 8 GB Mac, and closes the ambient-awareness gaps the 2026 capability inventory (`memory/research_ai_capability_domains_2026.md`) flagged — calendar reasoning, multi-language voice, continuous screen context, OCR-to-memory ingest, and macOS 16 (Tahoe) compatibility. The thesis: Phase 4 made Leah capable; Phase 5 makes Leah at-home on the operator's actual workday.

Seven deliverables. Five build waves. Twenty-three implementer tasks previewed.

| # | Deliverable | Wave | Ship gate |
|---|---|---|---|
| 1 | Marketplace plugin discovery | W3 | Operator installs a third-party plugin from inside Leah in ≤ 3 clicks; auto-update on next launch |
| 2 | Local Whisper distilled | W1 | Streaming STT resident ≤ 250 MB RAM with first-partial latency ≤ 400 ms |
| 3 | Calendar-aware reasoning | W2 | Reasoner cites operator's next 3 events without explicit `/calendar`; conflict-detection works |
| 4 | Multi-language voice | W2 | Wake-word + STT + TTS round-trip in ja-JP, es-ES, fr-FR, de-DE, zh-CN with parity ≤ 1.4× en-US |
| 5 | macOS 16 (Tahoe) compatibility | W1 | App passes Apple's macOS 16 validation suite; CI green on Tahoe runner |
| 6 | Vision OCR → memory ingest | W2 | Captured screen text searchable via `leah ask` within 90 s of capture |
| 7 | Live screen reasoning | W4 | Continuous screen-aware reasoning with consent banner + ≤ 3 W steady-state extra power draw |

Total: ~12 weeks solo (parallel-cap of 6 per `CLAUDE.md` Dispatch parallelism). Each wave = independently mergeable; v1.2 ship after W5.

### 0.1 What Phase 5 is NOT

- **Not a paid tier or revenue share.** BYOK Anthropic remains the cost model (predecessor §0.1; predecessor §17.18). The marketplace ships as free distribution only; paid plugin distribution stays explicitly out — see §1.13 deferral rationale and `memory/project_leah_ship_path.md` ("personal-use product, owner is the user, best-experience bar"). No Stripe wiring, no fee skim, no "Pro" plugin gate.
- **Not iOS / iPadOS / mobile companion.** Mobile remains v2 horizon (predecessor §2.10 + predecessor §0.1). Phase 5 does not ship an A2A mobile peer, does not add APNs, does not add a phone HUD. The capability-domain research (`memory/research_ai_capability_domains_2026.md`) shows CarPlay AI category exists; Leah waits.
- **Not a new LLM backbone.** Anthropic Sonnet 4.6 + Haiku 4.5 + Opus 4.8 remains canonical (predecessor §0.1).
- **Not a redesign of the v1 visual identity.** Phase 1–3 lock holds. Phase 5 only adds surfaces (Marketplace pane, calendar widget, language pickers, screen-aware HUD glyph) that compose with existing chrome.
- **Not a new vector store.** `sqlite-vec` via `modernc.org/sqlite/vec` stays single-file.
- **Not multi-Anthropic-key load-balancing.** Single operator, single key — adding key rotation/load-balancing is over-engineering absent a real cost or rate-limit incident. Operator may rotate keys via Settings; daemon never holds > 1 active key.
- **Not per-plugin Spaces (macOS Spaces-level isolation).** The Phase 4 plugin sandbox (predecessor §7) is already process-isolated + capability-gated + privacy-budgeted. True OS-level Spaces isolation adds complexity without a real compromise vector. Revisit only if a plugin escape-of-sandbox is demonstrated.
- **Not Sparkle-replacement self-update (delta diffs + downgrade-on-corrupt).** Sparkle ships and works (predecessor §13 supervisor + Phase 3 sign+notarize+Sparkle integration). Delta updates save bandwidth, not time, on a 40 MB binary; downgrade-on-corrupt is solved by Sparkle's signature check. Defer.
- **Not a voice persona library.** ElevenLabs Flash v2.5 + Apple Ava Premium suffice (predecessor §1.2). Persona switching is cosmetic; multi-language coverage (§4) is the load-bearing voice gap.
- **Not paid plugin distribution.** See first bullet.
- **Not mobile push (APNs).** No mobile target this phase.
- **Not operator-facing onboarding polish.** Phase 3 wizard already ships; Phase 4 added voice-onboarding copy; further wizard polish is a sub-1-day issue, handled out-of-band.

### 0.2 Cross-cutting invariants (extending Phase 4 §0.2)

The Phase 4 invariants (1–6) remain binding. Phase 5 adds:

7. **Marketplace is read-mostly + content-addressed.** Plugin manifests + binaries fetch over HTTPS from a static CDN (operator-trustable host: GitHub Releases or equivalent). Daemon never installs an unsigned plugin; signature verification is the same gate as Phase 4 §6 continuous attestation. No marketplace server-side state writes from Leah; no telemetry beacons.
8. **Language detection is best-effort, not blocking.** When voice (§4) detects an unknown language, fall back to en-US transcription with a one-line nudge in the panel rather than silently fail.
9. **Default-OFF for screen-continuous capture.** Phase 4 §0.2 (3) already covers camera continuous. Phase 5 §7 (live screen) inherits the same default-OFF discipline and adds a HUD glyph that is *always* visible while the stream runs (no hidden capture).
10. **OS-version compatibility is a ship gate, not a TODO.** Phase 5 §5 requires CI to run the suite on a Tahoe (macOS 16) runner before any Phase 5 PR merges. A v1.2 release cannot ship if the Tahoe runner is red.
11. **OCR-to-memory is rate-limited.** Phase 5 §6 ingest may not write > 200 memory rows / hour by default (operator can raise the cap in Settings → Memory → "Screen capture ingest rate"). Prevents accidental flood from a misclicked "ingest all open windows".
12. **Whisper RAM budget is enforced.** Phase 5 §2 ships the distilled model as the new default; the original 850 MB Whisper-large-v3 ONNX stays selectable in Settings → Voice → "Use full-fidelity STT model (850 MB)" for operators on 16 GB+ machines who want max accuracy. CI fails any voice subsystem PR that pushes resident RAM above 280 MB on the distilled model.
13. **No marketplace promotion of un-attested plugins.** Marketplace UI shows plugins that pass the same continuous-attestation gate that Phase 4 §6 installs. Failing-attestation plugins are filtered from search results — not surfaced with a warning, just hidden — to keep the default browse safe.

---

## 1. Deliverable 1 — Marketplace plugin discovery

### 1.1 Goal

Phase 4 ships the plugin SDK + sandbox (predecessor §7). Phase 5 ships the distribution path: the operator can search, install, update, and uninstall third-party plugins from inside Leah without `git clone` and without trusting random binaries. The marketplace is the answer to "Phase 4 built a runway; how do plugins actually reach the operator?"

This is leverage-1 because every Phase 4 plugin-SDK design decision (capability manifest, signature gate, privacy budget surface) becomes load-bearing UX in the marketplace. A working SDK without a marketplace is a tool; with one, it is an ecosystem.

### 1.2 Capability matrix

| Capability | Phase 4 baseline | Phase 5 addition |
|---|---|---|
| Plugin install | `leah plugin install ./path` (local file) | Marketplace search + 1-click install from Settings → Plugins |
| Plugin update | Manual re-install | Auto-update on daemon start, opt-out per plugin |
| Plugin discovery | Operator must know the URL/path | Searchable index with categories, ratings, capability filter |
| Trust | SDK signing + sandbox | + attestation continuity check on every load + marketplace signature gate |
| Source | Single-vendor (operator builds own) | Multi-author registry (anyone can publish; signature root pinned per author) |

### 1.3 Interfaces

#### 1.3.1 Daemon Go interfaces

```go
// internal/marketplace/index.go
type Index interface {
    // Search returns ranked plugin entries matching the query.
    // Filters may scope by capability requirement, category, or author.
    Search(ctx context.Context, query string, filters SearchFilters) ([]PluginEntry, error)
    Get(ctx context.Context, id PluginID) (PluginEntry, error)
    Refresh(ctx context.Context) (RefreshResult, error) // pulls latest index from CDN
}

// internal/marketplace/installer.go
type Installer interface {
    // Install fetches, verifies, sandboxes, and registers a plugin entry.
    // Blocks on attestation verdict; aborts if signature or capability gate fails.
    Install(ctx context.Context, id PluginID, version PluginVersion) (InstallReceipt, error)
    Uninstall(ctx context.Context, id PluginID) error
    Update(ctx context.Context, id PluginID) (UpdateReceipt, error)
    AutoUpdateAll(ctx context.Context) ([]UpdateReceipt, error) // daemon-start hook
}

type PluginEntry struct {
    ID             PluginID
    Name           string
    Author         AuthorID         // pinned signature root
    AuthorVerified bool             // author has published ≥ 3 plugins with no revoke
    Version        PluginVersion
    Category       Category         // "voice", "memory", "integration", "vision", "utility"
    Capabilities   []CapabilityReq  // mic, screen, mail-read, calendar-write, etc.
    Sha256         []byte
    SizeBytes      int64
    InstallCount   int64            // CDN-aggregated; never per-operator
    LastUpdated    time.Time
    AttestationOK  bool             // matches Phase 4 §6 verdict at index-build time
}

type SearchFilters struct {
    Category       *Category
    RequiresCap    []CapabilityReq // filter OUT plugins that demand any of these
    AuthorVerified bool
    MaxSizeBytes   int64
}
```

#### 1.3.2 IPC frame kinds (additions to predecessor §17.2)

| Frame kind | Direction | Payload | Notes |
|---|---|---|---|
| `marketplace.search` | HUD → daemon | `{query, filters}` | Returns `marketplace.results` |
| `marketplace.results` | daemon → HUD | `{entries[]}` | Ranked; paginated at 50 |
| `marketplace.install` | HUD → daemon | `{id, version?}` | Returns install progress events |
| `marketplace.install.progress` | daemon → HUD | `{id, stage, pctDone}` | `stage`: fetch | verify | sandbox | register | done |
| `marketplace.uninstall` | HUD → daemon | `{id}` | Returns ack |
| `marketplace.update.available` | daemon → HUD | `{entries[]}` | Surfaced via notification widget |

#### 1.3.3 Swift protocol (HUD process)

```swift
// LeahHUD/Marketplace/MarketplaceCoordinator.swift
protocol MarketplaceCoordinator {
    func search(query: String, filters: MarketplaceFilters) async throws -> [PluginEntry]
    func install(id: PluginID) async throws -> InstallReceipt
    func uninstall(id: PluginID) async throws
    func observeUpdates() -> AsyncStream<[UpdateAvailable]>
}
```

### 1.4 Index format

The marketplace index is a single signed JSON file fetched from a CDN endpoint (`https://marketplace.leah.app/index.v1.json` or operator-configurable in Settings → Plugins → "Marketplace URL"). No server-side state from Leah; the index is a static artifact published per release.

```jsonc
{
  "schemaVersion": 1,
  "generatedAt": "2026-06-23T12:00:00Z",
  "signature": "ed25519:base64...",  // signature root pinned in the binary at build time
  "plugins": [
    {
      "id": "com.example.kagi-search",
      "name": "Kagi Search",
      "author": "alice@example.com",
      "authorVerified": true,
      "version": "1.2.0",
      "category": "integration",
      "capabilities": ["network-out:kagi.com"],
      "sha256": "deadbeef...",
      "sizeBytes": 423000,
      "installCount": 1820,
      "lastUpdated": "2026-06-20T09:14:00Z",
      "downloadURL": "https://github.com/.../releases/download/v1.2.0/kagi-search.leahplugin",
      "homepage": "https://github.com/example/leah-kagi-search",
      "description": "Search Kagi from Leah; respects operator API key.",
      "categoryDigest": "..."
    }
  ]
}
```

### 1.5 Auto-update policy

- Daemon checks the index on start + every 24 h while running.
- Update available → emit `marketplace.update.available` IPC; surface in HUD notification widget ("Kagi Search 1.2.1 available").
- Operator decides per-plugin: "Auto-update" (default), "Notify only", "Pinned at version X".
- Auto-update gate: same Phase 4 §6 attestation check + signature verify + capability-delta confirm. If the new version requests a capability the prior version did not, the update is *NOT auto-applied* — surfaces as a manual approval banner ("Kagi Search now wants screen-read access. Approve?"). This prevents capability creep.

### 1.6 Data model

New tables in `leah.db` (migration `2026-06-23-001-marketplace.sql`):

```sql
CREATE TABLE marketplace_index_cache (
    fetched_at      INTEGER NOT NULL,
    body            BLOB NOT NULL,        -- gzipped index JSON
    signature_ok    INTEGER NOT NULL,     -- 1 if signature verified at fetch
    PRIMARY KEY(fetched_at)
);

CREATE TABLE marketplace_plugin (
    id              TEXT PRIMARY KEY,     -- plugin ID (com.example.foo)
    installed_at    INTEGER NOT NULL,
    version         TEXT NOT NULL,
    autoupdate      INTEGER NOT NULL DEFAULT 1,
    pinned_version  TEXT,                 -- null = float; non-null = pinned
    last_updated_at INTEGER,
    sha256          BLOB NOT NULL,
    attestation_ok  INTEGER NOT NULL DEFAULT 1
);

CREATE TABLE marketplace_capability_grant (
    plugin_id       TEXT NOT NULL REFERENCES marketplace_plugin(id) ON DELETE CASCADE,
    capability      TEXT NOT NULL,
    granted_at      INTEGER NOT NULL,
    granted_by      TEXT NOT NULL,        -- 'install' | 'update-prompt' | 'settings'
    PRIMARY KEY(plugin_id, capability)
);

CREATE TABLE marketplace_revoke (
    plugin_id       TEXT NOT NULL,
    reason          TEXT NOT NULL,        -- 'author-revoke' | 'attestation-fail' | 'operator'
    revoked_at      INTEGER NOT NULL,
    PRIMARY KEY(plugin_id, revoked_at)
);
```

### 1.7 Security model

- **Index signature is pinned in the binary.** The Ed25519 public key that signs the index lives in the app bundle, set at build time. An attacker who compromises the CDN cannot serve a malicious index without also extracting the signing key.
- **Plugin binary signature.** Each plugin manifest includes the SHA-256 of its binary; daemon refuses to load on mismatch. The Phase 4 §6 attestation gate runs on every load, not just install.
- **Capability deltas surface as approval prompts.** A version bump that adds capabilities cannot auto-install.
- **No telemetry.** Install counts on the index are CDN-aggregated download counts (HTTP server logs); Leah never reports back. No per-operator analytics, no opt-in metrics. The 2026 capability research (`memory/research_ai_capability_domains_2026.md`) shows every vendor ships telemetry; Leah's "Operator owns the agent loop" position (CLAUDE.md Identity/output + `memory/research_ai_assistants_big_tech_2026.md` Leah differentiation) breaks here if we add one.
- **Marketplace URL is operator-overridable.** Operator can point Leah at a self-hosted index for air-gapped or alternate-distro deployments. The pinned signature root still applies; operator who wants to override the signature root edits Settings → Plugins → "Trust roots" and adds their own.
- **Revoke list.** If an installed plugin appears in the index's revoke list (per `marketplace_revoke`), daemon unloads it at next start and surfaces a notification.

### 1.8 Failure modes

| Failure | Detection | Degraded behavior |
|---|---|---|
| Index fetch fails (network) | HTTP error | Use cached `marketplace_index_cache`; surface a discreet "Catalog stale (last refresh: 2h ago)" tag in Settings → Plugins |
| Index signature fails | Verify step | Refuse to use; fall back to last-known-good cached index; log to OSLog; surface red banner |
| Plugin binary SHA mismatch | Pre-install verify | Abort install; show error toast with plugin name + author |
| Plugin requires capability operator denied | Install step | Surface capability sheet; operator confirms or cancels |
| Auto-update breaks plugin (panic on load) | Daemon load failure × 3 | Roll back to prior version; mark plugin as broken; surface notification "Kagi Search 1.2.1 crashed on load; reverted to 1.2.0" |
| CDN serves index version > schema known | Schema check | Refuse; surface "Update Leah to use the latest marketplace" |
| Operator-pinned version no longer in index | Resolve step | Keep installed version functional; flag as "Unmaintained" in Settings → Plugins |

### 1.9 Performance budget

| Surface | RAM target | CPU target | Latency target |
|---|---|---|---|
| Index fetch (4 MB compressed) | 8 MB transient | < 5% × 200 ms | ≤ 800 ms cold cache hit |
| Search query (over cached index) | n/a | < 2% × 30 ms | ≤ 50 ms p95 |
| Plugin install (1 MB plugin) | 12 MB transient | < 8% × 1.2 s | ≤ 1.5 s end-to-end |
| Auto-update check (daemon start) | n/a | < 2% × 200 ms | ≤ 250 ms |
| Marketplace pane open (Settings) | 18 MB | < 1% steady | ≤ 120 ms paint |

### 1.10 UI surfaces

- **Settings → Plugins (extended pane)** — previously listed only installed plugins; now adds a "Browse" tab with search bar, category chips ("Voice", "Memory", "Integration", "Vision", "Utility"), filter sheet (capability filter, author-verified toggle, size cap). Each plugin card shows name, author, install count, capabilities required, "Install" button.
- **HUD notification widget** — `marketplace.update.available` events surface as toast: "3 plugin updates available." Click → Settings → Plugins.
- **Capability approval sheet** — modal sheet when an update or install adds new capabilities. Shows existing capabilities (struck-through if removed), new capabilities (bold), explanation per capability.
- **No surface in wizard.** First-launch shape stays fixed (per predecessor §0.2 invariant). Marketplace is discovered post-wizard.
- **No dashboard surface.** Marketplace is opt-in browse; operator does not need a count card.

### 1.11 Telemetry policy (binding)

Leah does not send install counts, search queries, telemetry events, error reports, or any other beacon back to a marketplace server. The CDN log of binary downloads is the only data source; that is a passive HTTP server log and does not contain operator identity beyond an IP address visible to the CDN provider for the duration of the request.

If a future version wants opt-in crash reporting for the marketplace specifically, it must be a separate, named, operator-toggled Setting — never silent, never default-on.

### 1.12 What ships in v1.2 vs deferred

**Ships:**
- Searchable index (categories, capability filter, author-verified flag)
- 1-click install/uninstall from Settings → Plugins
- Auto-update on daemon start (per-plugin opt-out)
- Capability-delta approval prompt
- Operator-overridable marketplace URL + trust roots
- Revoke list honored at next-start

**Deferred to v1.3+:**
- **Paid plugin distribution.** Predecessor §0.1 fixes BYOK-only cost model; CLAUDE.md decision priority + `memory/project_leah_ship_path.md` (personal-use, owner is user) makes a revenue-share product distortion. Free distribution only.
- **In-marketplace ratings / reviews.** Adds server-side state + telemetry surface that conflicts with §1.11 binding. Operator who wants opinions reads the plugin's GitHub README; this is enough.
- **Plugin dependency resolution (plugin A depends on plugin B).** Phase 5 plugins are flat; no transitive deps. If plugins start needing shared libraries, revisit in v1.3.
- **Search ranking algorithm (beyond install-count + freshness).** v1.2 ranks by `(installCount × freshness)` only. No personalization, no operator-behavior signal. Sufficient.

### 1.12.1 Marketplace search ranking detail

For v1.2, ranking is:

```
score(entry, query) = textMatch(entry, query) × log(1 + installCount) × freshness(entry)
freshness(entry)    = exp(-(now - lastUpdated) / 90d)
textMatch(entry, q) = BM25(entry.name + entry.description, q)
```

No personalization, no operator-behavior signal, no cross-operator collaborative filtering — those break the telemetry-abstention invariant §1.11. If two plugins tie on score, ranked by `installCount` descending then by `name` ascending for determinism.

Filters apply *before* ranking:
- `Category` filter narrows the pool.
- `RequiresCap` filters OUT plugins that demand any of the listed capabilities (operator wants to hide plugins that need, e.g. mic when they've said "no mic plugins").
- `AuthorVerified` keeps only verified-author entries.
- `MaxSizeBytes` clips at the operator's cap.

### 1.12.2 Marketplace browse pagination

50 entries per page. Pagination state is local-only (no marketplace server). Operator scroll loads next-page from the cached index, which contains all entries (full index download is < 5 MB compressed even at 10k plugins, so no pagination needed at the fetch layer).

### 1.13 Why no paid distribution

Three load-bearing reasons:

1. **Cost model invariant.** Predecessor §0.1 fixes BYOK-only; adding Stripe + revenue share + tax handling + payout infrastructure breaks the "single binary, operator's machine, no account" position the 2026 research surfaced as Leah's differentiator (`memory/research_ai_assistants_big_tech_2026.md` — "Operator owns the agent loop").
2. **Ship-path discipline.** `memory/project_leah_ship_path.md` makes Leah a personal-use product with the owner as user. Revenue infrastructure has no positive UX impact for that user; it only adds risk surface (PCI scope, chargeback fraud, plugin author tax compliance).
3. **Marketplace adoption signal.** Free distribution measures whether plugins reach operators. If v1.2 ships, 50+ free plugins appear, operators install them, and *only then* a real demand for paid distribution emerges — revisit. Not the other way around.

---

## 2. Deliverable 2 — Local Whisper distilled

### 2.1 Goal

Phase 4 ships Whisper-large-v3 ONNX as the default local STT at 850 MB resident (predecessor §1.7). That budget locks out 8 GB Macs from always-on voice and pushes 16 GB Macs to ≥ 50% memory pressure when voice + reasoner + HUD + Chrome are all live. Phase 5 ships a distilled student model (Whisper-distil-large-v3 or distil-small.en/multi as the family) tuned for streaming inference, targeting ≤ 250 MB resident with ≤ 1.3× WER degradation vs the full model on the operator's empirically observed speech.

This is leverage-2 because RAM headroom unblocks every Phase 5 ambient feature — continuous live-screen (§7), OCR-to-memory (§6), calendar-aware reasoning (§3) — all of which want to coexist with always-on voice without forcing the operator to choose.

### 2.2 Model selection

- **Primary candidate:** Whisper-distil-large-v3 (`distil-whisper/distil-large-v3`) — 756 M params → ONNX-quantized to int8 ≈ 200 MB on disk, ≈ 240 MB resident with ONNX runtime overhead.
- **Fallback:** Whisper-distil-medium.en (243 M params) — disk ≈ 120 MB, resident ≈ 160 MB. Used when memory pressure detected via `os.PSI`.
- **Multi-language:** Whisper-distil-small (244 M params, multi-lingual) — covers §4 (multi-language voice); resident ≈ 150 MB. Loaded conditionally when `voice.lang` is non-en.
- **Original full-fidelity model retained** as opt-in (Settings → Voice → "Use full-fidelity STT model (850 MB)").

### 2.3 Interfaces

```go
// internal/voice/stt.go (existing interface from predecessor §1.3.1)
// New implementations:

// internal/voice/stt/distilled.go
type DistilledSTT struct {
    modelPath  string
    runtime    *onnxruntime.Session
    lang       Language
    // ...
}

func NewDistilledSTT(modelPath string, lang Language) (*DistilledSTT, error)
// Stream() respects predecessor §1.3.1 STT interface contract.
// Info() returns ProviderInfo{name: "whisper-distil-large-v3", isLocal: true, ramMB: 250}
```

The existing `STT` interface (predecessor §1.3.1) does not change. The daemon swaps the default `STT` provider behind `Info().Name` selection.

### 2.4 Selection logic

```go
// internal/voice/select.go
func selectSTT(ctx context.Context, prefs VoicePrefs, sys SysSnapshot) STT {
    if prefs.UseFullFidelity && sys.AvailableRAMMB > 2000 {
        return whisperLargeV3 // Phase 4 default
    }
    if sys.AvailableRAMMB < 400 || sys.MemoryPressure == os.PressureHigh {
        return whisperDistilMediumEn // fallback
    }
    if prefs.Language != "en-US" {
        return whisperDistilSmallMulti // covers §4
    }
    return whisperDistilLargeV3 // new default
}
```

`SysSnapshot` is captured once per voice session start; the choice is fixed for that session to avoid mid-utterance model swap.

### 2.5 Performance budget (revised vs predecessor §1.7)

| Surface | RAM target | CPU target | Latency target |
|---|---|---|---|
| Whisper-distil-large-v3 cold load | 250 MB | 12% × 350 ms | First partial ≤ 400 ms post-VAD-stop |
| Whisper-distil-large-v3 streaming, idle | 250 MB resident | < 2.5% steady | n/a |
| Whisper-distil-medium.en fallback | 160 MB | < 2% steady | First partial ≤ 350 ms |
| Whisper-distil-small (multi) | 150 MB | < 2% steady | First partial ≤ 450 ms |
| Full-fidelity opt-in (Whisper-large-v3) | 850 MB (unchanged) | per predecessor §1.7 | per predecessor §1.7 |

### 2.6 Quality target

- **WER (word error rate) target on operator's voice:** ≤ 1.3× the full-fidelity baseline. The operator records a 5-minute calibration corpus on first voice session; daemon computes WER nightly on the corpus comparing distilled vs full-fidelity output. If WER > 1.5× for 3 consecutive nights, daemon automatically suggests switching to full-fidelity (Recommender, §3 of predecessor).
- **Streaming first-partial latency:** within 50 ms of full-fidelity (distilled is faster in absolute terms; the cap is to prevent a regression).
- **Punctuation + casing:** parity with full-fidelity. The distil family preserves these features.

### 2.7 Data model

```sql
CREATE TABLE voice_model (
    id            TEXT PRIMARY KEY,        -- e.g. 'whisper-distil-large-v3'
    family        TEXT NOT NULL,           -- 'distil-whisper' | 'whisper'
    lang          TEXT NOT NULL,           -- 'en-US' | 'multi'
    disk_path     TEXT NOT NULL,
    sha256        BLOB NOT NULL,
    ram_mb        INTEGER NOT NULL,
    loaded_count  INTEGER NOT NULL DEFAULT 0,
    last_loaded_at INTEGER
);

CREATE TABLE voice_calibration (
    id            INTEGER PRIMARY KEY,
    recorded_at   INTEGER NOT NULL,
    audio_path    TEXT NOT NULL,            -- ~/Library/Application Support/Leah/calib/
    transcript    TEXT NOT NULL,
    duration_ms   INTEGER NOT NULL
);

CREATE TABLE voice_wer_sample (
    id            INTEGER PRIMARY KEY,
    at            INTEGER NOT NULL,
    calibration_id INTEGER NOT NULL REFERENCES voice_calibration(id) ON DELETE CASCADE,
    model_id      TEXT NOT NULL REFERENCES voice_model(id),
    wer           REAL NOT NULL,
    transcribed   TEXT NOT NULL
);
```

### 2.8 Security model

- **Model integrity** — each model file SHA-256 checked at load. Mismatch → refuse to load + log + fall back to next-best model.
- **Calibration audio is local** — `~/Library/Application Support/Leah/calib/*.wav` is the operator's voice; never uploaded. Settings → Voice → "Delete voice calibration" wipes the directory + zero the rows.
- **No model upload.** Distilled model ships in app bundle; not fetched at runtime. Reduces network attack surface for STT.
- **Cloud fallback** (Phase 4 §1.5 OpenAI Whisper API) remains opt-in and unchanged; distilled is strictly local.

### 2.9 Failure modes

| Failure | Detection | Degraded behavior |
|---|---|---|
| Distilled model fails to load (file corruption) | ONNX load error | Fall back to next-smaller distilled; if all fail, fall back to `SFSpeechRecognizer` single-shot |
| WER calibration shows > 2× degradation | Nightly compute | Surface notification recommending full-fidelity opt-in; do not auto-switch (operator chooses) |
| Memory pressure spikes during streaming | `os.PSI` watcher | Continue current utterance; switch to medium.en at next session start |
| Operator records calibration in noisy environment | Audio energy + SNR check | Discard sample; prompt operator to re-record in quieter space |
| ONNX runtime version mismatch | Boot check | Refuse load; log; surface "Voice model requires app update" |

### 2.10 UI surfaces

- **Settings → Voice (extended)** — "STT model" dropdown: "Distilled (default, fast, low RAM)" | "Multi-language (250 MB, slower)" | "Full-fidelity (850 MB, max accuracy)".
- **Settings → Voice → Calibration sheet** — "Record calibration" button (5-min guided recording, predefined script in operator's language) + "Last calibration: 2 days ago, WER 4.1% (distilled) vs 3.6% (full)".
- **HUD ambient** — no new chrome; voice waveform unchanged.
- **Recommender (predecessor §3)** — new recommendation kind `voice-fidelity-upgrade` surfaced when sustained WER degradation crosses threshold.

### 2.11 What ships in v1.2 vs deferred

**Ships:**
- Whisper-distil-large-v3 as new default
- Whisper-distil-medium.en pressure fallback
- Whisper-distil-small as multi-language path (used by §4)
- Calibration corpus + nightly WER tracking
- Settings → Voice model dropdown
- Recommender hook for fidelity upgrade

**Deferred to v1.3+:**
- **Operator-fine-tuned model.** Adapter weights trained on the operator's voice. Promising but the engineering cost is large and quality lift vs distilled is unverified. Revisit when 6 months of calibration corpus exists.
- **On-device LLM-based punctuation/diarization.** Distilled handles punctuation; diarization is single-operator scope per predecessor §1.9.
- **Whisper-distil-tiny (< 100 MB).** Distilled-medium.en already covers the low-RAM fallback. Tiny adds nothing without quality drop > 2×.

---

### 2.12 ONNX runtime selection

Phase 5 uses `onnxruntime-go` v1.18+ (Apache 2.0). The runtime is configured:

- **CPU + ANE provider** preferred — Apple Neural Engine accelerated execution where available.
- **Memory arena** disabled — int8 quantized models benefit from per-op allocation rather than arena reuse.
- **Thread count** = `runtime.NumCPU() / 2`, capped at 4. STT is mostly memory-bound, not CPU-bound.
- **Graph optimizations** set to `ORT_ENABLE_ALL` at session creation.

These knobs apply at session-init only; mid-stream changes are not supported by ONNX Runtime without re-initializing.

### 2.13 Cold-load amortization

A naïve approach reloads the model on every voice session start (350 ms cold cost). To amortize, the daemon:

- On voice subsystem initialization (daemon start), pre-warms the *default* STT provider (distil-large-v3 in the common case). Cold cost paid once at boot, not per session.
- On `selectSTT()` returning a non-default provider for a specific session, lazy-loads that model and keeps it resident for 5 minutes after session end (covers operator's "ask another question 30 s later" pattern).
- After 5 min idle, evicts the lazy-loaded model; on next demand, reloads.

The pre-warm slot supports at most ONE pre-warmed model; switching the default via Settings → Voice reloads. Operator-facing UX: a quiet pre-warm shows no spinner.

### 2.14 Calibration script per locale

Each locale's calibration script (≈ 5 min recital) is bundled in the app at `Resources/voice/calibration/scripts/<locale>.json`:

```jsonc
{
  "locale": "en-US",
  "totalSeconds": 320,
  "lines": [
    "The quick brown fox jumps over the lazy dog.",
    "Pack my box with five dozen liquor jugs.",
    "How vexingly quick daft zebras jump."
    // ~80 lines covering phonemes + numbers + punctuation + common-domain words
  ]
}
```

The script is designed to:
1. Cover all phonemes in the locale (linguist-curated).
2. Include numbers, dates, technical terms (operator's likely domain).
3. Test punctuation cues (statements vs questions).
4. Run ≈ 5 minutes at moderate pace.

Operator records reciting the script; the daemon stores the audio at `~/Library/Application Support/Leah/calib/<locale>-<timestamp>.wav` and the reference transcript in `voice_calibration.transcript`.

### 2.15 WER computation

```go
// internal/voice/calibration.go
func computeWER(reference, hypothesis string) float64 {
    refTokens := tokenize(reference)        // lowercase + punctuation-strip + split
    hypTokens := tokenize(hypothesis)
    distance := levenshtein(refTokens, hypTokens)
    return float64(distance) / float64(len(refTokens))
}
```

Tokenization is locale-aware:
- Latin scripts: whitespace split + punctuation strip.
- CJK scripts (zh-CN, ja-JP): character-level for hanzi/kanji; per-word for kana/katakana.

Edge tokens (numbers, dates) are normalized: "twenty twenty six" and "2026" count as a match.

### 2.16 Idle behavior + power

When voice is enabled but no session is active:
- Wake-word model resident at 35 MB (Phase 4 §1.7 unchanged).
- Whisper-distil-large-v3 NOT resident — loaded on first wake.
- First-wake cost: 350 ms model load + 400 ms first partial = 750 ms total before any TTS.

If operator's first-wake latency is felt:
- Settings → Voice → "Keep STT model warm" loads Whisper-distil-large-v3 at daemon start and keeps it resident (250 MB extra RAM cost).
- Default OFF — most operators tolerate 750 ms first-wake; only "always-on power user" needs warm.

## 3. Deliverable 3 — Calendar-aware reasoning

### 3.1 Goal

Phase 1–3 ships gcal adapter (token wiring per `memory/leah_first_launch_integration_auth.md`). Phase 4 does not add reasoning hooks on calendar. Phase 5 makes the reasoner calendar-aware *by default*: every prompt receives the operator's next 3 calendar events as ambient context, conflict detection runs on schedule mutations, and the focus-panel surfaces the next event as a one-line "Up next" footer. The 2026 capability inventory (`memory/research_ai_capability_domains_2026.md`) calls this gap out: "Calendar smart-scheduling (Motion/Reclaim parallels)".

This is leverage-3 because calendar is the operator's most temporally-relevant context after current focus — and a reasoner that doesn't know about it generates schedule-blind suggestions that frustrate operators (Phase 4 recommender, predecessor §3, surfaces irrelevant nudges during meetings).

### 3.2 Capability matrix

| Capability | Phase 1–3 baseline | Phase 5 addition |
|---|---|---|
| Read calendar | `/calendar` explicit command | Ambient context injected into every reasoner prompt |
| Conflict detection | None | Flag overlap on `/schedule` or `/meet` commands; warn on auto-suggested time |
| Smart scheduling | None | Reasoner suggests free 30-min blocks given operator's working hours |
| Up-next display | None | HUD ambient + focus panel show next event countdown |
| Multi-calendar | None | Aggregate across gcal + iCal local + Outlook (when ms365 adapter exists) |

### 3.3 Interfaces

```go
// internal/calendar/store.go (existing from gcal adapter wiring)
// New ambient-context interface:

// internal/reasoner/calendarctx.go
type CalendarContext interface {
    // Next returns events in [now, now+window]. Sorted ascending.
    Next(ctx context.Context, window time.Duration, maxN int) ([]CalEvent, error)
    // Conflicts returns events overlapping the proposed range.
    Conflicts(ctx context.Context, start, end time.Time) ([]CalEvent, error)
    // FreeBlocks returns gaps ≥ minDuration in [now, now+window] respecting working hours.
    FreeBlocks(ctx context.Context, window, minDuration time.Duration) ([]TimeRange, error)
}

type CalEvent struct {
    ID       string
    Title    string         // PII; never injected into telemetry
    Start    time.Time
    End      time.Time
    Source   CalSource      // gcal | ical-local | ms365
    Conf     ConfMode       // in-person | video | phone | unknown
    Attendees int           // count only; never names in ambient context
    Privacy  PrivacyClass   // 'public' | 'private' | 'confidential' per calendar API
}
```

#### 3.3.1 Reasoner prompt injection

The reasoner receives a system-prompt prefix derived from `CalendarContext.Next(2h, 3)`:

```
Current time: 2026-06-23 14:37 PDT.
Upcoming (next 2h, top 3):
- 15:00 — 1:1 with K (30m, video)
- 16:00 — Phase 5 review (60m, in-person)
- 17:30 — gym (60m)
```

This prefix is added by the daemon, not by HUD, and never sent to plugins (capability gate: plugins do not get calendar context unless they request `calendar-read`, in which case they get a redacted form).

### 3.4 Working-hours model

- Operator sets working hours in Settings → Calendar → "Working hours" (default Mon–Fri 09:00–18:00, operator-tz).
- "Free blocks" computation respects working hours: no free-block suggestions outside.
- Operator can mark a calendar as "Personal" — `Personal` calendars are aggregated for conflict detection but ignored for free-block suggestion (a "yoga" event on personal calendar should not block a work-hours work suggestion, just inform).

### 3.5 Data model

```sql
CREATE TABLE calendar_source (
    id            TEXT PRIMARY KEY,         -- 'gcal:alice@example.com'
    kind          TEXT NOT NULL,            -- 'gcal' | 'ical-local' | 'ms365'
    display_name  TEXT NOT NULL,
    role          TEXT NOT NULL,            -- 'work' | 'personal'
    enabled       INTEGER NOT NULL DEFAULT 1,
    last_synced_at INTEGER
);

CREATE TABLE calendar_event_cache (
    src_id        TEXT NOT NULL REFERENCES calendar_source(id) ON DELETE CASCADE,
    event_id      TEXT NOT NULL,
    title         TEXT NOT NULL,
    start_at      INTEGER NOT NULL,
    end_at        INTEGER NOT NULL,
    privacy       TEXT NOT NULL CHECK(privacy IN ('public','private','confidential')),
    conf_mode     TEXT,
    attendee_count INTEGER NOT NULL DEFAULT 0,
    raw           BLOB,                     -- gzipped original event JSON for diff
    cached_at     INTEGER NOT NULL,
    PRIMARY KEY(src_id, event_id)
);

CREATE TABLE calendar_working_hours (
    weekday       INTEGER NOT NULL,         -- 0=Sun..6=Sat
    start_minute  INTEGER NOT NULL,         -- minutes from midnight
    end_minute    INTEGER NOT NULL,
    tz            TEXT NOT NULL,
    PRIMARY KEY(weekday, tz)
);

CREATE TABLE calendar_redact_rule (
    id            INTEGER PRIMARY KEY,
    src_id        TEXT NOT NULL REFERENCES calendar_source(id),
    pattern       TEXT NOT NULL,            -- e.g. '*therapy*' redacts title
    replacement   TEXT NOT NULL DEFAULT '(private)'
);
```

### 3.6 Security model

- **PII in titles.** Calendar event titles often contain sensitive content ("Therapy", "Doctor", names). Ambient context injection respects the `Privacy` field: `confidential` events render as `(busy)` in the prompt prefix, not by title.
- **Operator-defined redact rules.** `calendar_redact_rule` lets the operator hide patterns ("therapy", "AA") even from the reasoner prompt — the reasoner sees `(busy)` for those events.
- **Plugins do NOT see calendar by default.** Reading calendar requires `calendar-read` capability, gated through Phase 4 §6 attestation. Even with the capability, plugins get the redacted form.
- **Privacy budget** — each ambient prompt injection counts a single "calendar-read" tick against the calendar privacy budget per Phase 4 §8.
- **Token storage** — gcal/ms365 tokens stored per `memory/leah_first_launch_integration_auth.md` (mode 0600 in `$HOME/.leah-state/secrets/`). No change.

### 3.7 Failure modes

| Failure | Detection | Degraded behavior |
|---|---|---|
| gcal token expired | API 401 | Refresh via gcal adapter (existing); if refresh fails, surface notification + suppress ambient injection |
| Calendar source unreachable (network) | API timeout | Use last cached events ≤ 24 h old; tag prefix with "(calendar may be stale)" |
| Event count > 3 in window | Slice | Inject top 3 by start time; mention "+N more" in prefix |
| Working hours not set | Default applied | Use Mon–Fri 09–18 in operator's tz; nudge operator to confirm in Settings |
| Cache stale > 24 h | Cached-at check | Force refresh on next reasoner call; tolerate up to 200 ms latency add |
| Operator denies calendar permission | First-fetch error | Disable feature; surface in Settings as "Calendar disabled" |

### 3.8 Performance budget

| Surface | RAM target | CPU target | Latency target |
|---|---|---|---|
| Ambient context fetch (cache hit) | n/a | < 0.5% | ≤ 8 ms p95 |
| Ambient context fetch (cache refresh) | n/a | < 2% × 200 ms | ≤ 250 ms p95 |
| Conflict detection (single proposal) | n/a | < 0.5% | ≤ 4 ms |
| Free-block computation (8 h window) | n/a | < 1% × 30 ms | ≤ 40 ms |
| Cache refresh (full daily) | 6 MB transient | < 3% × 400 ms | ≤ 500 ms |
| Steady-state RAM overhead | ≤ 12 MB | n/a | n/a |

### 3.9 UI surfaces

- **HUD focus panel** — small "Up next" footer ("15:00 — 1:1 with K, 23m") when next event is < 2 h away. Click → opens calendar app at the event.
- **HUD ambient** — countdown dot turns gold 10 minutes before next event; pulses 1 minute before.
- **Settings → Calendar (new pane)** — source list, working hours, redact rules, ambient injection toggle (default ON), "Test ambient prompt" button that shows the prefix the reasoner would receive.
- **Notification widget** — `calendar.conflict.warn` event: "Proposed time conflicts with 16:00 review."
- **Voice** — operator can ask "when's my next?" and reasoner answers from cached context (≤ 100 ms).
- **Dashboard** — "Today" card listing today's events (existing dashboard chrome).

### 3.10 What ships in v1.2 vs deferred

**Ships:**
- Ambient context injection (top 3 in next 2h)
- Conflict detection on `/schedule`, `/meet`
- Free-block computation
- Up-next focus-panel footer
- Settings → Calendar pane with working hours + redact rules
- Multi-source aggregation (gcal + iCal local; ms365 when adapter lands)

**Deferred to v1.3+:**
- **Auto-scheduling (Motion-style)** — Leah proposes and books meetings autonomously. Useful but high-risk if the reasoner mis-schedules. Defer until 90 d of conflict-detection telemetry shows zero false positives.
- **Multi-attendee scheduling** — find-a-time across multiple operators. Requires multi-tenant trust model that Leah's "personal-use" position (`memory/project_leah_ship_path.md`) does not yet support.
- **Recurring-pattern detection** — "you take Mondays off; never schedule there". Bandit-recommender (predecessor §3) can absorb this in v1.3.
- **Travel-time integration** — add buffer for in-person events based on Maps ETA. Requires Maps API + location permission; defer.

---

### 3.11 Multi-source dedup

When the same event appears on multiple calendar sources (e.g. an invite added to both work-gcal and personal-iCal), the daemon dedups via:

```
dedupKey = sha256(normalize(title) + start_at + end_at + sorted(attendee_emails))
```

If two events from different sources share a `dedupKey`, the daemon keeps the one with the highest-trust source (gcal > ms365 > ical-local for work events; ical-local > gcal > ms365 for personal). The merged event surfaces in the reasoner prompt once.

If sources disagree on title or time but `dedupKey` collides on attendees + roughly-same timing (within 30 min), surface a notification: "Calendar conflict — two sources show different details for the 15:00 event."

### 3.12 Reasoner prompt injection lifecycle

The CalendarContext-derived prefix is added by the daemon's reasoner middleware, not by the calling code. Lifecycle:

```
HUD posts user prompt → daemon receives
  ↓
daemon.reasoner.MiddlewareChain runs:
  1. PrivacyClassifier (predecessor §17.17) on user prompt content
  2. CalendarContext.Next(2h, 3) → derive prefix
  3. Prefix prepended to system-prompt
  4. Anthropic API call
  ↓
streamed response back to HUD
```

The prefix is NOT a tool call (no `mcp.calendar.next` round trip). It is a static, refresh-on-demand string that the daemon injects on each turn. Round-trip latency stays ≤ 420 ms p95 (§13.4) because the calendar cache typically hits.

### 3.13 Conflict detection on schedule mutations

When operator says "schedule a meeting at 3pm" or types `/schedule 3pm`:

1. Daemon parses the natural-language time → `time.Time` value.
2. Daemon calls `CalendarContext.Conflicts(start, end)`.
3. If conflicts.Len > 0, reasoner response includes:
   - "I'd schedule that, but you have <conflictTitle> at <conflictStart>."
   - "Options: reschedule to next free block (<freeBlock1>), or override."
4. Operator confirms or chooses option.

The schedule itself is not yet booked autonomously in v1.2 (auto-scheduling deferred per §3.10). Operator runs the calendar app to book; Leah surfaces the next-free-block as a suggestion.

### 3.14 Working-hours timezone handling

Operator may travel; the daemon detects timezone changes via:

- `time.Local` poll on each calendar fetch.
- If `time.Local.String()` differs from last recorded, surface a notification: "Working hours follow your local time. Update Settings if you want to keep [PT] hours while traveling."

`calendar_working_hours.tz` row stores the operator's *home* tz. Daemon uses `time.LoadLocation(tz)` to compute "is now within working hours?" — meaning a traveling operator's working hours stay relative to home, not local.

### 3.15 Privacy classes in real calendars

Google Calendar exposes `visibility: "private"` and `"confidential"` per event. iCal `CLASS:PRIVATE` and `CLASS:CONFIDENTIAL` are the iCalendar equivalents. The daemon maps:

| Source | Field | `PrivacyClass` |
|---|---|---|
| gcal | `visibility=default` | `public` |
| gcal | `visibility=public` | `public` |
| gcal | `visibility=private` | `private` |
| gcal | `visibility=confidential` | `confidential` |
| ical-local | `CLASS:PUBLIC` or absent | `public` |
| ical-local | `CLASS:PRIVATE` | `private` |
| ical-local | `CLASS:CONFIDENTIAL` | `confidential` |
| ms365 | `sensitivity=normal` | `public` |
| ms365 | `sensitivity=personal` | `private` |
| ms365 | `sensitivity=private` | `private` |
| ms365 | `sensitivity=confidential` | `confidential` |

A `confidential` event in the ambient prompt becomes `(busy)`; the time still appears. A `private` event renders title but not attendees in the prompt. A `public` event renders fully (subject to operator redact rules).

### 3.16 Calendar cache refresh strategy

- **Foreground refresh:** when the focus panel opens or the operator runs a calendar-touching command, the cache refreshes if > 5 min stale.
- **Background refresh:** every 10 min while the daemon is alive, refresh all enabled sources.
- **Push refresh:** gcal supports push notifications via `Events.watch`. Phase 5 §3 implementation MAY opt into push if a webhook endpoint is feasible (it isn't on a local daemon without a tunnel). Default: poll. Push deferred.
- **Burst refresh:** when operator runs `/calendar` explicit command, force-refresh + wait up to 800 ms.

Cache TTL: stale-after 10 min, hard-expire after 24 h. Hard-expired cache means "don't render at all"; stale cache means "render with (stale) tag".

### 3.17 Multi-source priority for "Up next" footer

The HUD "Up next" footer (§3.9) selects from all enabled sources. The event chosen is the one with the earliest `Start` time in the next 2 h, regardless of source. If two events tie on start time (within 60 s), prefer:

1. The one the operator has accepted RSVP to.
2. The one on the work-role calendar (vs personal-role).
3. The one with more attendees.

## 4. Deliverable 4 — Multi-language voice

### 4.1 Goal

Phase 4 ships voice in en-US only (predecessor §1.9 — "Localization of wake-word (en-US only)" deferred to v1.2). `memory/project_leah_ship_path.md` makes the voice-English-only "documented limitation" a binding pre-launch blocker. Phase 5 ships ja-JP, es-ES, fr-FR, de-DE, zh-CN as supported voice locales — wake-word, STT, TTS, and reasoner UX strings all language-aware.

This is leverage-4 because the operator is plausibly multilingual; voice is currently a 1.4× faster input method than typing for English speakers but a 0× input method for everyone else. The 2026 capability inventory (`memory/research_ai_capability_domains_2026.md` — "China consumer-AI dominated by Doubao 100M+ DAU") shows the rest of the world's AI-assistant traffic doesn't run on English.

### 4.2 Supported languages (v1.2)

| Language | Locale | Wake-word | STT | TTS local | TTS cloud | Reasoner |
|---|---|---|---|---|---|---|
| English (US) | en-US | wake-leah-en.mlmodel (Phase 4) | distil-large-v3 (§2) | Apple Ava Premium | ElevenLabs Flash v2.5 | en-US prompts |
| Japanese | ja-JP | wake-leah-ja.mlmodel | distil-small (multi) | Apple Kyoko Premium | ElevenLabs Flash v2.5 | ja-JP system strings |
| Spanish | es-ES | wake-leah-es.mlmodel | distil-small (multi) | Apple Mónica Premium | ElevenLabs Flash v2.5 | es-ES system strings |
| French | fr-FR | wake-leah-fr.mlmodel | distil-small (multi) | Apple Amélie Premium | ElevenLabs Flash v2.5 | fr-FR system strings |
| German | de-DE | wake-leah-de.mlmodel | distil-small (multi) | Apple Anna Premium | ElevenLabs Flash v2.5 | de-DE system strings |
| Mandarin (CN) | zh-CN | wake-leah-zh.mlmodel | distil-small (multi) | Apple Tingting Premium | ElevenLabs Flash v2.5 | zh-CN system strings |

Note: "wake-leah" stays the wake phrase across locales (preserves brand recall); only the acoustic model varies. Operator may set a localized wake phrase ("Leah" itself is locale-portable) via Settings → Voice → "Wake phrase" — custom phrases retrain on operator's voice (off-by-default; uses Phase 4 wake-word training infra deferred per predecessor §1.9 — see §4.10 deferral note).

### 4.3 Interfaces

```go
// internal/voice/lang.go
type LanguagePack struct {
    Locale          string         // BCP 47, e.g. "ja-JP"
    WakeModelPath   string
    STTHint         string         // "japanese" passed to Whisper-distil-small
    TTSVoiceLocal   string         // "com.apple.voice.premium.ja-JP.Kyoko"
    TTSVoiceCloud   string         // ElevenLabs voice ID for that language
    SystemStrings   map[string]string // localized UI + nudge copy
}

// internal/voice/detect.go
type LangDetector interface {
    // Detect runs a quick (≤ 200 ms) inference on the first 2 s of audio.
    Detect(ctx context.Context, audio []float32) (Locale, Confidence, error)
}
```

### 4.4 Language detection

- **Default behavior:** Operator sets `voice.lang` in Settings → Voice → "Voice language". Daemon loads that language pack at session start.
- **Auto-detect mode (opt-in):** Operator enables "Detect language per session". Daemon runs lightweight detector on first 2 seconds of audio (Whisper-tiny's language-id head, ≈ 20 MB transient). If detected ≠ configured, swap language pack within 1 utterance.
- **Fallback:** Unknown detection → fall back to operator's configured locale + log via predecessor §0.2 invariant 8 ("language detection best-effort, not blocking").

### 4.5 Wake-word per-locale

Each `wake-leah-<lang>.mlmodel` is trained on the local-language phonology of "Leah" plus ~200 negative samples from native speakers in that language. Ships as part of app bundle (adds ≈ 18 MB total across 6 models).

Wake-word detection runs continuously against the *currently-loaded* language model only. Switching languages reloads the wake model (≤ 200 ms transient).

### 4.6 Data model

```sql
CREATE TABLE voice_lang_pack (
    locale          TEXT PRIMARY KEY,
    wake_model_path TEXT NOT NULL,
    stt_hint        TEXT NOT NULL,
    tts_voice_local TEXT NOT NULL,
    tts_voice_cloud TEXT NOT NULL,
    enabled         INTEGER NOT NULL DEFAULT 1
);

CREATE TABLE voice_lang_pref (
    id              INTEGER PRIMARY KEY CHECK(id = 1),
    locale          TEXT NOT NULL REFERENCES voice_lang_pack(locale),
    auto_detect     INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE voice_lang_detection (
    id              INTEGER PRIMARY KEY,
    session_id      INTEGER NOT NULL REFERENCES voice_session(id) ON DELETE CASCADE,
    detected        TEXT NOT NULL,
    confidence      REAL NOT NULL,
    applied         INTEGER NOT NULL,    -- 1 if pack-swap happened
    at              INTEGER NOT NULL
);

CREATE TABLE voice_string (
    locale          TEXT NOT NULL,
    key             TEXT NOT NULL,         -- e.g. 'nudge.too_quiet'
    value           TEXT NOT NULL,
    PRIMARY KEY(locale, key)
);
```

### 4.7 Security model

- **Wake-word model integrity** — each locale model SHA-256 verified at load; mismatch refuses load.
- **STT fallback path (cloud Whisper)** — when used in non-en locale, the cloud-fallback audio carries language hint; OpenAI Whisper API has equal handling for all listed languages. No new privacy surface vs Phase 4 §1.5.
- **System-strings table** is operator-readable + operator-editable (Settings → Voice → "Edit system strings"). Allows operator to override any nudge phrasing without app update.
- **Threat model** — auto-detect mode does not introduce new audio exfiltration: detection runs locally on the same Whisper-distil-small that handles STT.

### 4.8 Failure modes

| Failure | Detection | Degraded behavior |
|---|---|---|
| Wake model for chosen locale fails to load | Boot check | Fall back to en-US wake model + log; surface in Settings as "Wake model failed for ja-JP — using en-US" |
| Detector misclassifies (e.g. ja-JP → en-US) | Operator manual switch | Operator overrides; log mis-classification + downweight detector confidence threshold |
| TTS cloud has no voice for locale | API 404 | Fall back to local Apple voice for that locale (always available on macOS) |
| Operator speaks a language not in supported set | Detector returns low-confidence | Fall back to configured locale; nudge: "I don't recognize that language yet" |
| System-strings key missing for chosen locale | Lookup miss | Fall back to en-US string for that key + log |

### 4.9 Performance budget

| Surface | RAM target | CPU target | Latency target |
|---|---|---|---|
| Multi-language STT (distil-small) | 150 MB resident | < 2.5% steady | First partial ≤ 450 ms (per §2.5) |
| Language detection (per session start) | 20 MB transient | < 5% × 150 ms | ≤ 200 ms |
| Wake-word swap (locale change) | n/a | < 8% × 200 ms | ≤ 250 ms |
| Localized TTS (Apple voice) | 30 MB | < 4% × duration | TTFB ≤ 300 ms (slight regression vs Phase 4 en) |
| Localized TTS (ElevenLabs cloud) | 8 MB | < 1% | TTFB ≤ 200 ms |

### 4.10 UI surfaces

- **Settings → Voice → "Voice language"** — dropdown listing supported locales. Default = OS primary language if supported, else en-US.
- **Settings → Voice → "Detect language per session"** — opt-in toggle.
- **Settings → Voice → "Wake phrase"** — read-only "wake-leah" for v1.2 (custom phrase training deferred).
- **Wizard step 4** — language picker added; defaults to OS primary language.
- **HUD focus panel** — all system strings (placeholders, errors, nudges) render in the configured locale.
- **Voice nudges** ("I didn't catch that") render in the *detected* language when auto-detect is on.

### 4.11 What ships in v1.2 vs deferred

**Ships:**
- 6 language packs (en, ja, es, fr, de, zh-CN)
- Per-locale wake model + STT hint + TTS routing + system strings
- Operator-set + auto-detect
- Wizard language picker
- Settings → Voice editable strings table

**Deferred to v1.3+:**
- **Additional locales** (pt-BR, ko-KR, hi-IN, ar-SA, etc.) — distil-small covers them at STT level; need wake-word retraining + TTS voice IDs + string table. Add 2–3 per release cadence.
- **Code-switching mid-utterance** ("English with Spanish embedded loanwords"). distil-small handles loanwords reasonably; first-class code-switch detection waits.
- **Custom wake-phrase training UI.** Predecessor §1.9 deferral; remains deferred — model file replacement still requires reinstall. Revisit in v1.4 with on-device training infra.
- **Localized voice persona library.** Per §0.1 deferred; remains deferred.

---

### 4.12 System-strings registry

Phase 5 introduces ~80 user-visible UI strings (focus panel placeholders, settings labels, error toasts, nudges). For each locale, the table `voice_string` holds the localized form. Default English seeds:

| Key | en-US |
|---|---|
| `nudge.too_quiet` | "I couldn't hear that — speak up?" |
| `nudge.too_long` | "Long thought — break it up if you can." |
| `nudge.barge_ack` | "Got it." |
| `error.mic_unavailable` | "Microphone isn't available right now." |
| `error.wake_failed` | "Wake-word model failed to load." |
| `wizard.lang.title` | "What language do you speak to Leah?" |
| `settings.voice.detect` | "Detect language each session" |
| `settings.voice.calib` | "Calibrate voice" |
| ... | ... |

Translations are commissioned from the same translation service used for predecessor §1.9 system strings (operator-controlled budget, paid out-of-band; not Leah revenue). Operator may edit any key in Settings → Voice → Edit system strings — the row override wins.

### 4.13 Locale-mismatch handling

If operator selects voice locale ja-JP but the OS primary language is en-US:

- All HUD chrome stays in the OS primary language (en-US). System strings only swap inside voice flows.
- This prevents the "operator changed voice lang and now everything is Japanese" surprise.
- Operator who wants full UI localization: macOS System Settings → Language & Region (which Leah respects via the OS bundle).

### 4.14 Streaming TTS in non-English locales

Apple's Premium voices for non-English vary in TTFB:

| Locale | Voice | TTFB target | Notes |
|---|---|---|---|
| en-US | Ava Premium | 250 ms | Phase 4 baseline |
| ja-JP | Kyoko Premium | 290 ms | Slightly higher cold-start |
| es-ES | Mónica Premium | 280 ms | |
| fr-FR | Amélie Premium | 280 ms | |
| de-DE | Anna Premium | 270 ms | |
| zh-CN | Tingting Premium | 320 ms | Highest TTFB; pre-warm helps |

For zh-CN especially, the daemon pre-warms the TTS engine at voice-session start to keep first-utterance TTFB ≤ 300 ms.

### 4.15 Locale switching mid-session

Mid-session locale switch (operator started in en-US, the auto-detector flips to es-ES on first utterance) requires:

1. Halt current STT stream (no partial loss; final emit if any).
2. Swap STT model: en-US distil-large-v3 → multi distil-small.
3. Swap wake model: en → es.
4. Swap TTS voice: Ava → Mónica.
5. Resume listening.

Total swap latency: ~250 ms (per §4.9 row 3). Operator sees a brief pause + a quiet "(switched to es-ES)" indicator in the focus panel.

Mid-utterance swap is NOT supported — auto-detect runs on the first 2 s, so swap happens before the operator's full sentence is in.

### 4.16 ElevenLabs voice routing per locale

ElevenLabs Flash v2.5 supports multilingual generation. The voice-ID-per-locale table:

| Locale | Voice ID (placeholder) | Voice description |
|---|---|---|
| en-US | `21m00Tcm4TlvDq8ikWAM` (Rachel) | English female, warm |
| ja-JP | `<japanese voice id>` | Japanese female |
| es-ES | `<spanish voice id>` | Spanish female |
| fr-FR | `<french voice id>` | French female |
| de-DE | `<german voice id>` | German female |
| zh-CN | `<mandarin voice id>` | Mandarin female |

Voice IDs are placeholder pending W2-T3 selection; operator may override any in Settings.

### 4.17 Hybrid utterances (mixed-locale)

If operator speaks an utterance like "Schedule a 1:1 con Maria for 3pm" (mixed English + Spanish):

- Auto-detect (if on) might flip on the first 2 s. If it lands on en-US, the distil-small multi-language path still transcribes Spanish words correctly (it's trained multi-lingually); just the wake/TTS stay English.
- If it lands on es-ES, the English words ("Schedule a 1:1") get transcribed Spanish-tone but still recognizable.
- Operator-feedback loop: if mis-transcription rate climbs, the operator can disable auto-detect and stick with their primary locale.

True first-class code-switching detection is deferred to v1.3+ per §4.11.

## 5. Deliverable 5 — macOS 16 (Tahoe) compatibility

### 5.1 Goal

Apple's macOS 16 (codename Tahoe, public release Q4 2026) introduces API deltas Leah must absorb before v1.2 ships, plus new affordances (system-level Live Activities for menubar apps, refined accessibility APIs, new audio session modes) Leah can opt into. Ship gate: app passes Apple's macOS 16 validation suite + CI green on a Tahoe runner + every Phase 5 PR runs the Tahoe smoke test before merge.

This is leverage-5 because shipping a v1.2 in late 2026 that requires "macOS 15 only" is a launch-day own-goal. The compatibility work is unglamorous but binding.

### 5.2 Surface inventory (macOS 16 deltas to absorb)

| Surface | macOS 16 delta | Leah action |
|---|---|---|
| `NSStatusItem` | New scene-based status item lifecycle | Migrate menubar code to scene API; preserve fallback for macOS 15 |
| `AVAudioEngine` | New `playAndRecord` mode for full-duplex; deprecates `recordOnly` | Voice runtime adopts new mode; gates via `available(macOS 16)` |
| `Vision.framework` | `VNRecognizeTextRequest` revision 4 (faster + better non-Latin scripts) | OCR path (predecessor §4.4) prefers revision 4 on Tahoe; revision 3 fallback for macOS 15 |
| `Speech.framework` | New on-device transcription provider | Evaluate vs Whisper-distil; v1.2 keeps distil as default; SFSpeechRecognizer fallback updated to new provider |
| `CGDisplayStream` | Deprecated; replaced by `SCStream` + `SCStreamFrameInfo` | Live screen (§7) targets `SCStream` directly; no `CGDisplayStream` path on Tahoe |
| Accessibility | New `AXObservedAttribute` granular events | Per-app focus detection (predecessor §1) gains finer granularity on Tahoe |
| `Network.framework` | TLS 1.3 0-RTT default | Marketplace fetch (§1) opts in |
| `Combine` | Several deprecations | Audit; migrate to `AsyncSequence` where flagged |
| Sparkle | Sparkle 2.7 required for Tahoe sign+notarize | Bump Sparkle dep; re-test auto-update flow |
| App sandbox entitlements | New `com.apple.security.network.client.bonjour-only` | Multi-device sync (predecessor §2) opts into narrower entitlement |
| `NSPanel` | New `.hudWindow` style mask attributes | Focus panel (predecessor) audits for visual regression |
| `LSApplicationCategoryType` | New `productivity.assistant` category | App Store metadata (if ever submitted) gets new category |

### 5.3 Interfaces

No new Go interfaces. Swift code uses `if #available(macOS 16, *)` guards extensively; daemon code uses runtime version detection where the macOS API matters (audio session mode, screen capture provider).

```swift
// LeahHUD/Compat/Compat.swift
enum OSVersion {
    case macOS15
    case macOS16
    case future(major: Int, minor: Int)
}

protocol CompatLayer {
    var version: OSVersion { get }
    func makeStatusItem() -> StatusItemProvider
    func makeScreenCapture() -> ScreenCaptureProvider
    func makeAudioSession() -> AudioSessionProvider
    // ... per-surface providers chosen at runtime
}
```

### 5.4 CI integration

- New CI runner: `macos-tahoe-15-pro` (Apple Silicon, macOS 16). Every Phase 5 PR runs the existing test suite + a Tahoe-only smoke test that exercises menubar, hotkey, voice round-trip, screen capture, OCR, marketplace install.
- Existing CI runner (`macos-sequoia-15` for macOS 15) remains; PR must pass *both* before merge.
- A "Tahoe-red" run blocks merge regardless of other approvals — per §0.2 invariant 10.

### 5.5 Data model

No new tables. Existing schema unchanged.

### 5.6 Security model

- **Entitlement minimization.** Tahoe lets Leah request narrower entitlements (e.g. `bonjour-only` for sync); Phase 5 migrates to the narrower set wherever available.
- **TLS 1.3 0-RTT** for marketplace fetch — replay-safe only because marketplace requests are idempotent GETs of static content.
- **Sparkle 2.7** — re-verify EdDSA signature gate on Tahoe; pinned signing key unchanged.

### 5.7 Failure modes

| Failure | Detection | Degraded behavior |
|---|---|---|
| Running on macOS 15 (older) | Boot version check | Use Phase 4 code paths; do not enable Tahoe-only features (e.g. revision 4 OCR, narrower entitlements) |
| Running on macOS 17+ (future) | Boot version check | Best-effort: assume Tahoe APIs; log "future OS" warning; do not enable v1.2 — wait for next Leah release |
| Apple revokes / breaks a Tahoe API in a point release | CI Tahoe runner fails | Block merge; investigate; ship hotfix |
| `SCStream` permission revoked mid-session | Permission callback | End screen-capture session cleanly; surface "Screen recording permission lost — re-grant in System Settings" |

### 5.8 Performance budget

| Surface | Tahoe target | Sequoia baseline | Notes |
|---|---|---|---|
| Cold start | ≤ 1.2 s | ≤ 1.4 s | Tahoe app launch faster via new scene API |
| `SCStream` setup | ≤ 80 ms | n/a (uses CGDisplayStream) | Replaces CGDisplayStream with lower per-frame copy cost |
| `VNRecognizeTextRequest` rev4 (1080p frame) | ≤ 110 ms | rev3 ≤ 180 ms (Sequoia) | Per Apple benchmark; verified in CI |
| `AVAudioEngine` playAndRecord setup | ≤ 50 ms | ≤ 80 ms | Lower-latency duplex |

### 5.9 UI surfaces

No new UI. Visual parity with macOS 15 is the goal; any Tahoe-only visual change documented in `CHANGELOG.md` and audited against the v1 visual identity lock (predecessor §0.1).

### 5.10 What ships in v1.2 vs deferred

**Ships:**
- Full macOS 16 (Tahoe) compatibility — passes Apple's validation suite
- Adoption of new APIs: `SCStream`, `VNRecognizeTextRequest` revision 4, `AVAudioEngine` new mode, `Vision.framework` updates, Sparkle 2.7
- Narrower app-sandbox entitlements where available
- CI Tahoe runner enforced as merge gate

**Deferred to v1.3+:**
- **Apple Intelligence integration via `SystemLanguageModel` API.** Tahoe exposes on-device Apple foundation model. Useful as a cost-zero backup for low-stakes prompts, but the model is much smaller than Sonnet/Haiku and quality lift is unverified for Leah's prompt mix. Defer.
- **macOS 16 widget kit extensions** (system Lock Screen widgets, etc.). Out of scope; Leah's HUD widget surface (predecessor §10) is distinct from system widgets.
- **macOS 16-exclusive Live Activities for menubar.** Investigate; ship in v1.3 if the surface adds value for sync-toast or marketplace-update events.

---

### 5.11 Availability-guard pattern

Phase 5 Swift code uses two patterns:

```swift
// Pattern 1 — runtime branch
func makeStatusItem() -> StatusItemProvider {
    if #available(macOS 16, *) {
        return TahoeStatusItem()
    } else {
        return SequoiaStatusItem()
    }
}

// Pattern 2 — compile-time partial (when types are 16-only)
@available(macOS 16, *)
final class TahoeStatusItem: StatusItemProvider {
    // ... uses macOS 16 APIs without guards inside
}
```

CI lints for raw `@available` without explicit `*` (universal availability) and for runtime branches that fall through to no fallback.

### 5.12 Sequoia / Tahoe code-divergence ledger

W1-T1 spec adds a per-surface table to `docs/engineer/runbooks/tahoe-compat.md`:

| Surface | Sequoia path | Tahoe path | Test coverage |
|---|---|---|---|
| Menubar status item | `NSStatusBar.system.statusItem(...)` legacy lifecycle | New scene-based status API | Smoke: menubar item appears in both CI runners |
| Screen capture | `CGDisplayStreamCreate` | `SCStream` | Smoke: 0.5 fps sample works in both |
| Audio session | `AVAudioEngine.recordOnly` | `AVAudioEngine.playAndRecord` new mode | Voice round-trip in both |
| OCR | `VNRecognizeTextRequest` rev 3 | rev 4 | OCR latency benchmark in both |
| ... | ... | ... | ... |

Each divergence row must list test coverage that exercises both paths. PRs that add a Tahoe-only path without a Sequoia fallback fail review.

### 5.13 Tahoe-runner provisioning

CI runner `macos-tahoe-15-pro` provisioned via GitHub-hosted runner pool (when Tahoe enters GA on hosted) or self-hosted on a dedicated Mac mini M2 running macOS 16. Self-hosted path:

1. Mac mini M2, 16 GB RAM, 512 GB SSD.
2. macOS 16 + Xcode 17 + Apple Developer cert.
3. GitHub Actions self-hosted runner installed; labeled `macos-tahoe-15-pro`.
4. Auto-update macOS pinned to "Don't auto-install minor versions during business hours."
5. Runner runs only on Phase 5 branches + main; isolated from Phase 4 builds.

If GitHub provides macOS 16 hosted runners before W1-T1, switch to that.

### 5.14 Tahoe smoke-test suite

`make tahoe-smoke` runs:

1. App launch + Sparkle no-update-check.
2. Menubar status item present.
3. ⌥Space focus panel opens within 80 ms.
4. Wizard step 1-6 walkthrough.
5. Voice "Hey Leah, what time is it?" round-trip.
6. `/look` on a sample screenshot.
7. `/capture` on a sample screen → memory write.
8. Marketplace search returns ≥ 1 result.
9. Screen-watch session start + 5 fps for 30 s.
10. App quit (clean teardown).

Each step has a pass/fail gate. Total runtime ≤ 5 min.

### 5.15 Apple Intelligence stance

macOS 16 ships an on-device foundation model via `SystemLanguageModel` API. Leah's stance:

- **Don't depend on it** for any user-facing reasoning in v1.2 (per §5.10 deferral). Anthropic Sonnet/Haiku/Opus stays canonical.
- **Audit the API** in W1-T1; document model size, latency, capability boundary.
- **Consider for v1.3** as a zero-cost backup for low-stakes prompts (e.g. classifier in §6.4 might run via Apple model when offline; today it requires Haiku).

Decision rationale: Apple's model is much smaller than Haiku; quality lift unverified for Leah's prompt mix. Defer.

### 5.16 macOS 17 future-readiness

Phase 5 cannot anticipate macOS 17 deltas, but the compat layer (§5.3) is generic. When macOS 17 ships:

1. Add a new case to `OSVersion`.
2. Add a runtime branch in `CompatLayer` factory methods.
3. Audit each Phase 5 surface for new deprecations.
4. CI adds a `macos-17` runner.

The pattern carries forward indefinitely; phase specs do not re-define the compat structure each release.

## 6. Deliverable 6 — Vision OCR → memory ingest

### 6.1 Goal

Phase 4 §4 ships OCR-on-demand (`/read` command on screenshot). Phase 5 makes OCR ingest a first-class memory write path: operator captures a screen (or selection), Leah extracts the text, classifies it, and writes selected blocks to memory with structured metadata (source app, timestamp, OCR confidence, operator-tag). Future `leah ask` can recall the captured content semantically.

This is leverage-6 because the operator's screen is the highest-signal-density information surface they touch all day, and Phase 4 already captures it on demand — the missing primitive is *durable storage with retrieval*. The 2026 capability inventory (`memory/research_ai_capability_domains_2026.md`) cites Limitless as the canonical "passive screen memory" reference; Limitless was absorbed by Meta and the product is in decline. Leah occupies the operator-owned-equivalent position.

### 6.2 Capability matrix

| Capability | Phase 4 baseline | Phase 5 addition |
|---|---|---|
| OCR on demand | `/read` command returns text in panel | + write to memory with classifier-curated subset |
| OCR confidence | Per-block confidence available | + threshold gating (`< 0.6` blocks skip ingest) |
| Memory write | Manual via `leah remember` | + auto-write on `/capture` command |
| Tag inference | Manual | + tag inference from source app (Slack → "slack", Mail → "mail", Chrome → URL host) |
| Search | `leah ask` over manually-tagged memory | + recall OCR-ingested content with source context |

### 6.3 Interfaces

```go
// internal/vision/ingest.go
type OCRIngester interface {
    // Capture + OCR + classify + write. Idempotent within a 60 s window
    // (same screen state does not double-write).
    Capture(ctx context.Context, mode CaptureMode, opts IngestOpts) (IngestReceipt, error)
    Reingest(ctx context.Context, imagePath string, opts IngestOpts) (IngestReceipt, error)
}

type IngestOpts struct {
    SourceAppHint string         // e.g. "com.tinyspeck.slackmacgap"
    OperatorTags  []string
    MinConfidence float64        // default 0.6
    MaxBlocks     int            // default 50
    Classify      bool           // run classifier? default true
}

type IngestReceipt struct {
    EventID       int64
    BlocksWritten int
    BlocksSkipped int               // low confidence or duplicate
    TagsAssigned  []string
    MemoryRowIDs  []int64
}
```

### 6.4 Classifier

Lightweight Haiku 4.5 call (predecessor §17.14 routing) given OCR text + source app context; outputs:

```jsonc
{
  "memoryWorthy": true | false,
  "category": "conversation" | "code" | "reading" | "form" | "ui-chrome" | "other",
  "tags": ["slack", "phase5"],
  "summary": "Discussion of marketplace plugin pricing model",
  "redactSpans": [[100, 134]]   // PII byte ranges within the OCR text
}
```

- `memoryWorthy=false` for `ui-chrome` and `form` categories by default (operator can opt in via Settings).
- `redactSpans` blanks credit-card-shaped, SSN-shaped, and API-key-shaped substrings before write.
- Classifier latency target: ≤ 600 ms p95.

### 6.5 Data model

```sql
CREATE TABLE memory_capture (
    id            INTEGER PRIMARY KEY,
    at            INTEGER NOT NULL,
    source_app    TEXT,                 -- bundle ID
    source_url    TEXT,                 -- when source is a browser
    mode          TEXT NOT NULL,        -- 'screenshot' | 'selection' | 'live_screen'
    image_sha256  BLOB NOT NULL,
    thumb_path    TEXT NOT NULL,        -- 256-px JPEG; auto-pruned after 30 d
    ocr_text      TEXT NOT NULL,        -- full OCR for forensic; redacted form goes to memory rows
    classifier_json BLOB,
    blocks_written INTEGER NOT NULL DEFAULT 0,
    classifier_ms  INTEGER
);

CREATE TABLE memory_capture_block (
    id            INTEGER PRIMARY KEY,
    capture_id    INTEGER NOT NULL REFERENCES memory_capture(id) ON DELETE CASCADE,
    memory_row_id INTEGER NOT NULL REFERENCES memory(id) ON DELETE CASCADE,
    block_text    TEXT NOT NULL,
    confidence    REAL NOT NULL,
    bbox          BLOB NOT NULL,         -- packed [x,y,w,h] in image pixels
    redacted      INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE memory_capture_rate (
    bucket_hour   INTEGER PRIMARY KEY,    -- floor(at / 3600)
    count         INTEGER NOT NULL DEFAULT 0
);
```

The existing `memory` table (Phase 1) gains no new columns; OCR-ingested rows look identical to any other memory row, just with the extra `memory_capture_block` join.

### 6.6 Security model

- **Default-OFF.** Operator opts in via Settings → Memory → "Capture screen text to memory". Wizard does NOT include this toggle.
- **Per-app blocklist.** Settings → Memory → Capture → "Never capture from these apps" — defaults to Password Manager apps (1Password, Bitwarden), Banking apps (a known-list), and "Private Browsing" Chrome/Safari windows (detected via Accessibility API).
- **PII redaction at write time.** Classifier `redactSpans` enforced. A `memory_capture_block` row can have `redacted=1`, and the corresponding `memory.text` is the post-redaction form.
- **Image retention.** Raw image thumbnail at 256 px JPEG kept 30 days for operator audit; full-res discarded immediately. Full OCR text kept indefinitely (it is the memory content).
- **Rate limit (per §0.2 invariant 11).** ≤ 200 memory rows / hour by default; configurable Settings → Memory → "Screen capture ingest rate".
- **Privacy budget.** Each ingest event counts against the vision privacy budget per Phase 4 §8.
- **Operator scrub.** Settings → Memory → "Scrub last N captures" wipes both `memory_capture` rows and the joined `memory` rows transactionally.

### 6.7 Failure modes

| Failure | Detection | Degraded behavior |
|---|---|---|
| OCR fails (image unreadable) | Vision.framework error | Skip ingest; surface error toast |
| Classifier returns `memoryWorthy=false` | Classifier output | Discard; record `blocks_written=0` for forensic |
| Rate limit hit | `memory_capture_rate` bucket | Reject with toast "Ingest rate limit hit. Settings → Memory → Capture to adjust." |
| App is on blocklist | Pre-capture check | Refuse ingest; no record kept |
| Classifier API call times out | 2 s timeout | Best-effort: write all OCR blocks with confidence ≥ 0.6, no classification tags |
| Memory table grows > operator threshold | Settings cap | Surface notification; offer "Auto-prune captures > 90 d" |
| OCR contains password-shaped substring missed by classifier | Heuristic post-filter | Refuse write; log via OSLog without text content |

### 6.8 Performance budget

| Surface | RAM target | CPU target | Latency target |
|---|---|---|---|
| Screenshot capture | n/a | n/a | ≤ 30 ms |
| OCR (1080p screen, rev 4 on Tahoe) | 80 MB transient | < 10% × 110 ms | ≤ 130 ms |
| Classifier (Haiku, redacted OCR text) | n/a | < 1% × 600 ms | ≤ 700 ms |
| Memory write (10 blocks) | 4 MB transient | < 2% × 50 ms | ≤ 60 ms |
| End-to-end capture → searchable | n/a | combined | ≤ 90 s p95 (includes embedding job) |
| Search (`leah ask` over captured) | n/a | < 2% × 80 ms | ≤ 100 ms |

### 6.9 UI surfaces

- **HUD focus panel** — new `/capture` command (alongside `/read`); operator types after a screenshot. Optional inline tags.
- **HUD widget** — "Recent captures" widget (opt-in); 3-card stack with thumbnail + summary.
- **Settings → Memory → Capture pane** — toggle, blocklist, rate limit, "Scrub last N", retention policy.
- **`leah ask` CLI** — captures surface as memory rows with `source: screen-capture` tag.
- **Notification widget** — `capture.scrubbed` event when an operator-initiated scrub completes ("Removed 47 captured rows").

### 6.10 What ships in v1.2 vs deferred

**Ships:**
- `/capture` command + IPC + ingest pipeline
- Haiku classifier + PII redaction
- Per-app blocklist (defaults populated)
- Rate limit + operator scrub
- Settings → Memory → Capture pane
- Search integration via existing memory store

**Deferred to v1.3+:**
- **Continuous passive ingest** (Limitless-style "everything you see"). The §7 live-screen-reasoning surface is in this phase; passive *write* is not. The privacy + retention envelope is a larger discussion; ship the on-demand path first, learn from operator usage.
- **OCR for video** (paused video frame ingest). Niche; defer.
- **Image-content classification beyond OCR text** (e.g. "this is a code editor, classify as code"). Bundle into v1.3 if it lifts classifier precision meaningfully.
- **Cross-platform OCR** (e.g. Linux). v2 horizon.

---

### 6.11 Idempotency

The OCR ingest path is idempotent within a 60 s window per screen state:

- Each capture computes `image_sha256` of the captured image.
- Before write, daemon checks `SELECT 1 FROM memory_capture WHERE image_sha256 = ? AND at > (now - 60)`.
- Hit → return existing `IngestReceipt` without re-running OCR or classifier.
- Miss → proceed with full pipeline.

This prevents double-write if operator rapidly retypes `/capture` on the same screen.

### 6.12 Source-app + source-URL inference

When the capture mode is `screenshot` or `selection`, the daemon attempts to infer source app:

1. `NSWorkspace.frontmostApplication.bundleIdentifier` at capture moment.
2. If the frontmost app is a browser (Safari/Chrome/Brave/Arc), Accessibility API reads the active tab URL.
3. Both `source_app` and `source_url` get stored.

Privacy: the URL is stored verbatim in `memory_capture.source_url`. Operator-defined redact rules can blank URLs matching patterns (Settings → Memory → Capture → "URL redact list").

### 6.13 Tag inference detail

Source-app → default tag mapping:

| Bundle ID | Default tag |
|---|---|
| `com.tinyspeck.slackmacgap` | `slack` |
| `com.apple.mail` | `mail` |
| `com.google.Chrome` | `web` + URL host |
| `com.apple.Safari` | `web` + URL host |
| `com.microsoft.VSCode` | `code` |
| `com.apple.dt.Xcode` | `code` |
| `com.figma.Desktop` | `design` |
| `notion.id` | `notes` |
| `com.linear.linear` | `linear` |
| `<other>` | `(none)` |

Operator can edit the table (Settings → Memory → Capture → "Source tag map").

### 6.14 OCR block ordering

Vision.framework returns recognized text blocks in arbitrary order. The daemon re-orders before classification:

1. Sort by bounding-box `y` ascending (top-to-bottom).
2. Within rows (y difference < 20 px), sort by `x` ascending (left-to-right).
3. Concatenate with `\n` between rows, single space within a row.

This yields reading-order text suitable for the classifier prompt.

### 6.15 Confidence threshold

`MinConfidence = 0.6` default. Per-block confidence comes from `VNRecognizedText.confidence` (0–1 scale). Blocks below threshold are dropped before classification — they would only confuse the classifier with garbled text. Operator can lower the threshold in Settings (e.g. for low-resolution screens or unusual fonts).

### 6.16 Capture promote from screen-watch

When §7 screen-watch fires a signal and the operator clicks "Save to memory" on the toast, the daemon:

1. Finds the latest `screen_watch_signal` row.
2. Retrieves `ocr_excerpt` and `thumb_path`.
3. Constructs an `IngestOpts` from the signal context (rule name → tag).
4. Calls `OCRIngester.Reingest(ctx, thumb_path, opts)`.

This is the only path where screen-watch writes to memory; default behavior is forget-the-frame.

### 6.17 Memory growth bounds

Operator-configurable in Settings:

- `Settings → Memory → Auto-prune captures > N days` — default 90 days.
- `Settings → Memory → Cap total capture rows at N` — default 100 000.

Both checked on each ingest; when over cap, oldest captures evict (cascade-deletes joined memory rows). Operator-tag-protected captures (operator manually tagged `keep-forever`) bypass the prune.

### 6.18 Capture surfacing in HUD

The "Recent captures" widget shows the last 3 captures with thumbnail + summary. Click → opens a detail sheet showing full OCR text + classifier output + memory rows. From the detail sheet, operator can:

- Delete this capture (cascades the joined memory rows).
- Re-classify (re-run Haiku on the OCR text — useful if the classifier mis-categorized).
- Add operator tags.

The widget is opt-in (Settings → Widgets → "Recent captures") to keep default HUD chrome minimal.

## 7. Deliverable 7 — Live screen reasoning

### 7.1 Goal

Phase 4 §4 enables `/look` (screenshot reasoning) on demand. Phase 5 §7 extends that to a continuous, low-rate screen-stream reasoning: while enabled, Leah samples the screen at 0.5 fps, runs lightweight delta detection, and proactively offers contextual help when the operator pauses on a recognizable surface (e.g. an error dialog, an unread Slack thread, a JIRA ticket). The 2026 capability inventory (`memory/research_ai_capability_domains_2026.md`) cites Gemini Live as the canonical "vision + screen-context" reference; Leah's local-first equivalent fills the gap.

This is leverage-7 because it transforms Leah from a "summon-me" tool to a "watch-with-me" companion *only when the operator wants*. The HUD glyph + default-OFF + power budget keep it from being creepy.

### 7.2 Modes

| Mode | Trigger | Frame rate | Default |
|---|---|---|---|
| Off | n/a | 0 fps | default |
| On-foreground | Toggle in Settings; active only while Leah HUD frontmost | 0.5 fps | OFF |
| On-always | Toggle + confirm dialog | 0.5 fps | OFF (requires double-opt-in) |

### 7.3 Interfaces

```go
// internal/vision/screenwatch.go
type ScreenWatcher interface {
    // Start begins continuous screen sampling at the configured fps.
    // Returns a channel of events (deltas detected, signals fired).
    Start(ctx context.Context, opts WatchOpts) (<-chan WatchEvent, CancelFunc, error)
    Status() WatchStatus
}

type WatchOpts struct {
    FPS              float64       // default 0.5
    Mode             WatchMode     // OnForeground | OnAlways
    SignalRules      []SignalRule  // operator-editable
    PerAppMute       []BundleID
    EnergyBudgetMW   int           // soft cap; daemon throttles to honor
}

type WatchEvent struct {
    At         time.Time
    Kind       WatchEventKind   // FrameSampled | DeltaDetected | SignalFired | Throttled | Stopped
    SignalName string           // when Kind=SignalFired; e.g. "error-dialog"
    Thumb      []byte           // 256-px JPEG; only when signal fires
    OCRText    string           // small excerpt; only when signal fires
    Reasoning  string           // assistant suggestion; truncated to 240 chars
}

type SignalRule struct {
    Name        string
    Trigger     TriggerKind   // OCRMatch | UIClassMatch | DeltaSurfaceClass
    Pattern     string        // regex for OCRMatch
    SuppressFor time.Duration // dedupe window
}
```

### 7.4 Signal rules

Operator-editable list of triggers. Each rule, when fired, causes the HUD to surface a small toast with the suggestion. Default rules ship in v1.2:

| Name | Trigger | Example |
|---|---|---|
| `error-dialog` | UIClassMatch: `AXSheet` containing words "error", "failed" | "Want me to look up this error?" |
| `unread-slack-thread` | OCRMatch in Slack: "(N new) replies" with N > 5 | "Thread has 7 new replies. Summarize?" |
| `jira-ticket-open` | URL match Chrome/Safari: `*atlassian.net/browse/*` | "Pull related context from memory?" |
| `code-stack-trace` | OCRMatch: `Traceback|at .*\(.*:[0-9]+\)` | "Decode this stack trace?" |
| `meeting-agenda-open` | URL match: gcal/zoom meeting in progress | "Take notes during this meeting?" |

Operator can add/remove/edit rules in Settings → Vision → Signals.

### 7.5 Reasoning routing

- **Local-first.** Delta detection runs locally (perceptual hash of consecutive frames). Only when a signal fires does a Haiku 4.5 call run on the OCR excerpt — never the full frame.
- **No frame leaves the Mac unless** the signal-firing rule explicitly requests `mode: VisionSonnet` (per Phase 4 §4.3) — e.g. operator hits the suggestion button. Even then, only the *signal-cropped* region is sent.
- **Privacy budget.** Every signal-fired event counts against the vision privacy budget per Phase 4 §8.

### 7.6 Data model

```sql
CREATE TABLE screen_watch_session (
    id            INTEGER PRIMARY KEY,
    started_at    INTEGER NOT NULL,
    ended_at      INTEGER,
    mode          TEXT NOT NULL CHECK(mode IN ('on_foreground','on_always')),
    fps           REAL NOT NULL,
    frames_sampled INTEGER NOT NULL DEFAULT 0,
    signals_fired INTEGER NOT NULL DEFAULT 0,
    energy_mwh    INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE screen_watch_signal (
    id            INTEGER PRIMARY KEY,
    session_id    INTEGER NOT NULL REFERENCES screen_watch_session(id) ON DELETE CASCADE,
    rule_name     TEXT NOT NULL,
    fired_at      INTEGER NOT NULL,
    accepted      INTEGER,                 -- 1 if operator accepted suggestion
    ocr_excerpt   TEXT,
    thumb_path    TEXT
);

CREATE TABLE screen_watch_rule (
    id            INTEGER PRIMARY KEY,
    name          TEXT NOT NULL UNIQUE,
    trigger_kind  TEXT NOT NULL,
    pattern       TEXT NOT NULL,
    suppress_s    INTEGER NOT NULL DEFAULT 60,
    enabled       INTEGER NOT NULL DEFAULT 1,
    is_default    INTEGER NOT NULL DEFAULT 0
);
```

### 7.7 Security model

- **Default-OFF + double-opt-in for `OnAlways`.** Predecessor §0.2 invariant 3 already covers default-OFF for ambient; Phase 5 §0.2 invariant 9 makes the HUD glyph mandatory whenever the stream is live.
- **HUD glyph is non-suppressible.** Operator cannot hide the "screen-watch active" indicator while screen-watch is active. Makes covert recording physically impossible from the UI.
- **Per-app mute.** Operator lists apps that screen-watch ignores entirely (Password Managers, banking, private browsing). When frontmost app is muted, sampling pauses.
- **Privacy budget cap.** ≤ 30 cloud-routed signals per 24 h by default. Operator can lift in Settings.
- **No raw frames stored.** Only the OCR excerpt + 256-px thumbnail of the *signal-firing* region is persisted. Sampled frames that did not fire a signal are discarded immediately.
- **Energy guard.** Daemon throttles fps if measured energy exceeds budget (≤ 3 W steady-state extra power draw on a M3 Pro baseline).

### 7.8 Failure modes

| Failure | Detection | Degraded behavior |
|---|---|---|
| Energy budget exceeded | Power-sample watcher | Throttle fps to 0.25; surface "Screen-watch energy use elevated — reducing sample rate" |
| Signal rule regex panics | Try/catch on match | Disable that rule; log; surface in Settings |
| Operator denies screen-recording permission | Permission callback | Disable feature; surface "Grant screen recording in System Settings" |
| Mute-app list violated (e.g. window classifier wrong) | Periodic re-check | Pause sampling immediately; log without content |
| Signal flood (> 10 fires in 60 s) | Counter | Suppress signals for 5 min; surface "Too many signals — paused" |
| Daemon restart mid-session | Watchdog (predecessor §9) | New session row; restart watcher; operator sees ≤ 2 s outage |

### 7.9 Performance budget

| Surface | RAM target | CPU target | Power target |
|---|---|---|---|
| Frame sample (0.5 fps, 1080p) | 18 MB | < 1.5% steady | < 1.5 W |
| Delta detection (perceptual hash) | n/a | < 0.5% steady | < 0.3 W |
| Signal match (rules engine) | 4 MB | < 0.3% steady | < 0.2 W |
| OCR on signal fire (rev 4 on Tahoe) | 80 MB transient | per §5.8 | per frame |
| Haiku call on signal | n/a | < 0.5% × 500 ms | < 0.1 W per call |
| **Total steady-state (idle screen)** | **22 MB** | **< 2%** | **< 3 W** |

### 7.10 UI surfaces

- **Settings → Vision → Screen watch (new pane)** — mode dropdown (Off / On-foreground / On-always), rule list (operator-editable), per-app mute, energy budget slider.
- **HUD glyph (mandatory while active)** — small eye icon in the focus panel chrome + a dot in the menubar item. Cannot be hidden.
- **Notification widget** — signal-fired events surface as toasts with "Accept" / "Dismiss".
- **Wizard** — NOT changed (per Phase 4 §0.2 invariant 1-revised: wizard shape stays fixed).
- **Power dialog** — first time `OnAlways` is enabled, modal sheet shows energy budget + opt-in confirmation.

### 7.11 What ships in v1.2 vs deferred

**Ships:**
- 0.5 fps continuous screen sampling (default OFF)
- 5 default signal rules
- Operator-editable rules + per-app mute
- HUD glyph + menubar dot
- Energy guard + sample-rate throttle
- Memory join: signal-fired events can be promoted to memory via `/capture` (§6 join)

**Deferred to v1.3+:**
- **Predictive UI overlay** (Leah draws on top of the screen). Adds compositor complexity; defer until 90 d of signal-firing telemetry shows it would help.
- **Multi-monitor support** (sample all displays). v1.2 samples the *main* display only; multi-monitor is engineering work + privacy-budget rework.
- **Per-window scoping** (sample only Slack, ignore everything else). Useful narrow case; achievable in v1.3 via mute list inverse.
- **Live audio + screen joint reasoning** (e.g. "in this meeting, take notes when speaker says X"). Powerful but cross-modal — defer; Phase 5 keeps screen and voice as separate reasoning surfaces.

---

### 7.12 Frame-sampling lifecycle

The sampler runs as a dedicated goroutine inside the daemon:

```go
// internal/vision/screenwatch/sampler.go
func (s *sampler) run(ctx context.Context, frames chan<- Frame) {
    ticker := time.NewTicker(time.Duration(1000.0/s.fps) * time.Millisecond)
    defer ticker.Stop()
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            if s.shouldSkip() { continue }  // mute-app, energy throttle
            frame, err := s.cap.Sample(ctx)
            if err != nil {
                s.errCounter.Add(1)
                continue
            }
            select {
            case frames <- frame:
            default:
                // drop oldest if downstream is behind
                s.dropCounter.Add(1)
            }
        }
    }
}
```

The sampler never blocks on downstream consumers — dropped frames are counted but acceptable at 0.5 fps. The OCR pipeline downstream typically catches up within the next frame interval.

### 7.13 Per-app mute classifier

To detect frontmost app + classify as "muted":

1. `NSWorkspace.frontmostApplication.bundleIdentifier` poll every 500 ms.
2. Compare against `screen_watch_rule` mute list.
3. If match → sampler enters mute state; pauses sampling.
4. Periodic re-check (every 5 s) confirms mute still applies.

Private browsing detection (Chrome/Safari):
- Chrome: read `incognito` boolean via DevTools protocol (not exposed locally without browser extension; defer).
- Safari: AppleScript `private browsing of front window` (requires automation permission).
- v1.2 fallback: detect window title pattern (Chrome "Private", Safari "Private Browsing"); add to mute when seen.

### 7.14 Signal-rule pattern language

OCRMatch rules use Go `regexp` syntax. To prevent ReDoS attacks via malicious patterns, the daemon:

- Compiles each pattern once at rule-add time; failure surfaces in Settings.
- Imposes a per-match timeout of 50 ms.
- If timeout exceeded, disable that rule + log + surface in Settings.

UIClassMatch rules walk the Accessibility tree of the frontmost app:

```
AXTitle ~= /error|failed/i and AXRole == "AXSheet"
```

DSL parsed at rule-add time; tree-walk runs per-signal-fire (rate-limited).

### 7.15 Cloud-route opt-in

When operator clicks the suggestion toast "Want me to look up this error?", the daemon:

1. Fetches the signal-firing region (last sampled frame, cropped to the OCR bbox).
2. Re-runs OCR on the cropped region (higher fidelity than the down-sampled stream OCR).
3. Submits cropped image + OCR text to Sonnet vision API.
4. Streams response back to HUD.

The cropping ensures only the signal-relevant region leaves the Mac — not the full screen.

Privacy-budget tick: each cloud-route counts once. Per-day cap §14.2.

### 7.16 Signal coalescing

If multiple rules fire on the same frame (e.g. a JIRA URL with an error dialog overlay), the daemon coalesces into a single toast:

- Highest-priority rule's suggestion text shown.
- Toast includes "+1 more" badge if other rules also fired.
- Click on toast → secondary sheet with the other rule's suggestion.

Priority order (configurable): `error-dialog` > `code-stack-trace` > `meeting-agenda-open` > `jira-ticket-open` > `unread-slack-thread`.

### 7.17 Operator-feedback loop

Each toast surfaces "Accept" / "Dismiss". Outcome wired back to the Phase 4 recommender (predecessor §3) as a `Recommendation.Outcome` row with kind `screen-watch-signal/<rule_name>`. Bandit rank then adjusts: rules dismissed N times in a row get demoted; rules accepted get boosted.

Operator can opt out (Settings → Vision → "Use signal outcomes to learn") — keeps rule priority static.

## 8. Cross-cutting data model (Phase 5 additions consolidated)

Summary of new tables introduced across §1–§7. All migrations are additive (Phase 4 + Phase 3 tables unchanged). Migrations live in `internal/db/migrations/` and follow the existing schema-versioning protocol.

| Migration | Tables | Deliverable |
|---|---|---|
| `2026-06-23-001-marketplace.sql` | `marketplace_index_cache`, `marketplace_plugin`, `marketplace_capability_grant`, `marketplace_revoke` | §1 |
| `2026-06-23-002-voice-distilled.sql` | `voice_model`, `voice_calibration`, `voice_wer_sample` | §2 |
| `2026-06-23-003-calendar.sql` | `calendar_source`, `calendar_event_cache`, `calendar_working_hours`, `calendar_redact_rule` | §3 |
| `2026-06-23-004-voice-lang.sql` | `voice_lang_pack`, `voice_lang_pref`, `voice_lang_detection`, `voice_string` | §4 |
| `2026-06-23-005-os-compat.sql` | (no new tables) | §5 |
| `2026-06-23-006-memory-capture.sql` | `memory_capture`, `memory_capture_block`, `memory_capture_rate` | §6 |
| `2026-06-23-007-screen-watch.sql` | `screen_watch_session`, `screen_watch_signal`, `screen_watch_rule` | §7 |

Total new tables: 22. Total new columns on existing Phase 1–4 tables: 0 (all Phase 5 changes are additive via new tables).

### 8.1 Foreign-key + ON DELETE policy

All Phase 5 tables use `ON DELETE CASCADE` where parent-child semantic is clear (session → events, capture → blocks). Otherwise `ON DELETE RESTRICT` (e.g. `marketplace_plugin → marketplace_capability_grant`). No `SET NULL` — matches Phase 1–4 convention.

### 8.2 Index policy

Per Phase 1 convention: every foreign key gets an index; every `*_at` timestamp column gets an index when scan patterns hit it. Phase 5 spec inventories the required indices in each deliverable's migration file (not enumerated here for brevity).

### 8.3 Sync model

Per predecessor §2.3 + §2.5: Phase 5 tables participate in multi-device sync where the row class is operator-state-bearing (settings registers, anti-recommend rules, working hours, redact rules). Operationally-ephemeral rows do NOT sync (Whisper calibration audio, screen-watch session logs, OCR thumbnails). Per-table sync flag is declared in the migration file via a `sync_class TEXT` column on a `sync_table_meta` lookup table (added in `2026-06-23-001-marketplace.sql`).

| Table | Sync class | Rationale |
|---|---|---|
| `marketplace_plugin` | sync | Operator wants the same plugins on both Macs |
| `marketplace_capability_grant` | sync | Capability decisions are operator-state |
| `marketplace_index_cache` | local-only | Index is fetchable; no sync benefit |
| `voice_model` | local-only | Model files don't sync; metadata derives from disk |
| `voice_calibration` | local-only | Operator's voice corpus is per-Mac (mic differs) |
| `voice_wer_sample` | local-only | Local diagnostic only |
| `calendar_source` | sync | Same accounts on both Macs |
| `calendar_event_cache` | local-only | Cache; re-derivable |
| `calendar_working_hours` | sync | Operator preference |
| `calendar_redact_rule` | sync | Operator preference |
| `voice_lang_pack` | local-only | Model file metadata |
| `voice_lang_pref` | sync | Operator preference |
| `voice_lang_detection` | local-only | Diagnostic |
| `voice_string` | sync | Operator customizations |
| `memory_capture` | local-only | High volume; per-Mac screen |
| `memory_capture_block` | local-only (joined to memory rows that DO sync) | Block storage is ephemeral; the memory rows it produces sync via existing memory-table policy |
| `memory_capture_rate` | local-only | Bucket counter |
| `screen_watch_session` | local-only | Per-Mac |
| `screen_watch_signal` | local-only | Per-Mac |
| `screen_watch_rule` | sync | Operator preference |

---

## 9. Wave breakdown (5 waves × 4-5 tasks = 23 tasks)

### Wave 1 — Foundations (compatibility + RAM headroom)

**Theme:** Land before any Phase 5 surface PR — every later wave assumes both.

| Task | Deliverable | Scope | Owner kind |
|---|---|---|---|
| W1-T1 | §5 OS compat | Set up macOS 16 (Tahoe) CI runner; add `available(macOS 16)` guard scaffold; smoke test scaffold | Single-owner CI |
| W1-T2 | §5 OS compat | Migrate `NSStatusItem`, `NSPanel`, Combine-deprecated paths to Tahoe-compatible code with macOS 15 fallback | Implementer (Swift) |
| W1-T3 | §5 OS compat | Adopt `SCStream` (replacing `CGDisplayStream`) + `AVAudioEngine` new mode + `VNRecognizeTextRequest` rev 4 (under availability guard) | Implementer (Swift) |
| W1-T4 | §2 Whisper distilled | Land Whisper-distil-large-v3 in app bundle; new `STT` implementation; default-swap behind feature flag | Implementer (Go) |
| W1-T5 | §2 Whisper distilled | Pressure fallback (distil-medium.en) + RAM CI cap (fail PR > 280 MB resident) + voice calibration corpus tooling | Implementer (Go) |

**Wave 1 ship gate:**
- Tahoe CI runner green on `make check`
- `whisper-distil-large-v3` resident < 280 MB on cold start
- Phase 4 voice round-trip latency parity ± 50 ms

### Wave 2 — Ambient awareness (calendar + multi-language + OCR ingest)

**Theme:** Three independent ambient-context features; file-disjoint, parallelize to the 6-cap.

| Task | Deliverable | Scope | Owner kind |
|---|---|---|---|
| W2-T1 | §3 Calendar | Implement `CalendarContext` interface; reasoner ambient-injection hook; cache + working-hours model; redact rules | Implementer (Go) |
| W2-T2 | §3 Calendar | Settings → Calendar pane (Swift); HUD focus panel "Up next" footer; notification widget conflict warn | Implementer (Swift) |
| W2-T3 | §4 Multi-language | Land 6 wake-word models; STT-hint plumbing in distil-small path; per-locale TTS routing | Implementer (Go) |
| W2-T4 | §4 Multi-language | Settings → Voice language dropdown + auto-detect; wizard step-4 language picker; system-strings table + editor | Implementer (Swift) |
| W2-T5 | §6 OCR ingest | `OCRIngester` Go interface + Haiku classifier wiring + memory-write pipeline + rate limiter | Implementer (Go) |

**Wave 2 ship gate:**
- Calendar conflict-detection unit + integration tests green
- ja-JP + es-ES voice round-trip parity ≤ 1.4× en-US
- `/capture` writes memory rows searchable by `leah ask` within 90 s

### Wave 3 — Distribution (marketplace)

**Theme:** Phase 4 SDK reaches operators. Single-owner per shared root; file-disjoint within marketplace package.

| Task | Deliverable | Scope | Owner kind |
|---|---|---|---|
| W3-T1 | §1 Marketplace | Index format + signed-JSON fetcher + cache + signature pinning | Implementer (Go) |
| W3-T2 | §1 Marketplace | `Installer` Go interface: fetch, verify, sandbox, register; auto-update on daemon start | Implementer (Go) |
| W3-T3 | §1 Marketplace | Capability-delta approval flow (IPC frames + sheet); revoke-list honoring | Implementer (Go + Swift) |
| W3-T4 | §1 Marketplace | Settings → Plugins → Browse tab; HUD update-available toast | Implementer (Swift) |
| W3-T5 | §1 Marketplace | Author signing-root pinning at build time; reference plugin (first-party `kagi-search` or similar) published to the marketplace as a smoke test | Implementer (Go) |

**Wave 3 ship gate:**
- Operator installs reference plugin from inside Leah in ≤ 3 clicks
- Auto-update applies a 1.0.1 → 1.0.2 bump without prompts when capabilities unchanged
- Capability-delta prompt blocks 1.0.2 → 2.0.0 bump that adds `screen-read`

### Wave 4 — Watching screen (live screen reasoning)

**Theme:** Continuous low-rate screen-stream + signal rules; depends on §6 OCR ingest landing.

| Task | Deliverable | Scope | Owner kind |
|---|---|---|---|
| W4-T1 | §7 Screen reasoning | `ScreenWatcher` Go interface + sampler (0.5 fps) + delta detection (perceptual hash) + energy guard | Implementer (Go) |
| W4-T2 | §7 Screen reasoning | Signal rules engine + 5 default rules + per-app mute logic | Implementer (Go) |
| W4-T3 | §7 Screen reasoning | Signal-fire → OCR + Haiku call → notification widget toast (operator accept/dismiss) | Implementer (Go + Swift) |
| W4-T4 | §7 Screen reasoning | Settings → Vision → Screen watch pane + rule editor + per-app mute UI; mandatory HUD glyph + menubar dot | Implementer (Swift) |
| W4-T5 | §7 Screen reasoning | Energy-budget meter + privacy-budget integration; double-opt-in modal for `OnAlways`; capture promote (§6 join) | Implementer (Swift) |

**Wave 4 ship gate:**
- Steady-state power < 3 W extra on M3 Pro idle screen
- HUD glyph visible whenever sampling is live (visual test)
- Default rules fire on canned screen recordings (error dialog, JIRA URL, stack trace)

### Wave 5 — Polish + ship (v1.2 release)

**Theme:** Cross-deliverable polish, migration smoothness, telemetry checks (none added, but verify), ship gate.

| Task | Deliverable | Scope | Owner kind |
|---|---|---|---|
| W5-T1 | Cross | Migration smoke: fresh install + v1.1 → v1.2 in-place upgrade + 2-Mac sync round-trip | Single-owner QA |
| W5-T2 | Cross | Performance regression suite: voice round-trip, OCR latency, marketplace search, calendar ambient injection — all surfaces vs Phase 4 baseline | Single-owner perf |
| W5-T3 | Cross | Privacy invariant audit: telemetry beacons (none expected), screen-watch hidden-active mode (none), marketplace install-count (CDN log only) | Single-owner security |
| W5-T4 | Cross | Documentation updates: `CHANGELOG.md` Phase 5 section, `ARCHITECTURE.md` deltas, runbook for marketplace publishing | Single-owner docs |
| W5-T5 | Release | Sparkle 2.7 appcast publish; macOS 16 (Tahoe) sign + notarize on Tahoe runner; tag `v1.2.0`; release notes | Single-owner release |

**Wave 5 ship gate:**
- All Phase 4 + Phase 5 invariants (§0.2 of both specs) pass automated check
- Operator runs v1.2 on both Macs for 7 days without intervention
- `make check` green on macOS 15 and macOS 16 CI runners

### Wave ordering rationale

- W1 ships compatibility + RAM headroom first because every later feature depends on Tahoe APIs (W4, W6) or RAM budget (W2, W7).
- W2 ships three file-disjoint ambient features in parallel — they share no code; each touches its own `internal/calendar/`, `internal/voice/lang/`, `internal/vision/ingest/` tree.
- W3 ships marketplace third because the reference plugin tested in W3-T5 wants the Wave-2 features (e.g. a "calendar-extra" plugin needs Wave-2's `CalendarContext`).
- W4 ships screen-watch fourth because it depends on §6 (W2-T5) for the capture join + §5 (W1-T3) for `SCStream`.
- W5 is the integration + release wave.

---

## 10. File structure decisions

### 10.1 Go packages

```
internal/
  marketplace/          # NEW — §1
    index.go
    installer.go
    auth.go             # author signing-root pinning
    cdn.go              # HTTPS + cache layer
    capability.go       # capability-delta + grant
    revoke.go
  voice/
    stt/                # NEW subpackage — §2
      distilled.go      # whisper-distil-large-v3
      medium_en.go      # whisper-distil-medium.en
      small_multi.go    # whisper-distil-small multi
      select.go         # selectSTT()
    lang/               # NEW subpackage — §4
      pack.go
      detect.go
      strings.go
    calibration.go      # §2 nightly WER + corpus
  calendar/             # NEW — §3 (sits alongside existing gcal adapter)
    context.go
    cache.go
    workinghours.go
    redact.go
  vision/
    ingest/             # NEW subpackage — §6
      ocringest.go
      classifier.go     # Haiku call
      redact.go         # PII redaction
      rate.go
    screenwatch/        # NEW subpackage — §7
      watcher.go
      sampler.go
      delta.go
      signal.go
      energy.go
  compat/               # NEW — §5
    version.go
    audio.go            # AVAudioEngine version-aware
    screen.go           # SCStream vs CGDisplayStream
    vision.go           # VNRecognizeTextRequest rev selector
    sparkle.go          # Sparkle 2.7 wrapper
```

### 10.2 Swift modules

```
LeahHUD/
  Marketplace/          # NEW — §1
    MarketplaceCoordinator.swift
    MarketplaceBrowsePane.swift
    PluginCardView.swift
    CapabilityDeltaSheet.swift
  Settings/
    CalendarPane.swift  # NEW — §3
    VoiceLanguagePane.swift # NEW — §4 (extends existing VoicePane)
    MemoryCapturePane.swift # NEW — §6 (extends existing MemoryPane)
    ScreenWatchPane.swift # NEW — §7
  Compat/               # NEW — §5
    OSVersion.swift
    CompatLayer.swift
    Tahoe/
      TahoeStatusItem.swift
      TahoeScreenCapture.swift
      TahoeAudioSession.swift
    Sequoia/
      SequoiaStatusItem.swift
      ...               # fallbacks
```

### 10.3 Migrations

`internal/db/migrations/` follows the existing dated-numbered scheme. Seven new migrations land in W1–W4 (one per deliverable that touches schema).

### 10.4 Test fixtures

```
testdata/
  marketplace/          # NEW
    index.signed.json
    sample-plugin-v1.0.0.leahplugin
    sample-plugin-v1.0.1.leahplugin
    sample-plugin-v2.0.0.leahplugin  # capability-delta case
  voice/
    calibration-en.wav  # NEW; reference calibration for WER baseline
    calibration-ja.wav  # NEW
    calibration-es.wav  # NEW
    ...
  vision/
    error-dialog-screen.png  # NEW; signal-rule fixture
    jira-screen.png
    slack-screen.png
    stack-trace-screen.png
  calendar/
    sample-week.ics     # NEW; multi-source aggregation fixture
```

### 10.5 What does NOT change

- `internal/ipc/frame.go` — frozen-enum per CLAUDE.md "frozen-enum files: single-owner per dispatch". Phase 5 IPC additions go through a single PR that owns this file.
- `internal/obs/events.go` — same.
- `internal/db/schema.go` — top-level schema constants; migrations are the change surface.
- Existing dispatch templates in `docs/engineer/dispatch-templates/` — Phase 5 reuses them; the templates absorb feedback per `memory/feedback_dispatch_template_reference.md`.

---

## 11. Migration plan (v1.1 → v1.2)

### 11.1 In-place upgrade path

1. Operator runs Sparkle auto-update (or downloads notarized .dmg manually).
2. New binary launches; daemon reads schema version from `leah.db` (currently v1.1 schema).
3. Migration runner applies the 7 new migrations in order (§8 table).
4. Daemon restarts subprocesses (HUD, voice, vision); operator sees a one-line "Updated to v1.2" toast.
5. Voice continues using Phase 4's full Whisper-large-v3 until the operator reboots OR the daemon hits memory pressure — at which point distilled becomes the default (per §2.4 selection). This avoids a mid-day STT model swap.
6. Calendar adapter (if previously authorized) starts ambient context injection immediately.
7. Marketplace index fetches in the background; Settings → Plugins → Browse becomes populated within 5 s.
8. Multi-language packs sit unused until operator changes locale in Settings.
9. OCR ingest + screen-watch stay OFF (default per invariants §0.2 (3) + §0.2 (9)).

### 11.2 Downgrade path

- Sparkle does not support downgrade by default; predecessor §0.1 keeps Sparkle. If operator needs to revert, they reinstall v1.1 manually; schema is backward-compatible for additive changes only (Phase 5's 22 new tables are unread by v1.1 — they become dead rows that future v1.2 reinstall will pick up).
- The v1.1 daemon does NOT delete Phase 5 tables; they remain at-rest until v1.2 is reinstalled.

### 11.3 Multi-device sync interplay

When upgrading two paired Macs:

1. Each Mac runs the migration independently.
2. CRDT sync sees new tables tagged per §8.3 sync-class.
3. `marketplace_plugin` rows synced via per-row LWW; operator's plugin set replicates within ≤ 60 s of both Macs being online.
4. `calendar_*`, `voice_lang_pref`, `voice_string` synced as last-writer-wins registers.
5. `screen_watch_rule` synced — operator's custom signal rules replicate.
6. `voice_calibration` does NOT sync — each Mac has different mic + acoustic environment.

If only one Mac upgrades to v1.2 first, the v1.1 Mac will see sync packets with unknown sync-class entries; per predecessor §2.7 (schema-version-mismatch failure mode), sync suspends with that peer until both upgrade. Settings surface a banner: "Sync paused — Mac-A is on v1.1; update both to the same version."

### 11.4 Telemetry / Anthropic key

No change. BYOK persists. No new keys required for Phase 5 features.

### 11.5 Plugin compatibility (Phase 4 SDK plugins)

Phase 4 plugins continue to load. Phase 5's marketplace install path treats them identically to v1.2 plugins (same capability gate, same signature check). The only delta: plugins built against Phase 4's IPC frame set work unchanged; plugins that opt into Phase 5 new frame kinds (`marketplace.*`, `vision.snap` extensions, `calendar.*`) require a `manifest.minLeahVersion: 1.2.0`.

---

## 12. What ships in v1.2 vs deferred to v2

### 12.1 Ships in v1.2 (Phase 5 cumulative)

| Capability | Section |
|---|---|
| Marketplace plugin discovery + install + auto-update | §1 |
| Whisper distilled (default 250 MB) + RAM fallback + calibration | §2 |
| Calendar-aware reasoning + ambient injection + conflict + free-blocks | §3 |
| 6-language voice (en, ja, es, fr, de, zh) + auto-detect | §4 |
| macOS 16 (Tahoe) full compatibility | §5 |
| Vision OCR → memory ingest with rate limit + per-app blocklist | §6 |
| Live screen reasoning (default OFF) + 5 signal rules + HUD glyph | §7 |

### 12.2 Deferred to v1.3 (Phase 6 next-phase)

| Capability | Why defer |
|---|---|
| Paid plugin distribution | Predecessor §0.1 + `memory/project_leah_ship_path.md` (BYOK-only) |
| Auto-scheduling (Motion-style calendar) | Needs 90 d conflict-detection telemetry |
| Continuous passive screen ingest (Limitless-style) | Privacy envelope discussion; ship on-demand first |
| Operator-fine-tuned voice model | Cost-effort > value lift; revisit with 6 mo corpus |
| Custom wake-phrase training UI | Model file replacement still requires reinstall |
| Plugin dependency resolution | Phase 5 plugins are flat; ship deps when load-bearing |
| Marketplace ratings / reviews | Telemetry conflict — opinions live in plugin READMEs |
| Multi-monitor screen-watch | Privacy budget rework |
| Predictive UI overlay (Leah-draws-on-screen) | Compositor complexity |
| Apple Intelligence on-device model integration | Quality lift unverified |
| macOS 16 Live Activities for menubar | Investigate; ship in v1.3 if useful |
| Locales beyond en/ja/es/fr/de/zh | Add 2-3 per release; defer pt-BR, ko-KR, hi-IN, ar-SA |
| Code-switching mid-utterance | distil-small handles loanwords; first-class detection later |
| Multi-Anthropic-key load balancing | No real cost or rate-limit incident yet |
| Sparkle-replacement delta updates | Sparkle works; delta saves bandwidth only |
| Voice persona library | Cosmetic; multi-language is load-bearing voice gap |

### 12.3 Deferred to v2 (post-Phase 6)

| Capability | Why v2 |
|---|---|
| iOS / iPadOS companion (read-only HUD via A2A) | Per predecessor §2.10 — mobile remains v2 horizon |
| Mobile push (APNs) | No mobile target |
| Cross-platform (Linux, Windows) OCR + voice + HUD | Operator runs macOS; v2 may revisit |
| 1M-token reasoning contexts | Cost + latency; revisit when Sonnet pricing supports |
| Multi-attendee scheduling | Requires multi-tenant trust model |
| Travel-time integration | Needs Maps API + location permission |
| Per-plugin Spaces (true OS-level isolation) | Phase 4 sandbox sufficient absent demonstrated escape |
| iMessage / WhatsApp / SMS operator interaction | Adapters per `memory/leah_first_launch_integration_auth.md` |

---

## 13. Performance envelope (Phase 5 cumulative)

### 13.1 RAM budget (steady state, all Phase 5 features active where default-ON)

| Subsystem | Phase 4 | Phase 5 | Delta |
|---|---|---|---|
| Daemon core | 80 MB | 80 MB | 0 |
| Whisper (default) | 850 MB | 250 MB | **−600 MB** |
| Reasoner cache | 60 MB | 60 MB | 0 |
| Memory + vector store | 95 MB | 95 MB | 0 |
| Sync coordinator | 25 MB | 25 MB | 0 |
| Recommender | 18 MB | 18 MB | 0 |
| Calendar context | n/a | 12 MB | +12 MB |
| Marketplace | n/a | 18 MB | +18 MB |
| OCR ingest (idle) | n/a | 4 MB | +4 MB |
| Screen watch (OFF default) | n/a | 0 MB (when off) | 0 |
| HUD | 60 MB | 60 MB | 0 |
| **Total steady (default features)** | **1188 MB** | **622 MB** | **−566 MB** |

The −566 MB delta comes entirely from §2 Whisper distillation; the other Phase 5 additions are net +34 MB.

### 13.2 Steady-state CPU

Phase 4 baseline: < 6% on M3 Pro idle. Phase 5 target: ≤ 7% (Calendar + Marketplace background sync + idle screen-watch when off contribute ≤ 1%).

### 13.3 Steady-state power

Phase 4 baseline: < 5 W extra over idle on M3 Pro. Phase 5 target: ≤ 8 W when *all* features active (screen-watch in `OnAlways` mode contributes < 3 W).

### 13.4 Latency budgets (operator-felt)

| Surface | Phase 4 | Phase 5 | Notes |
|---|---|---|---|
| Voice round-trip (p95) | ≤ 1.1 s | ≤ 1.1 s | Distilled STT slightly faster than full-fidelity; offset by per-locale TTS routing |
| `/look` screenshot reasoning | ≤ 1.4 s | ≤ 1.3 s | Tahoe `VNRecognizeTextRequest` rev 4 ~40% faster |
| `leah ask` (RAG over memory) | ≤ 400 ms | ≤ 420 ms | +20 ms from calendar-context join when ambient-injection ON |
| Marketplace search (cached) | n/a | ≤ 50 ms | New |
| Marketplace install (1 MB plugin) | n/a | ≤ 1.5 s | New |
| Screen-watch signal → toast | n/a | ≤ 700 ms | OCR + Haiku call |
| Calendar conflict check | n/a | ≤ 4 ms | New |
| OCR capture → searchable | n/a | ≤ 90 s | Includes embedding job batch latency |

---

## 14. Security model (Phase 5 cumulative)

### 14.1 Trust roots (Phase 5 additions)

1. **Marketplace index signing key** — pinned Ed25519 public key in app bundle; rotated via app update only.
2. **Plugin author signing keys** — registered in `marketplace_plugin.sha256` per row; pinned per-author at first install.
3. **Voice model SHA-256** — pinned per row in `voice_model.sha256`; rotated via app update.
4. **Language pack SHA-256** — pinned per row in `voice_lang_pack`; rotated via app update.

### 14.2 Privacy budget (Phase 5 additions)

The Phase 4 §8 budget runtime gains new feature categories:

| Category | Per-day default cap | Notes |
|---|---|---|
| marketplace-fetch | 24 (1 / hour) | Index pull |
| marketplace-install | Unbounded | Operator-initiated; transient |
| calendar-read | 1440 (1 / min) | Ambient injection |
| voice-cloud-fallback | 200 utterances | Existing Phase 4 category; unchanged |
| ocr-capture | 1000 captures | Operator-initiated; rate-limited per §0.2 (11) |
| screen-watch-signal-cloud | 30 signal-fires routing OCR to Haiku | Per §7.7 |

Operator overrides per Settings → Privacy → Budgets.

### 14.3 Threat model deltas

- **Compromised marketplace CDN.** Mitigated by signed index + per-plugin SHA pinning. Attacker who controls the CDN cannot serve a modified plugin without owning the author's key.
- **Compromised plugin author key.** Revoke list (§1.5 + `marketplace_revoke`) honors at next-start; operator notified.
- **Calendar event title PII.** Mitigated by privacy classification + redact rules + plugin capability gate (§3.6).
- **Always-on screen capture as covert surveillance.** Mitigated by mandatory HUD glyph (§7.10) + default-OFF + double-opt-in + per-app mute + energy guard.
- **Voice language detection used to identify operator.** Detection runs locally; detection results never leave the Mac.
- **OCR ingest writes credentials to memory.** Mitigated by classifier `redactSpans` + heuristic post-filter (§6.7).

---

## 15. Observability (Phase 5 additions)

Phase 5 extends predecessor §0.2 (4) privacy-budget runtime and `internal/obs/events.go` with new histograms + counters:

| Metric | Kind | Section |
|---|---|---|
| `marketplace_search_latency_ms` | histogram | §1 |
| `marketplace_install_total` | counter (per outcome: success / sig-fail / sandbox-fail / cap-deny) | §1 |
| `marketplace_autoupdate_blocked_total` | counter (per reason) | §1 |
| `voice_stt_latency_first_partial_ms` | histogram (per model) | §2 |
| `voice_wer_distilled_vs_full` | gauge (rolling 7 d) | §2 |
| `voice_lang_detection_misclass_total` | counter | §4 |
| `calendar_ambient_injection_latency_ms` | histogram | §3 |
| `calendar_conflict_warnings_total` | counter | §3 |
| `os_compat_path_taken` | counter (per OS version + per surface) | §5 |
| `ocr_capture_total` | counter (per outcome: written / classifier-no / rate-limited / blocklisted) | §6 |
| `ocr_redact_spans_total` | counter | §6 |
| `screenwatch_signal_fired_total` | counter (per rule) | §7 |
| `screenwatch_power_watts` | gauge | §7 |

All metrics local-only; surfaced in `leah debug obs` CLI and in About → Debug pane.

---

## 16. Documentation deliverables

Phase 5 PRs that land documentation:

- `CHANGELOG.md` — Phase 5 entries per merge.
- `ARCHITECTURE.md` — sections 11–16 added (one per deliverable).
- `docs/engineer/runbooks/marketplace-publish.md` — NEW; how plugin authors sign + publish.
- `docs/engineer/runbooks/voice-calibration.md` — NEW; calibration corpus protocol.
- `docs/engineer/runbooks/calendar-redact.md` — NEW; redact-rule authoring guide.
- `docs/engineer/runbooks/tahoe-compat.md` — NEW; how to add new macOS-version-gated paths.
- `docs/engineer/runbooks/repo-settings.md` — UPDATED with new CI runner (Tahoe).
- `docs/engineer/specs/2026-06-23-leah-phase5-design.md` — THIS FILE (canonical).

---

## 17. Open questions (operator decisions)

Items flagged for operator confirmation per `memory/feedback_audit_recommended_not_autonomous.md` — these do NOT auto-dispatch:

1. **Marketplace CDN host.** Default proposal: `https://marketplace.leah.app/index.v1.json` (operator owns the domain). Alternative: GitHub Releases of a `leah-marketplace` repo. Choose at W3-T1.
2. **Plugin author verification process.** Open: should `authorVerified` require any human review, or is "≥ 3 plugins published, no revokes in 90 d" the entire algorithm? Proposed: yes, fully algorithmic — no human review (matches "no AI signatures, operator owns the loop" position). Confirm at W3-T1.
3. **Whisper-distil-large-v3 final model file.** Open: ship the HuggingFace `distil-whisper/distil-large-v3` ONNX-converted, or train a Leah-specific variant on operator's corpus? Proposed: ship the upstream HF model for v1.2; revisit per-operator-fine-tune in v1.3 per §2.11. Confirm at W1-T4.
4. **Calendar redact-rule defaults.** Open: should the daemon ship default redact rules (e.g. "(personal)" for events with the word "therapy")? Proposed: no — defaults could over-redact for operators whose work explicitly involves therapy/medical/etc. Operator adds rules. Confirm at W2-T1.
5. **Screen-watch signal rule defaults.** §7.4 ships 5 defaults. Open: are these the right 5? Proposed defaults align with the 2026 inventory's "agent-shaped surfaces" (error dialogs, ticket trackers, code stack traces). Confirm at W4-T2.
6. **Language pack downloads.** Open: ship all 6 packs in the app bundle (+90 MB), or download on first locale-change (faster install)? Proposed: ship in bundle. Single-download experience matters more than initial install size on a Mac with TB of disk.
7. **Memory capture default rate.** §0.2 (11) proposes 200/hour. Open: confirm. Operator who clicks `/capture` on 4-pane tmux at 3 fps would hit this fast; 200/hour is one capture every 18 s, which is "purposeful" not "ambient".
8. **macOS 15 support window.** Open: when does Leah drop macOS 15? Proposed: v1.3 (Phase 6) drops it. v1.2 supports both 15 + 16.

---

## 18. Risk register

| Risk | Severity | Likelihood | Mitigation |
|---|---|---|---|
| Whisper-distil quality regression > 1.5× WER for operator | Medium | Medium | Calibration corpus + nightly check + auto-recommend full-fidelity (§2) |
| Marketplace CDN goes down | Low | Low | Cached index + offline mode (§1.8 row 1) |
| Plugin author key compromise | High | Low | Per-plugin revoke + author-key rotation via app update |
| Tahoe API delta breaks Leah at OS release | High | Medium | CI Tahoe runner mandatory merge gate (§5.4) |
| Calendar ambient injection leaks PII via reasoner output | High | Low | Privacy class respected + operator redact rules + plugins do not see calendar by default (§3.6) |
| Screen-watch flags as covert surveillance | High | Low | Mandatory HUD glyph + default-OFF + double-opt-in (§7.7) |
| OCR ingest writes secrets to memory | High | Medium | Classifier `redactSpans` + heuristic post-filter + per-app blocklist (§6.6) |
| Multi-language voice WER unacceptable for non-en | Medium | Medium | Calibration per locale + cloud fallback opt-in (§4.8) |
| Marketplace becomes spam vector | Medium | Medium | Author-verified flag + revoke list + filter UI (§1.7) |
| Sparkle 2.7 sign-notarize regression | Medium | Low | Verify in W5-T5 ship gate; v1.1 fallback ready |
| Energy budget overage on M1 (older hardware) | Medium | Medium | Throttle to 0.25 fps + surface warning + per-Mac calibration (§7.7) |
| Phase 5 PR storm delays Phase 4 stability window | Low | Medium | W1 cannot start until Phase 4 has run 14 days clean (§0 boundary) |

---

## 19. Historical anchor table

| Phase | Spec | Status |
|---|---|---|
| Phase 0 (foundation) | `docs/engineer/specs/2026-06-04-foundation.md` | Shipped |
| Phase 1 (macOS native UI v1) | `docs/superpowers/specs/2026-06-21-leah-macos-native-ui-design.md` | Shipped (v0.x) |
| Phase 2 (closed-loop core) | (predecessor §0 of `2026-06-22-leah-phase4-design.md` references) | Shipped (v0.9) |
| Phase 3 (v1.0 public launch — voice + polish + sign+notarize+Sparkle) | `docs/superpowers/plans/2026-06-22-leah-macos-native-phase{2,3,4}.md` | Shipped (v1.0) |
| Phase 4 (multi-modal + multi-agent) | `docs/superpowers/specs/2026-06-22-leah-phase4-design.md` | In flight (v1.1 target) |
| **Phase 5 (distribution + ambient awareness)** | **`docs/superpowers/specs/2026-06-23-leah-phase5-design.md`** | **This file (v1.2 target)** |
| Phase 6 (v1.3) | Not yet authored | Future |

---

## 20. Cross-references

- **CLAUDE.md** — `Decision priority`, `Dispatch parallelism`, `Identity / output`, `Comments discipline`, `TDD + review`, `Worktree discipline`, `Token economy`, `Repo settings` — all binding for every Phase 5 PR.
- **`memory/research_ai_assistants_big_tech_2026.md`** — competitive baseline used to argue (§7) screen-watch positioning vs Gemini Live; (§1.11) telemetry abstention vs every big-tech vendor; (§6) operator-owned memory vs Limitless absorption.
- **`memory/research_ai_assistants_startups_2026.md`** — used to argue (§1.13) BYOK + no paid distribution vs the hardware startups that died; (§6) operator-owned screen memory vs Limitless/Bee acquisition pattern.
- **`memory/research_ai_capability_domains_2026.md`** — used to source gaps: calendar smart-scheduling (§3), multi-language coverage (§4), continuous screen-context (§7), OCR-to-memory (§6).
- **`memory/project_leah_ship_path.md`** — used to argue (§0.1, §1.13) BYOK + no paid tier; (§4) multi-language as binding pre-launch (Phase 4's "voice English-only" deferred limitation is now a Phase 5 deliverable).
- **`memory/leah_first_launch_integration_auth.md`** — Phase 5 inherits the `leah connect <integration>` CLI surface; calendar (§3) and any plugin (§1) needing integration auth uses it.
- **`memory/feedback_orphan_scan_before_tag.md`** — Phase 5 release gate (W5-T5) runs orphan-scan before the v1.2 tag.
- **`memory/feedback_audit_recommended_not_autonomous.md`** — §17 open questions are operator-decision items, not autonomous-dispatch items.
- **`memory/feedback_dispatch_template_reference.md`** — Phase 5 dispatch prompts reference `docs/engineer/dispatch-templates/<role>.md` by path; no re-authoring of rules per role.

---

## Appendix A — Detailed sub-protocols

### A.1 Marketplace author publishing flow

A plugin author who wants to publish to the Leah marketplace runs:

```
$ leah-plugin-sdk publish ./my-plugin
```

The SDK:

1. Reads `my-plugin/manifest.toml` (Phase 4 SDK format).
2. Builds the plugin binary (Go cross-compiled to darwin/arm64 + darwin/amd64).
3. Computes SHA-256 of the binary blob.
4. Reads the author's Ed25519 signing key from `~/.leah-plugin-sdk/keys/<author>.key` (mode 0600).
5. Signs `{manifest, sha256}` with the author key.
6. Uploads the signed bundle to the author's chosen distribution host (GitHub Releases, S3, custom CDN). The marketplace index does NOT host binaries; it only points to author-controlled URLs.
7. Submits an index-update PR to the marketplace index repo (operator-default: `https://github.com/leah-app/marketplace-index`). The PR adds a single JSON entry per the §1.4 schema.
8. Marketplace index repo CI re-signs the merged index with the marketplace root key + republishes to the CDN.

Author's private key never leaves their machine. The marketplace index repo cannot tamper with a plugin binary because each entry pins the SHA-256.

### A.2 Marketplace index build pipeline

The marketplace index is rebuilt on every merge to the index repo:

1. CI walks all plugin entries.
2. For each entry, CI fetches the binary from `downloadURL`, verifies SHA-256 + signature.
3. CI re-runs the Phase 4 §6 attestation check against the binary (signing chain, sandbox manifest, capability bounds).
4. Entries that fail any check are marked `attestationOK: false` and filtered from default browse (per §0.2 invariant 13).
5. CI re-computes `installCount` from the CDN log (last 30 days).
6. CI re-computes `authorVerified` per author: ≥ 3 entries, ≥ 90 d since first publish, zero revoke entries.
7. CI signs the merged index with the marketplace root Ed25519 key.
8. CI publishes to the CDN endpoint.

Index rebuild runs nightly even absent merges to refresh `installCount` and `lastUpdated` freshness.

### A.3 Marketplace revoke flow

If a plugin must be revoked:

1. Author submits a PR to the marketplace index repo adding the plugin ID + `reason: 'author-revoke'` to the revoke section.
2. CI re-signs the index.
3. On next daemon start (or 24-h auto-refresh) the operator's Leah sees the revoke entry.
4. Daemon unloads the plugin if currently loaded; surfaces a notification: "Plugin X revoked by author: <reason>. Removed."
5. Operator may re-install if the revoke was opaque (rare).

Marketplace root maintainers (the operator running the index repo) may add `reason: 'attestation-fail'` revokes for plugins whose signature or attestation degrades.

### A.4 Whisper distillation benchmark protocol

The W5-T2 performance regression suite runs the following benchmark on every Phase 5 PR touching `internal/voice/stt/`:

```
$ make voice-bench
```

Which:

1. Loads each of the three STT providers (distil-large-v3, distil-medium.en, distil-small-multi).
2. Streams the operator's calibration corpus (5 min English + 5 min per non-en locale if available).
3. Records: cold-load latency, first-partial latency p50/p95, RAM resident peak, WER vs reference transcript.
4. Compares against baseline stored in `testdata/voice/baseline.json`.
5. Fails if any metric regresses > 10%.

The baseline is regenerated on Phase 4 merge to main (one-time) + on operator-requested rebaseline (Settings → Voice → "Rebaseline").

### A.5 Whisper-distil ONNX quantization details

The distillation pipeline (run offline, before app bundling):

1. Start from upstream `distil-whisper/distil-large-v3` FP16 ONNX.
2. Apply ONNX Runtime int8 dynamic quantization on linear layers; keep attention layers at FP16.
3. Sanity-check on a 1 000-utterance held-out set: WER must not regress > 0.5 abs vs FP16.
4. Apply graph-level optimizations: constant folding, common-subexpression elimination, layer fusion.
5. Output `.ort` format optimized for ONNX Runtime ANE backend (Apple Neural Engine).
6. Sign the model file with the model-signing key (separate from author key and marketplace root).
7. Bundle into the Leah app at `Resources/voice/models/`.

Build-time check: `ls -lh Resources/voice/models/whisper-distil-large-v3.ort` must show ≤ 220 MB. CI fails the build otherwise.

### A.6 Calendar free-block algorithm

Given:
- `window` (e.g. next 8 h)
- `minDuration` (e.g. 30 min)
- Operator's working hours `[wh_start, wh_end]` per weekday
- Aggregated events from all `enabled=1` calendar sources

Algorithm (in pseudocode):

```
freeBlocks = []
now = clock.Now()
end = now + window
busy = sorted(events.filter(e -> e.End > now && e.Start < end), by Start)
busy = mergeOverlapping(busy)   // (a, b) + (b+5, c) → (a, c) when gap < 5 min

cursor = max(now, wh_start_today)
for event in busy:
    if event.Start - cursor >= minDuration && cursor < wh_end_today:
        freeBlocks.append(TimeRange(cursor, min(event.Start, wh_end_today)))
    cursor = max(cursor, event.End)

if cursor < wh_end_today && wh_end_today - cursor >= minDuration:
    freeBlocks.append(TimeRange(cursor, wh_end_today))

// Continue across day boundaries until `end`
return clip(freeBlocks, now, end)
```

Edge cases handled:
- Operator's `wh_end` < `wh_start` (overnight worker) — invert the working-hours logic.
- Operator's tz is DST-shifting today — use `time.Date()` to compute, not raw arithmetic.
- Personal-calendar events excluded from busy set when `Personal` role is set.

### A.7 OCR ingest classifier prompt template

The Haiku call from §6.4 receives:

```
System: Classify OCR'd screen text for memory storage.
Output STRICT JSON matching the schema in the schema field.

Source app: <sourceAppHint>
Source URL (if browser): <sourceURL>
OCR text (up to 50 blocks, ordered top-left to bottom-right):
<block1>
<block2>
...

Schema:
{
  "memoryWorthy": boolean,
  "category": "conversation" | "code" | "reading" | "form" | "ui-chrome" | "other",
  "tags": [string],
  "summary": string,       // <= 100 chars
  "redactSpans": [[int, int]]  // [start, end] byte offsets into the joined OCR text
}

Guidance:
- "memoryWorthy" = true when the text contains durable signal:
  conversation content, code, written text, articles, decisions.
- "memoryWorthy" = false for UI chrome (menus, toolbars, form labels alone).
- "redactSpans" MUST cover: credit-card numbers (Luhn-valid), SSN-shaped patterns,
  Anthropic API keys (sk-ant-*), AWS keys (AKIA*), bearer tokens (Bearer eyJ*).
- "tags" should mirror source app: e.g. "slack" if source is Slack.
```

Latency: Haiku 4.5 at ~600 ms p95; total ingest budget per §6.8 row 3.

### A.8 OCR PII heuristic post-filter

After the classifier returns, the daemon runs an independent regex sweep over the OCR text:

```go
// internal/vision/ingest/redact.go
var piiPatterns = []*regexp.Regexp{
    regexp.MustCompile(`\b4[0-9]{12}(?:[0-9]{3})?\b`),       // Visa
    regexp.MustCompile(`\b5[1-5][0-9]{14}\b`),                // MasterCard
    regexp.MustCompile(`\b3[47][0-9]{13}\b`),                 // Amex
    regexp.MustCompile(`\b\d{3}-\d{2}-\d{4}\b`),              // SSN
    regexp.MustCompile(`sk-ant-[A-Za-z0-9-_]{40,}`),          // Anthropic key
    regexp.MustCompile(`sk-[A-Za-z0-9]{32,}`),                // OpenAI key
    regexp.MustCompile(`AKIA[0-9A-Z]{16}`),                   // AWS access key
    regexp.MustCompile(`Bearer\s+eyJ[A-Za-z0-9_-]{20,}`),     // JWT bearer
    regexp.MustCompile(`-----BEGIN[A-Z ]+PRIVATE KEY-----`),  // PEM private key
}
```

If any heuristic matches a span the classifier did NOT redact, the daemon:

1. Refuses the write.
2. Logs to OSLog *without* the matched content.
3. Surfaces a notification: "OCR capture aborted — possible credential detected."

Luhn checksum applied to credit-card matches to reduce false positive.

### A.9 Screen-watch delta detection

To avoid Haiku calls on identical-screen samples, the watcher uses:

1. **Perceptual hash** (dHash, 64-bit) computed on each downsampled frame at 8×8 grayscale.
2. **Delta threshold:** Hamming distance ≥ 6 between consecutive hashes counts as a delta.
3. **Stable-state detection:** if 5 consecutive frames have Hamming distance < 6 to the first of the 5, the screen is "stable" and signal rules fire (operator is actively viewing this content, not scrolling through it).
4. **Suppression dedupe:** when a signal fires, the rule's `SuppressFor` clock starts; the same rule cannot fire again until `SuppressFor` elapses or the screen changes by ≥ 12 Hamming distance (clearly a different surface).

This keeps Haiku call rate < 1 / min in typical operator usage even with all 5 default rules active.

### A.10 Screen-watch energy guard

Sampling at 0.5 fps + delta detection + occasional OCR + occasional Haiku call should land < 3 W extra on a M3 Pro. To verify in production:

```go
// internal/vision/screenwatch/energy.go
type EnergyGuard struct {
    samples    ring.Buffer[powerSample]   // 60-sample ring, 1 sample / sec
    avgWattsLimit float64                 // 3.0 by default
    pollInterval time.Duration            // 1 s
}

// On each tick:
//   1. Read /usr/bin/powermetrics --samplers cpu_power --sample-count 1
//      (or the Apple Silicon equivalent via IOKit IOPowerSources)
//   2. Subtract baseline (Leah daemon idle power, sampled at startup)
//   3. Push to ring
//   4. If ring.Avg() > avgWattsLimit for 10 consecutive samples,
//      throttle sampler.fps from 0.5 to 0.25 and emit a Throttled event.
```

If thottle does not restore budget within 60 s, screen-watch suspends + surfaces "Screen-watch paused — energy budget exceeded".

### A.11 Multi-language wake-word training protocol

For each new locale (post-v1.2 addition workflow):

1. Collect 500 positive samples ("Leah" spoken by native speakers, varied prosody, varied SNR).
2. Collect 2 000 negative samples (random speech in the locale, similar SNR).
3. Train a CoreML small CNN on a 16 kHz mel-spectrogram input, output binary (wake / no-wake).
4. Target: false-accept rate ≤ 1 / hour passive listening, false-reject rate ≤ 5%.
5. Hold-out test on 20% of the corpus.
6. Convert to `.mlmodel`, sign with the model-signing key, bundle into app.
7. Add row to `voice_lang_pack`.

The v1.2 ship includes pre-trained models for the 6 listed locales. Subsequent locales follow this protocol.

### A.12 Migration test plan (W5-T1)

Three smoke scenarios:

**Scenario 1 — Fresh install (no prior Leah):**
- Mount notarized v1.2 .dmg.
- Drag to /Applications.
- Launch → wizard runs (Phase 3 6-step).
- Wizard step 4 shows language picker (Phase 5 §4.10).
- Wizard exit → daemon starts, all Phase 5 migrations apply (empty tables created).
- Operator runs `leah connect gcal` → calendar adapter authorizes → ambient injection starts.
- Operator runs `/capture` on a screen → OCR ingest path runs.
- Operator runs marketplace search → catalog populates.

**Scenario 2 — In-place upgrade v1.1 → v1.2:**
- Leah v1.1 running; daemon healthy; memory + sync data live.
- Sparkle delivers v1.2.
- Daemon restarts; migrations apply (the 7 from §8).
- Voice keeps using Whisper-large-v3 for current session per §11.1 (5).
- Operator hits ⌥Space → focus panel opens; voice still works.
- Operator opens Settings → Plugins → Browse → catalog populates.
- Operator changes voice language ja-JP → daemon swaps STT model + wake model.
- Operator restarts mac → next launch loads Whisper-distil-large-v3 by default.

**Scenario 3 — Two-Mac sync upgrade:**
- Mac-A on v1.1 + Mac-B on v1.1, paired, sync healthy.
- Mac-A upgrades to v1.2 first.
- Mac-A's sync coordinator emits the schema-version-mismatch IPC; Mac-B's daemon shows the banner per §11.3.
- Mac-B upgrades to v1.2.
- Sync resumes; Phase 5 sync-class tables replicate (marketplace_plugin grants, calendar_*, voice_lang_pref, screen_watch_rule, voice_string).
- Operator installs a plugin on Mac-A → within 60 s, plugin appears on Mac-B (capability-grant rows already replicated).

All three scenarios must pass before W5-T5 release.

### A.13 Telemetry abstention audit

W5-T3 audits the Phase 5 codebase for any new network call destinations:

```
$ rg -n 'http\.Get|http\.Post|net\.Dial|http\.NewRequest' internal/marketplace/ internal/calendar/ internal/voice/ internal/vision/
```

Expected destinations:

| Package | Destination | Justification |
|---|---|---|
| `internal/marketplace/` | `marketplace.leah.app` or operator-configured | Index fetch (§1) |
| `internal/marketplace/` | Plugin-author-controlled host (per index entry `downloadURL`) | Binary fetch |
| `internal/calendar/` | `googleapis.com` | gcal API (pre-existing) |
| `internal/calendar/` | `graph.microsoft.com` | ms365 API (when adapter wired) |
| `internal/voice/` | `api.elevenlabs.io` | TTS cloud (pre-existing) |
| `internal/voice/` | `api.openai.com` | Whisper API fallback (pre-existing, opt-in) |
| `internal/voice/` | `api.anthropic.com` | Haiku for §6 classifier, Sonnet for §7 cloud route |

Any other destination found in the audit must be either: removed (if it's a telemetry beacon) or justified in a code comment + this table.

### A.14 Dispatch template additions for Phase 5

Per `memory/feedback_dispatch_template_reference.md`, dispatch prompts reference templates by path. Phase 5 introduces no new role templates; existing implementer / reviewer / spec-author templates handle all Phase 5 work. The W3-T1 prompt should call out:

- File-disjoint constraint: each W2 task touches its own subtree; do not edit `internal/voice/stt.go` (Phase 4 surface) — extend via subpackage.
- Frozen-enum constraint: `internal/ipc/frame.go` additions go through W2-T2 single-owner PR; do not touch from other W2 tasks.
- Linter constraint: `golangci-lint run` clean per the existing template's "ship gate".

### A.15 Phase 5 PR-body template (operator-facing)

Per CLAUDE.md "drop ceremony" + `memory/feedback_pr_summary_style.md`, PR bodies should NOT read AI-generated. Phase 5 PR-body skeleton:

```
[3-6 line prose paragraph: what changed, why, what got smaller]

Test plan:
[3-5 bullets of concrete verification — file paths, test names, hex values]

Closes #<issue>
```

No `## Summary` headers, no emoji, no dash bullets in the prose paragraph.

---

## Appendix B — Out-of-scope candidate rationale (selected)

This appendix expands the §0.1 deferral rationale for the higher-friction candidates the operator's prompt listed but the spec did not adopt.

### B.1 Mobile companion (iOS/iPadOS read-only HUD via A2A)

The 2026 capability research (`memory/research_ai_capability_domains_2026.md`) shows the CarPlay AI category exists and the iMessage/WhatsApp-native pattern (Miora) has product-market fit. A mobile read-only HUD via A2A would be technically reachable in Phase 5 — the Phase 4 A2A peering substrate (predecessor §5) supports remote-peer rendering.

Deferred reasons:

1. **iOS app store policy.** Apple's BYOK-key store-policy posture for assistant apps is in flux; v1.2 ship date is uncertain if it depends on App Review.
2. **Read-only HUD value < write-capable companion.** A read-only mobile HUD without voice-back-to-Mac is "checking-on" the Mac, not "using" Leah. The voice-loop on iOS requires APNs + background-audio capability which is more substantial work.
3. **Operator's primary device is the Mac.** `memory/project_leah_ship_path.md` makes the operator the user; the operator does not have a stated mobile need that Mac doesn't already serve.

Revisit in v2 (post-Phase 6).

### B.2 Operator-facing onboarding polish

Phase 3 wizard is shipped (predecessor §0). Phase 4 added voice-onboarding copy. Further wizard polish is feasible as a sub-1-day patch series. It does not belong in a phase-spec because:

- No new surface introduced — wizard chrome stays as Phase 3 specified.
- No new interface — only string + asset changes.
- Operator-as-user does not report wizard friction in `memory/project_leah_ship_path.md`.

Out-of-band patch series sufficient if the operator surfaces specific friction.

### B.3 Paid plugin distribution

See §1.13 in-spec rationale. Three reinforcing arguments:

1. Cost-model invariant break.
2. Ship-path discipline (personal-use).
3. Adoption-signal sequence (free first, paid later if at all).

### B.4 Multi-Anthropic-key load balancing

The operator runs one Anthropic key. Rate limits on a per-key Anthropic plan are above Leah's per-day usage by ≥ 10×. The only scenario load-balancing helps: operator has multiple Anthropic accounts (work + personal). That's an account-management UX question, not an engineering one — Settings → API Keys can surface multiple keys with a selector if the operator wants. No need for daemon-side load balancing.

### B.5 Per-plugin Spaces (true OS-level isolation)

The Phase 4 sandbox is App Sandbox + capability-gated + privacy-budgeted. macOS Spaces-level isolation would mean each plugin runs in its own NSWindow + assigned Space. This:

- Adds compositor complexity (window management across plugin processes).
- Doesn't materially raise the trust boundary — the sandbox + signature gate already prevent code execution outside the plugin's intent.
- Confuses the operator (windows appearing on different Spaces).

Out unless a Phase 4 sandbox escape is demonstrated.

### B.6 Sparkle-replacement self-update

Sparkle delivers monolithic .dmg auto-update. The deltas the operator asked about (incremental update; downgrade-on-corrupt) are bandwidth optimizations:

- A typical v1.2 → v1.2.1 patch is ~40 MB. Delta would be ~5 MB. Operator bandwidth not the bottleneck on a Mac.
- Downgrade-on-corrupt: Sparkle's signature check already refuses to install a corrupt update; the daemon retains the prior version's bundle until success. Manual reinstall is the existing fallback.

Out.

### B.7 Voice persona library

Phase 4 ships ElevenLabs Flash v2.5 + Apple Ava Premium. Adding more personas:

- No operator-stated need; multi-language coverage (§4) is the real voice gap.
- Adds choice-paralysis surface in Settings.
- ElevenLabs catalog is accessible via API if the operator really wants a different voice; expose as a Settings → Voice → "ElevenLabs voice ID" override (one-line patch).

Out unless an operator-stated need surfaces post-v1.2.

---

## Appendix C — Phase 5 PR shape estimate

Per `CLAUDE.md` Token economy + dispatch parallelism:

| Wave | Tasks | Avg LoC per PR | Total LoC est | Parallelism |
|---|---|---|---|---|
| W1 (foundations) | 5 | 600 (compat scaffolding heavier) | ~3 000 | Mixed: T1 single-owner CI, T2/T3 parallel, T4/T5 parallel |
| W2 (ambient awareness) | 5 | 800 (3 new packages, full data flow each) | ~4 000 | All 5 file-disjoint, full parallel |
| W3 (marketplace) | 5 | 700 | ~3 500 | T1/T2 sequential (shared package); T3/T4/T5 parallel |
| W4 (screen reasoning) | 5 | 750 | ~3 750 | T1/T2 sequential, T3/T4/T5 parallel |
| W5 (polish + ship) | 5 | 200 (mostly tests + docs + release) | ~1 000 | Single-owner sequential |

**Total Phase 5 LoC estimate:** ~15 250 source-of-truth changes (excludes generated, tests proportional ~5 000 additional).

**Deletion targets (per "deletion default" in CLAUDE.md):**
- §2 Whisper distillation: ~150 LoC removed from old fallback in Phase 4 voice subsystem.
- §5 OS compat: ~200 LoC removed when CGDisplayStream paths fall to availability-guarded SCStream.
- §11.5 plugin compat: no removals — Phase 4 IPC frames keep working.

Each Phase 5 PR must include a "what got smaller?" line in the body. PRs that are pure-add must explicitly state why deletion was not possible.

---

## Appendix D — Open-source dependencies (Phase 5 additions)

| Dependency | License | Purpose | Source-of-truth tracking |
|---|---|---|---|
| `distil-whisper/distil-large-v3` (HF model file) | MIT | §2 STT default | `go.mod` does not track; SHA pinned in `voice_model.sha256` |
| `distil-whisper/distil-small` | MIT | §2/§4 multi-language STT | Same |
| Apple `Vision.framework` revision 4 | Apple OS-bundled | §6 OCR + §7 OCR | OS version gate; no dep |
| Apple `SCStream` | Apple OS-bundled | §7 screen capture | OS version gate; no dep |
| `onnxruntime-go` bindings | Apache 2.0 | §2 ONNX runtime wrapper | `go.mod` add in W1-T4 |
| Sparkle 2.7 | MIT | §5 Tahoe-compatible auto-update | Swift Package; bump in W1 |

No new Anthropic SDK version required; reuse Phase 4 SDK pinning.

---

## Appendix E — Operator playbook (post-v1.2 first-day)

A walkthrough for the operator picking up v1.2 for the first time. Composed of the highest-leverage flows the spec enables.

### E.1 First-launch refresh (in-place upgrade from v1.1)

Day 0 morning:

1. Sparkle auto-update delivers v1.2; operator clicks "Install and Relaunch."
2. Daemon restarts; migrations apply; "Updated to v1.2 — Phase 5 ships marketplace + 6 languages + calendar + screen capture + Tahoe + screen reasoning." toast appears in HUD.
3. Operator opens Settings → Plugins → Browse. Catalog populates within 5 s.
4. Operator searches "kagi" → installs `com.example.kagi-search` (first-party reference plugin).
5. Operator opens Settings → Voice. Notices new "Calibrate voice" button. Postpones; voice still works in en-US.
6. Operator opens Settings → Calendar. gcal auto-listed (Phase 3 wiring). Operator sets working hours Mon–Fri 09:00–18:00 PT.
7. Within 30 s, focus panel shows "Up next: 10:00 — standup, 12m."
8. Operator hits ⌥Space, asks "what's after standup?" — daemon reads from cache, < 100 ms answer.

### E.2 Multi-language flip

Day 0 afternoon (operator is bilingual JP+EN):

1. Settings → Voice → Voice language: ja-JP.
2. Daemon swaps wake model + STT path on next session.
3. Operator says "おはよう、レイア" — wake fires; transcription correct.
4. Reasoner replies in ja-JP system strings; TTS via Apple Kyoko Premium.
5. Operator enables "Detect language per session." Says "Hi Leah, what's my schedule?" — auto-detector flips to en-US; session swaps mid-utterance OK.

### E.3 Screen capture for memory

Day 1:

1. Operator reads a long article in Safari. Wants to remember the key claim.
2. Highlights the claim → ⌥⇧Space → drags rect over highlighted text → types `/capture article-name`.
3. Daemon OCRs the selection, classifier returns `category=reading, tags=[safari, "host.com", article-name]`, summary captured.
4. Memory row written within 90 s; embedding job runs.
5. Two weeks later, operator asks "what did the article on host.com say about X?" — `leah ask` finds the captured row.

### E.4 Calendar conflict warning

Day 2:

1. Operator types `/schedule 3pm Wed coffee`.
2. Daemon parses → checks `CalendarContext.Conflicts(Wed 15:00, Wed 15:30)`.
3. Returns: "Conflict with '1:1 with K' at 15:00 Wed. Next free 30 min: 15:30, 16:30, 17:00."
4. Operator chooses 15:30; daemon constructs an iCal `BEGIN:VEVENT` block + copies to clipboard for paste-into-calendar (auto-book is deferred §3.10).

### E.5 Screen-watch trial

Day 5 (operator decides to try screen-watch):

1. Settings → Vision → Screen watch → mode: On-foreground.
2. Operator works in VSCode. After 10 min, hits a stack trace.
3. Within 700 ms, toast: "Decode this stack trace?"
4. Operator clicks; signal-firing region (the stack trace) sent to Sonnet; explanation streams back.
5. Operator dismisses the toast 3 times across the day for `unread-slack-thread` — daemon learns; that rule deprioritizes.

### E.6 Marketplace plugin update with capability creep

Day 14:

1. Daemon checks marketplace index on start.
2. `com.example.kagi-search` has new version 1.3.0 that adds `screen-read` capability.
3. Auto-update does NOT apply (capability delta). HUD toast: "Kagi Search 1.3.0 wants screen-read access. Approve?"
4. Operator clicks → capability approval sheet shows the delta (network-out:kagi.com vs. + screen-read).
5. Operator denies. Plugin stays at 1.2.x; functional.
6. Author publishes 1.3.1 without screen-read; auto-update applies silently next morning.

### E.7 Two-Mac sync after v1.2

Day 21 (operator buys a new MacBook Pro):

1. New Mac: download v1.2 .dmg, drag to /Applications, launch.
2. Wizard runs; step 4 detects ja-JP locale + offers ja-JP voice.
3. Wizard exit; operator runs Settings → Sync → Pair → OTP shown on existing Mac.
4. Pairing completes in 1.2 s.
5. Marketplace plugin list replicates within 60 s; both Macs now have `com.example.kagi-search`.
6. Calendar working hours + redact rules + voice language pref replicate.
7. Operator's voice calibration does NOT replicate — operator runs Settings → Voice → Calibrate on the new Mac (different mic).

---

## Appendix F — Capability requirement glossary

Each plugin in the marketplace declares its `capabilities` array. Phase 5 expands the Phase 4 list:

| Capability | Description | Privacy budget impact |
|---|---|---|
| `network-out:<host>` | Plugin may make HTTPS requests to `<host>` (TLS, no other protocols) | None directly; logs counted |
| `mic` | Plugin may receive audio frames from the voice subsystem | Audio-out budget |
| `screen-read` | Plugin may receive screen frames from the vision subsystem | Vision budget |
| `calendar-read` | Plugin may read calendar events (redacted form) | Calendar-read budget |
| `calendar-write` | Plugin may create/modify calendar events | High; surface a confirmation prompt per write in v1.2 |
| `memory-read` | Plugin may query the memory store | Internal — no budget |
| `memory-write` | Plugin may add memory rows | Internal — rate-limited per §0.2 (11) |
| `mail-read` | Plugin may read inbox via gmail adapter | Mail-read budget |
| `mail-send` | Plugin may send mail | High; per-send confirm in v1.2 |
| `keychain-read:<service>` | Plugin may read a specific Keychain item (e.g. API key for its own service) | None directly; signature gates it |
| `file-read:<path>` | Plugin may read files matching the path glob | Filesystem budget |
| `file-write:<path>` | Plugin may write files matching the path glob | Filesystem budget; high friction prompt |
| `notification-post` | Plugin may surface notification widget toasts | Rate-limited (3 toasts/hour) |

Plugins must declare every capability they use; loading code paths that touch un-declared capabilities trips the Phase 4 sandbox + kills the plugin process.

---

## Appendix G — Cost projection

Phase 5 cumulative API costs per active operator (assumes default-ON features in normal day):

| API | Volume per day | Unit cost | Daily cost | Monthly cost |
|---|---|---|---|---|
| Anthropic Sonnet (reasoner) | ~30 prompts × 1k input + 500 output tokens | $3 / 1M in + $15 / 1M out | ~$0.32 | ~$9.50 |
| Anthropic Haiku (OCR classifier, screen-watch signals) | ~50 calls × 500 in + 200 out tokens | $0.80 / 1M in + $4 / 1M out | ~$0.08 | ~$2.50 |
| Anthropic Opus (rare, complex) | ~3 prompts × 2k in + 1k out | $15 / 1M in + $75 / 1M out | ~$0.16 | ~$5.00 |
| Voyage embeddings (memory + capture) | ~200 chunks | $0.12 / 1M tokens | ~$0.01 | ~$0.30 |
| ElevenLabs Flash v2.5 (TTS cloud) | ~30 sentences × 50 chars | $0.30 / 1k chars | ~$0.45 | ~$13.50 |
| OpenAI Whisper API (fallback) | rare, opt-in | $0.006 / min | ~$0.00 (default off) | ~$0.00 |
| Apple OS APIs (Vision, Speech, etc.) | unlimited | $0 | $0 | $0 |
| **Total (operator-felt)** | | | **~$1.02 / day** | **~$30.80 / month** |

The marketplace + sync + voice distillation add zero recurring API cost; they save cost by replacing cloud round-trips with local computation.

Per `memory/project_leah_ship_path.md` BYOK invariant: this is the operator's cost on their own Anthropic / ElevenLabs / Voyage accounts.

---

## Appendix H — Phase 5 PR fingerprint

A Phase 5 PR is recognizable by:

- Branch name: `phase5-w<N>-t<M>-<short-desc>` (e.g. `phase5-w2-t1-calendar-context`).
- Linear ticket: MAY-100-series (when migrated; current Phase 4 ends in MAY-90s).
- PR title: `<wave>(<deliverable-package>): <action> — <surface>` (e.g. `w2(calendar): land CalendarContext ambient injection`).
- PR body prose: 3-6 lines explaining what, why, what got smaller.
- PR labels: `phase-5`, `wave-N`, `deliverable-N` (per `.github/labels.yml`).

The PR-gates check (predecessor §15 invariant) validates the label set + branch-name regex on every PR open.

---

## Appendix I — Phase 5 dispatch examples (templates)

### I.1 Implementer dispatch — W2-T1 (calendar)

```
Role: implementer (Go)
Template: docs/engineer/dispatch-templates/implementer-go.md

Task: W2-T1 — Implement CalendarContext interface + cache + working-hours model + redact rules.

Spec: docs/superpowers/specs/2026-06-23-leah-phase5-design.md §3.3, §3.4, §3.5, §3.11, §3.12, §3.13, §3.14, §3.15, §3.16, §3.17.

Files to create:
- internal/calendar/context.go
- internal/calendar/cache.go
- internal/calendar/workinghours.go
- internal/calendar/redact.go
- internal/db/migrations/2026-06-23-003-calendar.sql

Files to MODIFY (single-owner this wave):
- internal/reasoner/middleware.go — add CalendarContext middleware step.

DO NOT touch:
- internal/ipc/frame.go (frozen-enum)
- internal/obs/events.go (frozen-enum)
- internal/voice/* (W2-T3/T4 owns)
- internal/vision/* (W2-T5 owns)

Tests required:
- internal/calendar/*_test.go covering each interface method
- Integration test: ambient prompt prefix derived from a 3-event mock cache
- Conflict detection with edge cases (overlap by 1 min, back-to-back)

Ship gate:
- golangci-lint clean
- make check green on both macOS 15 + macOS 16 CI runners
- Reviewer subagent verdict APPROVE
```

### I.2 Reviewer dispatch — W3-T2 (marketplace installer)

```
Role: reviewer (adversarial)
Template: docs/engineer/dispatch-templates/reviewer.md

PR: #<num> — w3(marketplace): Installer with verify+sandbox+register

Spec: docs/superpowers/specs/2026-06-23-leah-phase5-design.md §1.3, §1.6, §1.7, §1.8.

Dimensions to evaluate (ALL must clear for APPROVE):
1. Correctness — does Install() block on attestation? abort on sig mismatch?
2. Side effects — does an aborted install leave partial state in marketplace_plugin?
3. Conciseness — any duplicate verification logic vs Phase 4 §6 attestation?
4. Refactor / simplification — is the Capability gate code duplicated from Phase 4?
5. Doc updates — CHANGELOG.md + ARCHITECTURE.md sync?
6. Comment trimming — godocs adhere to comments-discipline?
7. Test coverage — sig-fail, sandbox-fail, cap-deny, happy-path?
8. Deletion default — does any Phase 4 code get smaller?
9. No AI signatures — body + commits + comments?
10. No ceremony — drop ## Summary headers if any?

Adversarial focus: try to land an attack via:
- Crafted marketplace index with bad signature
- Plugin manifest claiming SHA-256 of one binary, serving a different one
- Capability creep on update
- Race condition: two installs of the same plugin
```

---

## Appendix J — Phase 5 sequencing diagram

```
W1 ───────────────────► W2 ─────────────────► W3 ─────────────────► W4 ─────────────────► W5
(2 wk)                  (3 wk)                (2 wk)                (3 wk)                (2 wk)

W1: foundations         W2: ambient            W3: marketplace      W4: screen watch     W5: ship
- Tahoe CI runner       - calendar             - index + installer  - sampler+detect     - migration smoke
- Tahoe API adopt       - multi-lang voice     - cap-delta approve  - signal rules       - perf regression
- Distil-large-v3       - OCR ingest           - marketplace UI     - watch UI + glyph   - privacy audit
- Distil-medium fallbk  - 5 PRs parallel       - reference plugin   - capture promote    - docs
                                                                                          - release v1.2

Sequential within wave where shared roots.
Parallel within wave where file-disjoint.
W1→W2 hard gate. W2→W3 hard gate. W3+W4 partial-parallel (different packages).
W5 starts when W1+W2+W3+W4 all on main.
```

Estimated total wall-clock: 12 weeks. Estimated parallel-shipped PRs: ~23 distinct merges.

---

## Appendix K — Cross-spec invariant compatibility check

For every Phase 5 PR, the spec-parity script (`scripts/check-spec-parity.sh`) checks:

1. No mention of "Co-Authored-By" in any commit or PR body.
2. No `## Summary` or `## Test plan` headers in PR bodies.
3. No emoji in PR body (per `memory/feedback_pr_summary_style.md`).
4. Branch name matches `^phase5-w[1-5]-t[1-5]-[a-z0-9-]+$`.
5. PR labels include `phase-5` + `wave-N`.
6. Spec deliverable referenced in commit message OR PR body.
7. "What got smaller?" line present in PR body (or explicit `deletion-default: pure-add justified` marker).

The script runs in `make check` and as a GitHub action; failing any check fails the PR.

---

## Appendix L — Phase 5 close audit checklist

When v1.2 is tagged, the audit-session skill runs:

- [ ] All 7 deliverables shipped per §12.1.
- [ ] All 22 new tables created and indexed.
- [ ] Sync class declared for every Phase 5 table.
- [ ] Privacy budget categories added for marketplace, calendar, OCR, screen-watch.
- [ ] No new telemetry beacons (§A.13 audit clean).
- [ ] Tahoe CI runner green for 14 consecutive days pre-tag.
- [ ] Two-Mac sync smoke (§A.12 scenario 3) passes.
- [ ] Reference plugin (`com.leah.kagi-search` or similar) live in marketplace index.
- [ ] Calibration corpus tooling functional in 2+ locales.
- [ ] Screen-watch HUD glyph visible in all sampling-active states (visual regression test).
- [ ] Orphan-scan clean per `memory/feedback_orphan_scan_before_tag.md` (every new package wired into composition root).
- [ ] Open questions §17 resolved or explicitly deferred to v1.3 plan.

---

## Appendix M — Phase 5 deferred candidate complete list

The operator's dispatch listed 15 candidate deliverables; Phase 5 adopts 7. The remaining 8, with explicit deferral rationale:

| # | Candidate | Deferred to | Rationale |
|---|---|---|---|
| 8 | Paid plugin distribution (Stripe + revenue share) | v1.3+ or never | §1.13: cost model invariant + ship-path discipline + adoption-signal sequence |
| 9 | Operator-facing onboarding polish | Out-of-band patches | §0.1: Phase 3 wizard sufficient; no operator-stated friction |
| 10 | Voice persona library | v1.3+ if operator request | §0.1: cosmetic; multi-language is the load-bearing voice gap |
| 11 | Mobile companion (iOS/iPadOS read-only HUD via A2A) | v2 | §B.1: store policy + read-only-HUD < write-companion + operator's primary device is Mac |
| 12 | Multi-Anthropic-key support | v1.3+ if operator request | §B.4: account-management UX, not load-balancing engineering |
| 13 | Per-plugin Spaces (true OS-level isolation) | Never unless sandbox escape demonstrated | §B.5: Phase 4 sandbox sufficient; compositor complexity penalty |
| 14 | Mobile push (APNs) | v2 | No mobile target this phase |
| 15 | Self-update beyond Sparkle (delta + downgrade-on-corrupt) | v1.3+ or never | §B.6: bandwidth optimization, not blocker; Sparkle works |

Each deferral is reversible if operator priority shifts. The Phase 5 spec's argument is sequencing: ship the load-bearing 7 first, learn from operator usage, revisit the 8 deferred.

---

## Appendix N — Glossary

- **Author key** — Ed25519 key held by a plugin author; signs their plugin bundle.
- **Calibration corpus** — operator-recorded 5-min reference audio used for WER baseline.
- **Capability delta** — set difference between a new version's capabilities and the installed version's.
- **CRDT delta** — incremental change batch the multi-device sync replicates between peers.
- **Distil-whisper** — family of distilled student models of OpenAI Whisper.
- **HUD glyph** — visible indicator (eye icon) that the screen-watch sampler is active.
- **Marketplace root key** — Ed25519 key the marketplace index repo CI uses to sign the merged index.
- **Plugin** — Phase 4 SDK artifact: signed Go binary + capability manifest + sandbox spec.
- **Privacy budget** — Phase 4 §8 per-feature daily cap on sensitive operations.
- **Redact span** — byte-offset range in OCR text that must be blanked before storage.
- **Signal rule** — operator-editable pattern that fires a contextual suggestion when the screen-watch sees a match.
- **Sync class** — table-level tag indicating whether rows replicate to paired peers.
- **WER** — Word Error Rate; STT accuracy metric.

---

---

## Appendix O — Performance regression baseline anchors

The W5-T2 perf regression suite anchors against Phase 4 measurements. Phase 5 baseline values (captured from predecessor's perf budget tables):

| Metric | Phase 4 baseline | Phase 5 target | Tolerance |
|---|---|---|---|
| Voice round-trip p95 | 1.1 s | 1.1 s | +50 ms / −50 ms |
| `/look` (screenshot reasoning) | 1.4 s | 1.3 s | +100 ms / −400 ms |
| `leah ask` (RAG p95) | 400 ms | 420 ms | +50 ms / −unbounded |
| Daemon RAM steady | 1.19 GB | 622 MB | +100 MB hard cap |
| Daemon CPU steady | 6% | 7% | +1% hard cap |
| Cold start | 1.4 s | 1.2 s (Tahoe) / 1.4 s (Sequoia) | +200 ms |
| Memory store query p95 | 80 ms | 80 ms | +20 ms |
| Voice cold first-partial | 350 ms | 400 ms (distilled) | +100 ms |
| Sync delta apply (1 row) | 8 ms | 8 ms | +2 ms |

PR-level perf regression check enforces the tolerance row by row. Any regression exceeding tolerance blocks merge until either fixed or the tolerance is operator-approved expanded.

---

## Appendix P — Spec authoring discipline

This spec follows the same authoring rules the predecessor inherits from `CLAUDE.md` + `memory/feedback_pr_summary_style.md`:

- No AI signatures in normative body.
- No emoji.
- No `## Summary` headers.
- Concrete: file paths, line numbers (when stable), measurable done states.
- Past-tense not used (this is forward-looking); present-tense for invariants, future-tense for ship gates.
- Tables for inventories; prose for arguments.

Phase 5 PRs that modify this spec must follow the same rules.

---

End of `docs/superpowers/specs/2026-06-23-leah-phase5-design.md`.
