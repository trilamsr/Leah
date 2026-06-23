# Orphan code scan 2026-06-23

Scope: every package under `./...` audited for non-test inbound callers. Baseline: origin/main @ dedb595.

## Stats

- Total packages: 109
- `cmd/` entry points: 4 (zero callers by definition — kept)
- Orphans (zero non-test callers, non-cmd): 31
- With ≥1 non-test caller: 74

Method: `go list -f '{{.ImportPath}}|{{range .Imports}}{{.}} {{end}}' ./...` (TestImports excluded from .Imports), edge map built per package, no in-repo non-test caller ⇒ orphan candidate. Cross-checked each candidate with `grep` for blank imports and build-tag-gated callers — none found.

## Confirmed orphans (deletion candidates)

| Package | LOC | Test-LOC | Why it exists | Action |
|---|---|---|---|---|
| `internal/voice/session` | 433 | 584 | Voice-loop substrate; supersession audit confirmed unwired | DELETE — bundle with `wake`/`intents` per 2026-06-22 audit |
| `internal/voice/wake` | 109 | 208 | Voice-loop wake-word substrate; never wired | DELETE — same bundle |
| `internal/voice/intents` | 236 | 205 | Voice-loop intent substrate; never wired | DELETE — same bundle |
| `internal/mcp` | 1206 | 1282 | S11 loopback MCP server (A2A card, publish, redact, tools); spec'd but no daemon wiring | DECIDE — either wire into `cmd/leah-daemon` or delete; largest single orphan |
| `internal/strategist/video` | 346 | 369 | Higgsfield video pipeline scaffolding; no strategist caller | DECIDE — paired with `higgsfield` adapter; both unwired |
| `internal/adapters/higgsfield` | 348 | 499 | Higgsfield image/clip adapter; not referenced by any caller | DECIDE — pair with `strategist/video`; delete or wire |
| `internal/strategist/source/git` | 74 | 86 | Strategist source plugin; only imports parent `source` interface | DELETE — no dispatcher loads it |
| `internal/strategist/source/news` | 56 | 71 | Same shape — source plugin, no dispatcher | DELETE |
| `internal/strategist/source/voice` | 77 | 123 | Same shape — source plugin, no dispatcher | DELETE |
| `internal/macos/bluetooth` | 114 | 171 | macOS surface stub; no daemon wiring | DECIDE — Phase 3 may consume; if not, delete |
| `internal/macos/calendar` | 247 | 364 | macOS surface stub | DECIDE — same |
| `internal/macos/messages` | 309 | 391 | macOS surface stub | DECIDE — same |
| `internal/macos/mirror` | 188 | 197 | macOS surface stub | DECIDE — same |
| `internal/macos/notes` | 226 | 351 | macOS surface stub | DECIDE — same |
| `internal/macos/photos` | 267 | 427 | macOS surface stub | DECIDE — same |
| `internal/macos/reminders` | 246 | 340 | macOS surface stub | DECIDE — same |
| `internal/macos/safari` | 232 | 376 | macOS surface stub | DECIDE — same |
| `internal/macos/shortcuts` | 101 | 206 | macOS surface stub | DECIDE — same |
| `internal/macos/spotlight` | 117 | 165 | macOS surface stub | DECIDE — same |
| `internal/macos/wifi` | 100 | 143 | macOS surface stub | DECIDE — same |
| `internal/tts/apple` | 85 | 99 | Phase 3 Task 3 Apple Ava provider; self-tests only | KEEP-PENDING — Phase 3 Task 4 wires `tts.Provider`; verify wiring lands before deleting |
| `internal/tts/elevenlabs` | 134 | 242 | Phase 3 Task 2 ElevenLabs provider; self-tests only | KEEP-PENDING — same |
| `internal/flights` | 172 | 200 | Trip-planner / flights surface; self-tests only | DECIDE — pair with `tripplanner`; both unwired |
| `internal/maps` | 152 | 148 | Maps surface; self-tests only | DECIDE — same |
| `internal/markets` | 206 | 206 | Markets/finance surface; self-tests only | DECIDE — same |
| `internal/weather` | 190 | 180 | Weather surface; self-tests only | DECIDE — same |
| `internal/tripplanner` | 546 | 701 | Trip-planner core; self-tests only | DECIDE — large; bundle with flights/maps/weather decision |
| `internal/riskscore` | 129 | 155 | Position risk scorer; never wired | DECIDE — markets-domain pair |
| `internal/web/meta` | 484 | 407 | Web metadata extractor; never wired | DECIDE — bundle with brief/news pipeline plan |

Total LOC across deletion candidates (impl only): ~7.4k. Phase-3-pending TTS providers (`tts/apple` + `tts/elevenlabs`, 219 LOC impl) excluded from deletion total.

## Library-by-design (zero callers acceptable)

| Package | LOC | Note |
|---|---|---|
| `internal/obs/obstest` | 67 | Test-helper. 23 test-callers across adapters/connect/feeds/etc. KEEP. |
| `internal/testutil` | 31 | Test-helper. 20 test-callers across cmd-daemon/adapters/macos/voice. KEEP. |

## Entry points (zero callers expected)

`cmd/leah`, `cmd/leah-daemon`, `cmd/leah-eval`, `cmd/leah-hud` — main packages.

## Cross-reference

Supersession audit `docs/engineer/audits/2026-06-22-internal-voice-supersession.md` already identified `voice/session` + `voice/wake` + `voice/intents` (~2.7k LOC including their cascading callers in `voice` parent + `voice/loop`) as deletable pending Phase 3 owner confirmation. This scan confirms zero non-test callers across all three.

## Recommended immediate follow-up PRs

1. **`internal/strategist/source/{git,news,voice}`** — 207 impl LOC, dispatcher absent; safe delete now (or wire into strategist).
2. **Voice substrate bundle** — `voice/session` + `voice/wake` + `voice/intents` (~0.8k impl LOC standalone, ~2.7k with cascading parent symbols). Blocked on Phase 3 owner confirmation per the supersession audit.
3. **`internal/mcp`** — operator decision: ship S11 server-side wiring this wave or delete the 1.2k impl LOC.
