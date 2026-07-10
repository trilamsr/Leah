# Changelog

## v1.1 (Phase 4 — 2026-06-23)

- Voice on macOS: wake with "Hey Leah", hear replies without opening the HUD.
- Multi-device sync: pair Macs with a one-time code — memory and settings stay in sync, encrypted end to end.
- Smarter recommendations: learns from your patterns; an anti-list pane in Settings lets you say "stop suggesting this".
- Camera + vision: point a camera at a whiteboard or receipt; on-device OCR asks for consent before any read.
- Peer agents: authorize other Leah instances (yours or a teammate's) to answer scoped questions, with per-question consent and a spending cap.
- Plugin SDK: drop-in extensions run in a sandbox with their own budget. Ships with a weather plugin as a reference.
- Spending guardrails: one shared ledger tracks what every feature costs; when the cap fires, everything stops together.
- Watchdog: heartbeat + circuit breaker + memory-leak detector keep the background service healthy.

## v3.3.0 (2026-06-23)

- Text-to-speech: cloud voice as primary (ElevenLabs Flash v2.5) with an on-device Apple voice as fallback; a classifier chooses which to use based on privacy.
- Wake word: "Hey Leah" ships on-device, off by default. Per-app suppression list so it won't fire during a Zoom call.
- Push-to-talk: Fn key (built-in keyboards), right-Command key (external keyboards).
- Minimal mode: strips grain, italic, and gold accents from the UI (Settings → Appearance).
- Touch ID gate on memory purge and telemetry toggles.
- Live updates: knowledge, memory, and integrations push changes to the HUD as they happen — no polling.
- Citations on streaming answers: sources link back to the exact file/line or Linear issue.
- Peer agent read-only publish: other Leah instances can query your tools, never mutate them.
- Auto-updater: signed appcast on GitHub Pages with a rollback channel toggle in Settings → Advanced.
- Dashboard: unified surface for memory, agenda, briefs, news, and knowledge — reuses existing widget tiles.
- Brand hero assets finalized: 4 hero PNGs plus SVG/PDF mark.
