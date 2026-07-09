# leah social strategist — persona-shape decision (MVP)

Decision date: 2026-06-21
Scope: persona shape only. Full design (prompt assembly, Higgsfield/OpenAI wiring, post pipeline, scheduling) lives in a separate spec.

## Decision

**D — operator-defined persona**, sourced from `~/.config/leah/strategist/persona.md`, with a built-in **A (single voice) fallback** baked in when the file is absent or empty.

The strategist reads persona.md verbatim and forwards it into the generation prompt as the voice contract. No per-platform routing, no per-audience classifier, no LLM-inferred tone. If `persona.md` is missing, the strategist uses a hard-coded conservative default ("first-person, plain, no marketing voice, no hashtag spam") and emits a one-line `leah strategist init` hint on stderr.

## Justification (3 bullets)

- **Personal use, single operator, ~50 posts/mo.** YAGNI on per-platform/per-audience matrices we'd have to hand-tune anyway. The operator already mentally segments LinkedIn vs TikTok; the persona doc captures that segmentation in one place under operator control — no schema to invent now.
- **Voice leakage is the real failure mode** for an LLM social pipeline. Options A/B/C all delegate voice authorship to the model and accept some drift. D makes the operator the source of voice truth: the model interpolates content, not voice. This is the cheapest way to guarantee the output reads like the operator.
- **D forward-compatible with B and C.** A single `persona.md` can grow `## linkedin`, `## tiktok`, or `## peers` / `## employers` sections later without a code change — the strategist re-reads the file each run. B and C close that door (their shape is encoded in code/config schemas); D leaves it open and lets usage data after the first month dictate whether platform or audience sectioning is the right axis.

## What ships in MVP

1. `~/.config/leah/strategist/persona.md` — operator-authored file. Single document, freeform markdown. Strategist loads at every generation.
2. Built-in conservative fallback when the file is absent (string constant in code, not a separate file).
3. `leah strategist init` — writes a starter persona.md template into `~/.config/leah/strategist/` with prompt-anchor comments (tone, audience, do/don't, example post).
4. The strategist prompt assembles: persona.md → topic brief → platform target → format constraints. Same persona text used across LinkedIn / Instagram / Facebook / TikTok in MVP.

## What is explicitly out of scope for MVP

- Per-platform persona routing (option B). Re-evaluate after 30 days of post telemetry.
- Per-audience classifier (option C). Re-evaluate alongside B; collapses into B if the operator finds platform is the dominant axis.
- LLM-inferred persona from prior posts. Voice-from-corpus is a magic-trap and conflicts with "operator is source of voice truth."
- A/B testing persona variants. Single operator, low post volume — no statistical power.

## Re-eval trigger

After 30 days of strategist usage, audit whether any single platform's posts read distinctly off-voice. If yes, ship B as a sectioning convention inside the same persona.md (no schema migration). If no, hold D.
