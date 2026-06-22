# LLM Provider + Model Strategy for Leah

Date: 2026-06-21
Scope: Personal macOS AI assistant, single operator, ~50–200 queries/day, Go daemon.
Source data: Anthropic claude-api skill (cached 2026-06-04), OpenAI pricing (fetched 2026-06-21), Google AI pricing (fetched 2026-06-21).

---

## TL;DR — Recommendation

**Adopt a two-tier Anthropic stack**: `claude-sonnet-4-6` for user-facing chat + agent reasoning, `claude-haiku-4-5` for utility/router tasks (classification, summarization, reranking, intent detection). Use Anthropic ephemeral prompt caching with a 5-minute TTL on the stable system prompt + conversation history prefix. Go SDK (`github.com/anthropics/anthropic-sdk-go`) matches the existing `leah-daemon` stack and has first-class streaming + tool runner.

**Expected cost @ 100 q/day with the mixed strategy and 85% cache hit rate: ~$20/month** — equivalent to one ChatGPT Plus / Claude Pro subscription, but with full API control, MCP integration, structured outputs, and zero-retention privacy posture.

**Rationale (3 lines):**
1. Sonnet 4.6 is the felt-latency sweet spot — intelligence comparable to gpt-5.4 / Gemini 3.1 Pro at lower output cost ($15/MTok vs $15 / $12), with adaptive thinking that auto-tunes depth instead of fixed budgets.
2. Anthropic's prompt cache is the most aggressive in the market (0.1× read, 1.25× write) and break-even at 2 requests — Leah's 8-turn conversations with stable system prompts hit this on every turn after the first.
3. Anthropic Go SDK is GA, matches the daemon stack, exposes adaptive thinking, server-side tools, and streaming with `.Accumulate()` for clean final-message reconstruction.

---

## 1. Primary chat model — Sonnet 4.6

Comparison on the axes that matter for Leah (chat + agent reasoning + memory recall + voice transcript reasoning):

| Model               | Input $/1M | Output $/1M | Context | Max Output | Adaptive thinking | Prompt cache discount |
|---------------------|-----------:|------------:|--------:|-----------:|------------------|-----------------------|
| claude-haiku-4-5    | $1.00      | $5.00       | 200K    | 64K        | No (effort only) | 0.1× read / 1.25× write |
| **claude-sonnet-4-6** | **$3.00** | **$15.00** | **1M**  | **64K**    | **Yes**          | **0.1× read / 1.25× write** |
| claude-opus-4-8     | $5.00      | $25.00      | 1M      | 128K       | Yes              | 0.1× read / 1.25× write |
| claude-fable-5      | $10.00     | $50.00      | 1M      | 128K       | Always-on        | 0.1× read / 1.25× write |
| gpt-5.4             | $2.50      | $15.00      | n/a     | n/a        | reasoning_effort | 0.1× read (no write fee) |
| gpt-5.5             | $5.00      | $30.00      | n/a     | n/a        | reasoning_effort | 0.1× read (no write fee) |
| gemini-3.1-pro      | $2.00/$4.00 | $12.00/$18.00 | 1M    | n/a        | thinking budgets | 0.1× read + storage fee |

**Verdict: claude-sonnet-4-6.**

- **Intelligence**: Sonnet 4.6 matches gpt-5.4 and Gemini 3.1 Pro on the workloads Leah cares about (multi-turn conversation, tool use, memory recall over notes/transcripts). Opus and Fable are overkill for personal-assistant Q&A; reserve Opus 4.8 escalation for hard agentic or long-horizon coding tasks.
- **TTFT / felt-latency**: Sonnet 4.6 with `effort: "low"` and `thinking: {type: "disabled"}` for short chat turns matches Haiku-class TTFT (~300–500ms first token) while preserving Sonnet-tier reasoning when adaptive thinking kicks in for harder turns. This is the single biggest UX lever — leave adaptive on by default so the model chooses depth per turn.
- **Streaming smoothness**: All three providers stream reliably; Anthropic's SDK exposes `stream.Accumulate(event)` so you don't have to hand-merge delta events. Gemini's streaming has been historically less smooth (occasional reorder bugs); OpenAI is solid but the model itself is more expensive at the same intelligence tier.
- **Context window**: 1M tokens on Sonnet 4.6 is overkill for Leah's per-turn shape but matters for long memory-recall windows; no long-context premium below 200K.

---

## 2. Router / utility tier — Yes, use claude-haiku-4-5

For tasks called 5–20× more often than primary chat (widget classification, conversation summarization for memory, search reranking, intent detection), **route to claude-haiku-4-5**.

| Utility model        | Input $/1M | Output $/1M | Why pick |
|----------------------|-----------:|------------:|----------|
| **claude-haiku-4-5** | **$1.00**  | **$5.00**   | Same vendor, same SDK, same cache, same auth — zero operational overhead. |
| gpt-4o-mini (gpt-5.4-nano) | $0.20 | $1.25 | Cheaper but adds a second vendor, second SDK, second auth path, second monitoring surface. |
| gemini-2.5-flash     | $0.30/$0.625 | $2.50/$5.00 | Cheaper than Haiku but introduces a third API surface. |

**Worth the router complexity? Yes — narrowly.**

- For Leah's volume (~100 q/day primary × 10× utility = 1000 utility calls/day), the cost delta between Haiku and gpt-5.4-nano is ~$3–5/month. Not worth the second vendor.
- Routing complexity is one Go function: `if isUtilityTask(req) { client.Messages.New(..., Model: ModelClaudeHaiku4_5) }`. Cache and tools are shared.
- Stay single-vendor unless monthly utility spend exceeds ~$50/mo, at which point gpt-5.4-nano becomes worth evaluating.

**Do not "just use primary for everything"** — Haiku's 4–5× cheaper input is meaningful when widget classification fires on every keystroke or every voice transcript chunk. The router pays for itself within a week of normal use.

---

## 3. Prompt caching — Anthropic wins for Leah's shape

Leah's per-turn structure:
- System prompt (3000 tokens, stable across all turns and conversations)
- Conversation history (grows from ~0 → ~5000 tokens over an 8-turn convo)
- User query (200 tokens, volatile, always at the end)
- Assistant response (~600 tokens)

After the first turn in a conversation, **96% of input tokens are cacheable** (system + history).

| Provider | Cache write cost | Cache read cost | Min prefix | Break-even | TTL |
|----------|------------------|------------------|-----------|-----------|-----|
| **Anthropic** | **1.25× base** (5min) / 2× (1h) | **0.1× base** | 2048 tok (Sonnet) | **2 requests** | 5min default, 1h opt-in |
| OpenAI | No write fee | 0.1× base | 1024 tok | 1 request | 5–10 min (up to 1h off-peak) |
| Gemini | 1× base | 0.1× base | Higher (varies) | 1 request | + $4.50/1M tokens/hour storage |

**Quantified savings** for an 8-turn conversation (Leah's typical voice/chat session):
- Without caching: 8 × 5700 input tokens × $3/1M = **$0.137**
- With caching (85% hit rate): ~$0.052 input + $0.072 output = **$0.066** (a 52% savings; output dominates after cache)

**Why Anthropic wins for Leah**:
- OpenAI's "no write fee" is appealing on paper, but Sonnet 4.6's base input is $3 vs gpt-5.4's $2.50 — the 0.10× read tier puts them within $0.05 of each other.
- Gemini's storage fee ($4.50/1M tokens/hour) makes implicit caching for short-lived 8-turn convos uneconomical; explicit cache management adds API complexity Leah doesn't need.
- Anthropic auto-places `cache_control` on the last cacheable block via top-level `CacheControl` on `MessageNewParams` — one-line integration.

---

## 4. Cost projection — 1 operator, 100 queries/day (3000 queries/month)

Per-query model: 5700 input / 600 output tokens, 85% cache hit rate after first turn.

| Strategy                              | $/month cached | $/month uncached |
|---------------------------------------|---------------:|-----------------:|
| Sonnet-only                           | **$42.29**     | $78.30 |
| Opus-only                             | $70.48         | $130.50 |
| Fable-only (overkill)                 | ~$141          | ~$261 |
| gpt-5.4-only                          | $38.19         | $69.75 |
| gemini-3.1-pro-only                   | $30.55         | $55.80 |
| **Sonnet primary + Haiku utility (80/20)** | **$19.73** | $36–40 |
| Opus primary + Haiku utility (80/20)  | $25.37         | $50–55 |
| Haiku-only                            | $14.10         | $26.10 |

**Reference subscriptions** (rate-limited, no API):
- ChatGPT Plus: $20/mo
- Claude Pro: $20/mo

**Recommended strategy comes in at $19.73/month** — the same price as a ChatGPT Plus or Claude Pro subscription, but with full API access, MCP, custom tools, structured outputs, and Leah's chosen privacy posture.

Headroom: a 4× usage spike (400 q/day) puts mixed strategy at ~$80/month, still affordable for a personal-use product.

---

## 5. SDK ergonomics — Anthropic Go SDK is the cleanest fit

Leah-daemon is already Go (`go.mod` confirmed in repo). Anthropic provides a first-class Go SDK at `github.com/anthropics/anthropic-sdk-go`.

| Capability | Anthropic Go SDK | OpenAI Go SDK | Google Gemini Go SDK |
|------------|------------------|---------------|----------------------|
| Maturity | GA, actively maintained | GA | GA but less idiomatic Go |
| Streaming | `client.Messages.NewStreaming(...)` + `stream.Accumulate(event)` for final message | Idiomatic | Works, occasional reorder issues |
| Tool runner | Beta `BetaToolRunner` with `RunToCompletion()` + struct-tag schema gen | Manual loop typical | Manual |
| Structured outputs | `output_config.format` + `messages.parse()` helper | Yes | Yes |
| Retry / backoff | Built-in automatic retry on 429 / 5xx | Yes | Yes |
| Prompt cache integration | One-line `CacheControl: anthropic.NewCacheControlEphemeralParam()` | Automatic (no code) | Explicit cache mgmt |
| Adaptive thinking | `ThinkingConfigAdaptiveParam{}` typed | n/a (reasoning_effort) | thinking_budget int |
| Model constants | `anthropic.ModelClaudeSonnet4_6`, `ModelClaudeHaiku4_5_20251001` | Strings | Strings |

**Concrete code shape for Leah's primary call**:

```go
stream := client.Messages.NewStreaming(ctx, anthropic.MessageNewParams{
    Model:     anthropic.ModelClaudeSonnet4_6,
    MaxTokens: 16000,
    System: []anthropic.TextBlockParam{{
        Text:         systemPrompt,
        CacheControl: anthropic.NewCacheControlEphemeralParam(),
    }},
    Thinking: anthropic.ThinkingConfigParamUnion{
        OfAdaptive: &anthropic.ThinkingConfigAdaptiveParam{},
    },
    Messages: convHistory, // last user-turn content block also marked cache_control
})
```

Retry pattern: SDK auto-retries `RateLimitError` and `InternalServerError` with exponential backoff (default `max_retries=2`). Typed errors via `errors.As` against `anthropic.APIError` subclasses.

---

## 6. Privacy posture — Anthropic safest by default

For a personal assistant handling private data (conversations, voice transcripts, memory):

| Provider | Default retention | Zero-retention available? | Used for training? |
|----------|-------------------|---------------------------|--------------------|
| **Anthropic** | 30 days (Fable 5 requires this); zero-retention available for Opus/Sonnet/Haiku via org config | **Yes** (workspace setting) | No |
| OpenAI | 30 days; cached prompts not shared across orgs | Yes (ZDR by request, Enterprise tier) | No on API by default |
| Gemini Paid | "No" on training (paid tier) | Limited; storage fee applies for cache | No on paid tier; **Yes on free tier** |

**Why Anthropic wins**:
- Org-level zero-retention is a checkbox in Console — no contract negotiation, no Enterprise tier required.
- Sonnet 4.6 works under zero-retention (unlike Fable 5, which requires 30-day retention).
- Cache key/value tensors expire ≤24h max; in-memory caching available for ZDR orgs.
- No training on API data, no third-party data sharing.

For Leah, set the Anthropic workspace to ZDR. This is the strongest default privacy posture without sacrificing any feature Leah needs.

---

## Final recommendation

Use **claude-sonnet-4-6** as Leah's primary user-facing model (chat, agent reasoning, voice transcript reasoning, memory recall) with adaptive thinking on by default. Route utility tasks (widget classification, conversation summarization for memory, search reranking, intent detection) to **claude-haiku-4-5** behind a single `if isUtilityTask` switch. Enable Anthropic's ephemeral prompt cache (5-minute TTL) on the stable system-prompt + conversation-history prefix to get 0.1× read pricing on ~85% of input tokens after the first turn. Wire it through the official `github.com/anthropics/anthropic-sdk-go` Go SDK to match the existing leah-daemon stack. Set the Anthropic workspace to zero data retention. Reserve `claude-opus-4-8` as an explicit escalation path for hard agentic or long-horizon coding work — not the default. **Projected steady-state cost: ~$20/month** at 100 q/day, equivalent to a single ChatGPT Plus subscription but with full API control, structured outputs, MCP tool integration, and the strongest default privacy posture in the market.

---

## Vendor comparison summary

| Axis | Anthropic | OpenAI | Google |
|------|-----------|--------|--------|
| Primary chat model | Sonnet 4.6 ($3/$15) | gpt-5.4 ($2.50/$15) | Gemini 3.1 Pro ($2/$12) |
| Cheap utility model | Haiku 4.5 ($1/$5) | gpt-5.4-nano ($0.20/$1.25) | gemini-2.5-flash ($0.30/$2.50) |
| Cache read discount | 90% off (0.1×) | 90% off (0.1×) | 90% off + storage fee |
| Cache write premium | 1.25× | None | 1× + $4.50/1M tok/hr storage |
| Go SDK quality | GA, idiomatic, tool runner | GA | GA, less idiomatic |
| ZDR availability | Org-level checkbox | Enterprise tier / by request | Paid tier opt-in, limited |
| Adaptive reasoning | adaptive thinking (auto-tuned) | reasoning_effort levels | thinking_budget int |
| MCP / tool integration | First-class, native | Via shims | Via shims |

## Cost summary @ 100 q/day, 85% cache hit

| Stack | $/month |
|-------|---------|
| Sonnet 4.6 + Haiku 4.5 (recommended) | **~$20** |
| Sonnet 4.6 only | ~$42 |
| Opus 4.8 + Haiku 4.5 | ~$25 |
| Opus 4.8 only | ~$70 |
| gpt-5.4 + gpt-5.4-nano | ~$15–18 (savings not worth 2nd vendor) |
| Gemini 3.1 Pro only | ~$30 |
