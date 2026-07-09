# Changelog

## v1.1 (Phase 4 — 2026-06-23)

- Voice frontier runtime: Whisper-quality STT chain, BPE detokenizer, full-duplex coordinator, turn instrumentation. Answer-engine streams over voice without HUD focus.
- Multi-device sync: Bonjour discovery + OTP pairing + mTLS transport (`sync.NewMTLSConfig` shared-key derive), CRDT peer coordinator, attempt-counter backoff. Settings → iCloud sync pane.
- Recommend pass-2: signal dispatcher with voice announcer, SQLite engine, macOS mirror source, anti-list A/B pane.
- Camera + vision: OCR engine with consent store, Swift HUD router.
- Multi-agent A2A: inbound MCP token-scoped server + Connections pane. Leah-to-Leah client/server over ed25519 with consent + budget gating.
- Continuous attestation verifier + scoped question pool wired against MCP inbound.
- Plugin SDK: host + sandbox + quota meter. Weather plugin as reference.
- Privacy budget runtime (`budget.NewRuntime`) — shared ledger debited by A2A, recommend, vision.
- Watchdog supervisor: heartbeat + circuit breaker + leak detector.
- Deleted three superseded sketches (`docs/engineer/specs/2026-06-10-{voice-frontier,learn-recommend-apply,mcp-a2a-publish}.md`) subsumed by `docs/superpowers/designs/2026-06-22-leah-phase4-design.md`.

## v3.3.0 (2026-06-23)

- TTS subsystem: ElevenLabs Flash v2.5 cloud primary + Apple Ava Premium local fallback, `tts.cloud.frame` + `tts.local` IPC fan-out, daemon-side privacy classifier.
- Wake-word adapter: `wake-leah.mlmodel` bundled under `Resources/Models/`, VAD-gate + per-app suppression list ON by default.
- Push-to-talk: Fn (internal), right-⌘ (external).
- Minimal-mode runtime toggle (Settings → Appearance) — strips grain, italic, gold-accents.
- Touch ID gate for memory purge + telemetry toggle per §17.13.
- Push-source IPC fan-out complete — knowledge + memory + integrations push deltas to HUD.
- KG-backed citations on the answer-engine streaming path.
- MCP publish ships read-only (queries only, no mutations).
- Sparkle auto-appcast generator: EdDSA verify + rollback channel. Appcast on GitHub Pages; signing-key custody per §17.19.
- §4.7 dashboard surface: Memory + agenda + briefs + news + knowledge views over existing widget adapters.
- §17.12 marketing-hero asset slots finalized — 4 hero PNGs + SVG/PDF mark.
