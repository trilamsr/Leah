# Adopt vs Build Survey — Wave 3+ Features

Author: research subagent
Date: 2026-06-09
Operator directive: **"adopt and not build approach"**
Scope: 12 candidate features for Leah Wave 3+ roadmap

## Methodology

Each item below is graded along five axes:

- **Current state (2026)** — latest release, license, maturity signal
- **Cost** — free / per-call / subscription / hardware-only
- **Maintenance** — active commits, bus-factor, governance
- **Recommendation** — `ADOPT` (use as-is) / `WRAP` (thin Go shim around binary or HTTP API) / `BUILD` (custom required) / `DEFER` (not ready for self-host operator)
- **Justification** — 1-2 sentences anchored to operator-only self-host context

Citations resolve at the URL + accessed-date stamped per item. Live-fetched 2026-06-09 (some pages already cached <24h from prior research; cache windows noted inline).

---

## Summary Scoreboard

| #   | Feature                              | Pick                              | Verdict |
| --- | ------------------------------------ | --------------------------------- | ------- |
| 1   | Voice STT                            | whisper.cpp `large-v3-turbo-q5_0` | ADOPT   |
| 2   | Voice TTS                            | Kokoro-82M local + OpenAI fallback | WRAP   |
| 3   | Wake-word ("Hey Leah")               | openWakeWord                      | DEFER   |
| 4   | Email IMAP / Gmail Go client         | emersion/go-imap v2 + google api/gmail/v1 | ADOPT   |
| 5   | Calendar CalDAV / Google Calendar Go | google.golang.org/api/calendar/v3 | ADOPT   |
| 6   | Slack DM Go                          | slack-go/slack (Socket Mode)      | ADOPT   |
| 7   | Embedding store                      | sqlite-vec                        | ADOPT   |
| 8   | Local LLM                            | Ollama HTTP (`ollama/ollama`)     | WRAP    |
| 9   | Notification system                  | ntfy.sh (HTTP PUT) + osascript    | WRAP    |
| 10  | Voice waveform visualization         | wavesurfer.js                     | DEFER   |
| 11  | Memory embeddings ("leah recall")    | voyage-3-large remote + bge-m3 local fallback | WRAP |
| 12  | Personal data backup                 | restic 0.19.0                     | ADOPT   |

Totals: **6 ADOPT, 4 WRAP, 0 BUILD, 2 DEFER.** No item warrants a from-scratch build at this stage.

---

## 1. Voice STT (push-to-talk, hotkey)

### Current state (2026)

- **whisper.cpp** v1.8.6 (Jun 2, 2026), MIT, ggml-org / Georgi Gerganov. 34 releases, very active. Core ML path for Apple Neural Engine documented in repo.
- **MLX-Whisper** (`ml-explore/mlx-examples/whisper`), MIT, Apple. Active, less performant than whisper.cpp on the Mac Whisper benchmark at the time of writing.
- **NVIDIA Parakeet TDT 0.6B v2** (`nvidia/parakeet-tdt-0.6b-v2`), CC-BY-4.0, English-only, 25-lang variant exists. Top of HF ASR leaderboard; 360K downloads/month.
- **Distil-Whisper** (`huggingface/distil-whisper`), MIT, English-only, 6× faster than full-size Whisper but obsoleted by `large-v3-turbo` released after it.
- **OpenAI Whisper API** — paid per second, no MIT/local story.

### Cost

- whisper.cpp: free, CPU-only hardware OK; ANE acceleration adds zero cost on Apple Silicon.
- Parakeet: free model weights, but 16 GB+ VRAM baseline; Mac MLX port works.
- OpenAI API: $0.006 / minute = ~$0.36 / hour of dictation. Material at heavy use.

### Maintenance

whisper.cpp: ✅ green (release cadence weekly). Parakeet: NVIDIA-backed, ✅. Distil-Whisper: 🟡 last meaningful work was pre-turbo; superseded. MLX-Whisper: ✅ but Apple-only, narrower contributor base.

### Recommendation: **ADOPT — whisper.cpp `large-v3-turbo-q5_0`** (the existing plan)

### Justification

The 2026 mac-whisper-speedtest benchmark shows parakeet-mlx at 0.50s, mlx-whisper at 1.02s, whisper.cpp turbo-q5_0 at 1.23s, and WhisperKit Core ML at 0.19s. Parakeet is faster on English but loses 99→25 language coverage and needs more memory; WhisperKit is faster still but Argmax-controlled and Mac-only. For a single-operator macOS-first tool that may need multilingual dictation, whisper.cpp + Core ML is the lowest-risk pick and the existing plan stands. Reopen-trigger: if the operator only dictates English AND latency under 200ms matters, switch to WhisperKit; if a Linux dev box appears, fall back to plain whisper.cpp.

### Citations

- whisper.cpp repo, v1.8.6, accessed 2026-06-09 → https://github.com/ggerganov/whisper.cpp
- mac-whisper-speedtest benchmark, accessed 2026-06-09 → https://github.com/anvanvan/mac-whisper-speedtest
- Parakeet TDT 0.6B v2 model card, accessed 2026-06-09 → https://huggingface.co/nvidia/parakeet-tdt-0.6b-v2
- Distil-Whisper repo, accessed 2026-06-09 → https://github.com/huggingface/distil-whisper

---

## 2. Voice TTS

### Current state (2026)

- **OpenAI `tts-1-hd`** — closed-source, $30 / 1M chars. Mature, English-leading quality but accent options limited.
- **ElevenLabs Flash v2.5** — closed, $5–$330+/month tiered. Best naturalness; ~50ms TTFB on the API.
- **Cartesia Sonic-2 / Ink-2** — closed, "Line" voice-agent tier $0.06/min. Excellent latency.
- **Kokoro-82M** (`hexgrad/kokoro`) — Apache-2.0 weights, 82M params, runs on CPU or Apple MPS. 24kHz output, English-only at v0.9.
- **Apple Neural TTS** — free, on-device, requires AVSpeechSynthesisVoice via cgo; voices vary by macOS version. The `say` CLI is the operator-friendly version of this.

### Cost

- Kokoro: free, runs on the same Mac.
- OpenAI tts-1-hd: $30/1M chars ≈ $0.018 per 600-char paragraph.
- ElevenLabs Flash: ~$0.05/1k chars on the Creator tier.
- Cartesia Sonic: included in Line plan; not surfaced as a la carte char-pricing on the public page.
- Apple `say`: free.

### Maintenance

Kokoro: ✅ active, 1 maintainer (bus-factor=1 — risk). OpenAI/ElevenLabs/Cartesia: ✅ funded. `say`: Apple-maintained forever.

### Recommendation: **WRAP — Kokoro-82M local default + `say` fallback + optional OpenAI for premium voice**

### Justification

The current plan (`tts-1-hd` default, `say` fallback) ties every utterance to network + per-char billing for what is the most predictable workload in the system. Kokoro is Apache-2.0, runs in seconds on Apple Silicon, and sounds materially better than `say`. Wrap with a one-binary fallback chain `kokoro → say → openai (opt-in)`. Reopen-trigger: if voice cloning or multilingual TTS becomes a requirement, revisit ElevenLabs Flash. Bus-factor warning: Kokoro is essentially one person, so the `say` fallback must remain wired in.

### Citations

- Kokoro repo + license, accessed 2026-06-09 → https://github.com/hexgrad/kokoro
- OpenAI TTS docs, accessed 2026-06-09 → https://platform.openai.com/docs/guides/text-to-speech
- ElevenLabs pricing, accessed 2026-06-09 → https://elevenlabs.io/pricing
- Cartesia pricing, accessed 2026-06-09 → https://cartesia.ai/pricing

---

## 3. Wake-word ("Hey Leah")

### Current state (2026)

- **Picovoice Porcupine** — proprietary, on-device. Free tier exists but each custom keyword resource (`.ppn`) expires 30 days from generation on the free console; commercial tier starts at ~$6k/yr per third-party reports. Free tier limited to ≤3 active users / month.
- **openWakeWord** (`dscripka/openWakeWord`) — Apache-2.0 backbone (Google speech embeddings), trainable on fully synthetic data, runs on a single Pi3 core. Active.
- **Mycroft Precise** (`MycroftAI/mycroft-precise`) — original repo exists but Mycroft AI is defunct; community moved to `OpenVoiceOS/precise-lite` and `ovos-ww-plugin-openWakeWord`. Treat as abandoned.

### Cost

- Porcupine free tier: $0 but practically blocked by the 30-day `.ppn` rotation problem for any always-on custom keyword.
- openWakeWord: $0 plus ~1 CPU core's worth of compute.
- Precise: free but defunct.

### Maintenance

Porcupine: ✅ vendor. openWakeWord: 🟡 single maintainer but active. Precise: 🔴 abandoned.

### Recommendation: **DEFER — operator already declared wake-word deferred; reaffirm**

### Justification

Push-to-talk solves >95% of the voice-trigger problem for a single operator at a Mac. Always-on wake-word adds (a) constant mic listening which is privacy-loud, (b) the Porcupine custom-keyword rotation trap, and (c) a tuning burden (false-positive rate) that the operator has zero appetite for at Wave 3. When the operator wants wake-word, openWakeWord is the right pick because it is Apache-2.0 end-to-end and a self-host operator can train "Hey Leah" once. Reopen-trigger: explicit operator ask + a mic that is already always-on (e.g. AirPods Pro headset).

### Citations

- Porcupine repo, accessed 2026-06-09 → https://github.com/Picovoice/porcupine
- Picovoice pricing/Free tier, accessed 2026-06-09 → https://picovoice.ai/pricing/
- openWakeWord repo, accessed 2026-06-09 → https://github.com/dscripka/openWakeWord
- Mycroft Precise repo (abandoned), accessed 2026-06-09 → https://github.com/MycroftAI/mycroft-precise

---

## 4. Email IMAP / Gmail Go client

### Current state (2026)

- **emersion/go-imap** — v2 stable, MIT. Maintained by the long-running emersion mail stack (also `go-message`, `go-sasl`, `go-smtp`, `go-webdav`). Bus factor concentrated but the stack has shipped continuously since ~2016.
- **emersion/go-message** — MIT, streaming RFC-5322 + MIME parser. Required companion for parsing IMAP bodies.
- **google.golang.org/api/gmail/v1** — official Google Go client, BSD-3, autogenerated. Always tracks the JSON-API surface.
- **gmail-api-go** community wrappers — thin and underused vs the official client.

### Cost

All free. Gmail API has a free quota of 1B "quota units" / day per project, well over a single operator's mailbox traffic.

### Maintenance

✅ green across the stack.

### Recommendation: **ADOPT**

Use `emersion/go-imap` v2 + `emersion/go-message` for non-Gmail accounts (Fastmail, iCloud, self-hosted). Use `google.golang.org/api/gmail/v1` for Gmail. Triage logic lives in `internal/email/`, but the protocol/MIME work is outsourced.

### Justification

Both surfaces are mature; reinventing IMAP or Gmail OAuth is the canonical "do not build" decision. emersion is the de facto Go mail stack and is small enough to vendor if needed.

### Citations

- emersion/go-imap repo, accessed 2026-06-09 → https://github.com/emersion/go-imap
- emersion/go-message repo, accessed 2026-06-09 → https://github.com/emersion/go-message
- gmail v1 godoc (v0.283.0), accessed 2026-06-09 → https://pkg.go.dev/google.golang.org/api/gmail/v1

---

## 5. Calendar CalDAV / Google Calendar Go

### Current state (2026)

- **google.golang.org/api/calendar/v3** — official Google Go client, BSD-3, mirrors the REST API. v0.283.0 at access time.
- **emersion/go-webdav** — Apache-2.0 (per repo), supports WebDAV / CalDAV / CardDAV. Active but smaller surface; CalDAV reads are reliable, writes are still maturing per repo notes.

### Cost

Free; Google Calendar API quota is well above single-operator needs.

### Maintenance

✅ both maintained.

### Recommendation: **ADOPT — google calendar/v3 as primary, go-webdav as the iCloud / Fastmail / self-hosted path**

### Justification

Calendar awareness for Tier 3 is read-mostly (next meeting, conflicts). The official Google client is autogenerated and stable; emersion/go-webdav covers everyone else with one library. No reason to write a CalDAV parser in 2026.

### Citations

- calendar v3 godoc (v0.283.0), accessed 2026-06-09 → https://pkg.go.dev/google.golang.org/api/calendar/v3
- emersion/go-webdav repo, accessed 2026-06-09 → https://github.com/emersion/go-webdav

---

## 6. Slack DM Go

### Current state (2026)

- **slack-go/slack** — BSD-2, the canonical Go client. Covers Web API, RTM (deprecated by Slack), Events API webhooks, and Socket Mode. `SocketmodeHandler` is documented as "experimental" in the repo but is the path that avoids exposing a public HTTPS endpoint.

### Cost

Free; Slack app tokens are free for a personal workspace.

### Maintenance

✅ active, multi-contributor.

### Recommendation: **ADOPT — slack-go/slack with Socket Mode**

### Justification

Socket Mode keeps Leah behind the laptop's outbound firewall — no public webhook, no inbound tunnel, no DNS. Exactly what a self-host operator wants for chat watch. `slack-go/slack` is the only serious Go option and is healthy.

### Citations

- slack-go/slack repo, accessed 2026-06-09 → https://github.com/slack-go/slack

---

## 7. Embedding store

### Current state (2026)

- **sqlite-vec** (`asg017/sqlite-vec`) — Apache-2.0 SQLite extension. Single .so/.dylib that drops into any SQLite (including the one we already use for memory). Stable v0.x; rapid development.
- **lancedb** — Apache-2.0, embedded columnar store written in Rust, Python/Node/Rust SDKs primary. Go bindings exist but are thin.
- **chromem-go** — MIT, pure-Go embeddable vector DB with a Chroma-shaped API. Zero third-party deps. In-memory with optional file persistence.

### Cost

All free, all embedded (no server).

### Maintenance

sqlite-vec: ✅ Alex Garcia, active. lancedb: ✅ well-funded (LanceDB Inc). chromem-go: ✅ single maintainer, steady commits, small surface.

### Recommendation: **ADOPT — sqlite-vec**

### Justification

Leah's memory already lives in SQLite. sqlite-vec drops a vector column next to existing rows — zero new data plane, zero new backup target, restic already covers the file. Reopen-trigger: if memory > 10M vectors AND search latency degrades, swap to lancedb. chromem-go is a fine fallback if the cgo SQLite extension load becomes a packaging problem on macOS.

### Citations

- sqlite-vec repo, accessed 2026-06-09 → https://github.com/asg017/sqlite-vec
- lancedb repo, accessed 2026-06-09 → https://github.com/lancedb/lancedb
- chromem-go repo, accessed 2026-06-09 → https://github.com/philippgille/chromem-go

---

## 8. Local LLM

### Current state (2026)

- **llama.cpp** (`ggml-org/llama.cpp`) — MIT, the reference engine. CGO bindings exist but are a maintenance liability for a single-operator tool.
- **Ollama** (`ollama/ollama`) — MIT, wraps llama.cpp + adds an HTTP server, model registry, and one-command install. Already supports Kimi-K2.6, GLM-5.1, MiniMax, DeepSeek, gpt-oss, Qwen, Gemma per repo README.
- **MLX** (Apple) — MIT, fastest path on Apple Silicon for Apple-published models. Python-first; Go integration means subprocess.
- **llamafile** (`Mozilla-Ocho/llamafile`) — Apache-2.0, single-file executables. Great for distribution but ill-fitting for an operator who wants to swap models monthly.

### Cost

Free; hardware-only (RAM/VRAM is the limit, not $).

### Maintenance

llama.cpp + Ollama: ✅ green. MLX: ✅ Apple. llamafile: ✅ Mozilla AI.

### Recommendation: **WRAP — Ollama HTTP client from Go**

### Justification

A subprocess-managed `ollama` daemon at `http://127.0.0.1:11434` gives us model swap (Qwen 7B → Llama 3 70B) by changing one string, isolates the GPU work into a process the OS already handles, and the HTTP surface is trivial to mock in tests. Avoid CGO llama.cpp bindings unless single-binary distribution becomes a hard requirement. Reopen-trigger: if startup latency or memory residency becomes an issue, switch to MLX subprocess on Apple Silicon for the same wrap pattern.

### Citations

- llama.cpp repo, accessed 2026-06-09 → https://github.com/ggml-org/llama.cpp
- Ollama repo, accessed 2026-06-09 → https://github.com/ollama/ollama
- llamafile repo, accessed 2026-06-09 → https://github.com/Mozilla-Ocho/llamafile

---

## 9. Notification system

### Current state (2026)

- **macOS `osascript`** — already in use; sufficient for foreground alerts. No bundle ID = no Notification Center action handling.
- **Pushover** — already wired; $5 lifetime per device, paid HTTP service, reliable but vendor-locked.
- **ntfy.sh** (`binwiederhier/ntfy`) — Apache-2.0, self-hostable HTTP PUT/POST notification server with iOS/Android/desktop apps. Public instance free; self-host is one `ntfy serve` binary.
- **Apple UserNotifications via cgo** — requires a signed app bundle. Heavy lift for a CLI tool.
- **dunst** — Linux only; defer until Leah has a Linux target.

### Cost

ntfy.sh public instance: free. Self-host: $0 + a $5/mo droplet if not running it on the Mac itself.

### Maintenance

ntfy: ✅ active, multi-contributor.

### Recommendation: **WRAP — ntfy.sh (self-hosted or public) + keep `osascript` for foreground**

### Justification

Pushover works but adds a vendor and a per-device cost. ntfy gives the same "phone buzz from a Go program" with a single HTTP PUT and is self-hostable when the operator wants to keep the notification path inside their own perimeter. Wrap pattern: `notify(channel, body)` chooses `osascript` if foreground-mac, `ntfy.sh` HTTP PUT otherwise. Reopen-trigger: if a real native macOS UI surface ships, revisit UserNotifications via Swift sidecar.

### Citations

- ntfy.sh repo, accessed 2026-06-09 → https://github.com/binwiederhier/ntfy

---

## 10. Voice waveform visualization

### Current state (2026)

- **wavesurfer.js** (`katspaugh/wavesurfer.js`) — BSD-3, the de-facto waveform component for web. Active, plugin ecosystem (regions, spectrogram, timeline).
- **Tone.js** — MIT, Web Audio synthesis library. Overkill for visualization-only.
- **Raw Canvas API** — zero dep, ~200 lines for a JARVIS-style bar visualizer.

### Cost

All free.

### Maintenance

wavesurfer.js: ✅. Tone.js: ✅.

### Recommendation: **DEFER**

### Justification

Leah has no UI surface today. Speculating on waveform visualization before there is a place to render it is the textbook "default simpler" anti-pattern. When a UI lands (dashboard, status bar applet), pick wavesurfer.js for real audio playback; pick a 50-line Canvas bar visualizer for the "is the mic hot" indicator. Building it now would be building inventory for a customer that does not exist.

### Citations

- wavesurfer.js repo, accessed 2026-06-09 → https://github.com/katspaugh/wavesurfer.js

---

## 11. Memory embeddings for "leah recall" semantic search

### Current state (2026)

- **voyage-3-large** — closed-source API, $0.12 / 1M tokens, top MTEB score (65.1) in 2026.
- **voyage-code-3** — closed-source API, $0.18 / 1M tokens, code-tuned.
- **OpenAI text-embedding-3-large** — $0.13 / 1M tokens (Batch API: $0.065). Already authenticated in the OpenAI key.
- **OpenAI text-embedding-3-small** — $0.02 / 1M tokens (Batch: $0.01). Excellent floor.
- **BGE-M3 / bge-large-en-v1.5** — Apache-2.0 / MIT, runs locally via Ollama or llama.cpp.

### Cost

Realistic Leah memory: ~100K stored chunks, ~300 tokens each = 30M tokens.

- voyage-3-large initial bulk: $3.60
- text-embedding-3-large: $3.90 (Batch: $1.95)
- text-embedding-3-small: $0.60
- BGE-M3 local: $0 (CPU-seconds only)

Steady-state ingest is negligible at single-operator volume.

### Maintenance

Voyage: ✅ (Anthropic-aligned). OpenAI: ✅. BGE: ✅ BAAI.

### Recommendation: **WRAP — voyage-3-large as quality default, bge-m3 local fallback when offline**

### Justification

Quality differences at the top of the MTEB are real; voyage-3-large is the current SoTA general-purpose model and the operator already pays per-token elsewhere. Local BGE-M3 keeps "leah recall" working on a plane without network. Wrap pattern: `embed(text)` selects backend by `LEAH_EMBED=voyage|openai|local`. Avoid voyage-code-3 unless code search becomes the dominant query — voyage-3-large covers code at near-parity. Reopen-trigger: if monthly embedding cost crosses $20, swap default to text-embedding-3-small or BGE local.

### Citations

- Voyage embeddings docs + pricing, accessed 2026-06-09 → https://docs.voyageai.com/docs/embeddings + https://docs.voyageai.com/docs/pricing
- MTEB leaderboard (HF Space), accessed 2026-06-09 → https://huggingface.co/spaces/mteb/leaderboard
- Embedding comparison roundup, accessed 2026-06-09 → https://tokenmix.ai/blog/text-embedding-models-comparison

---

## 12. Personal data backup

### Current state (2026)

- **restic** — BSD-2, v0.19.0 (Jun 9, 2026 — released the same day as this survey). 50 releases, very mature. The reference deduplicating-encrypted-backup tool.
- **rustic** (`rustic-rs/rustic`) — Apache-2.0 / MIT dual, Rust rewrite reading + writing the restic repo format. Windows still experimental.
- **kopia** — Apache-2.0, Go, CLI + GUI, encrypted dedup + compression. Production-grade.
- **borgmatic** — wraps BorgBackup; mature but Borg's repo format is older and less actively evolved than restic's.

### Cost

All free; storage cost (S3, B2, local disk) is the variable.

### Maintenance

restic: ✅ very active. rustic: ✅ active and bidirectionally restic-compatible. kopia: ✅ active. borgmatic: ✅ maintained but slowing.

### Recommendation: **ADOPT — restic 0.19.0 (reaffirm existing pick)**

### Justification

restic shipped a stable release **today** (2026-06-09 v0.19.0); the project is unambiguously alive. rustic is interesting as a faster CLI against the same repo format but does not justify swapping the default given Mac arm64 first-party support in restic. kopia has a nicer GUI but adds a parallel data plane the operator does not need. Existing plan stands. Reopen-trigger: if restic backup wall-clock exceeds 30 minutes on the operator's dataset, evaluate rustic against the same repo without migration.

### Citations

- restic repo, v0.19.0 released 2026-06-09, accessed 2026-06-09 → https://github.com/restic/restic
- rustic repo, accessed 2026-06-09 → https://github.com/rustic-rs/rustic
- kopia repo, accessed 2026-06-09 → https://github.com/kopia/kopia

---

## Cross-cutting observations

1. **No BUILD verdicts.** Every Wave 3+ feature has a credible OSS or commercial primitive in 2026. The operator directive maps cleanly onto reality.
2. **Bus-factor risk concentrated in two places:** Kokoro-82M (one maintainer) and openWakeWord (one maintainer). Both have viable closed/paid fallbacks (OpenAI TTS / Picovoice), so the risk is "may need to swap" not "may have to write from scratch."
3. **Subprocess wrap > CGO bindings** for every heavy model: whisper.cpp (binary), Ollama (HTTP), Kokoro (Python subprocess or HTTP), restic (binary). Each one isolates a non-Go runtime from Leah's binary and makes the OS the supervisor.
4. **Two DEFER calls** (wake-word, voice waveform UI) are speculative-only; defer until operator demand is visible.
5. **Cost ceiling at heavy use** (Wave 3+ self-host, single operator): embeddings + voice STT (OpenAI fallback) + TTS (OpenAI fallback) projects to well under $20/month. Hosting and storage cost dominates only if Pushover gets swapped for an SMS gateway.

---

## Appendix: items intentionally NOT surveyed

- General-purpose vector databases (Pinecone, Qdrant, Milvus, Weaviate) — all client-server, anti-self-host.
- Whisper hosted forks (Replicate, Modal, Deepgram) — collapse to "OpenAI Whisper API" cost-bucket without OpenAI's accuracy advantage.
- Email full clients (Thunderbird, Mailpile) — out of scope; Leah needs a triage agent, not a UI.
- Borg / Duplicacy — covered implicitly under "backup" category; restic ecosystem dominates.
