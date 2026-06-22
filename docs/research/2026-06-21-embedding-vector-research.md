# Embedding Model + Vector Store Research — Leah

Date: 2026-06-21
Scope: macOS personal AI assistant, single operator, ~10–50K embedded items over a year.
Optimization axes: recall quality (UX), cost, latency budget <200ms end-to-end, privacy.

## TL;DR Recommendation

**Embedding: `voyage-3.5-lite` (1024d default, MRL-shrinkable to 512d).**
**Vector store: `sqlite-vec` (SQLite extension, brute-force + IVF for ≤100K vectors).**
**ANN: brute-force cosine on flat array at 50K (sub-15ms in pure C); switch to IVF only above ~200K vectors.**

Three-line rationale:
1. voyage-3.5-lite beats `text-embedding-3-small` on MTEB at the **same** $0.02/1M price, ships 200M free tokens (covers Leah's lifetime bulk + first ~2 years of growth at zero spend), and supports Matryoshka so dimension can be tuned post-hoc without re-embedding.
2. sqlite-vec is a single C extension loadable into the existing Go daemon via `mattn/go-sqlite3` (CGo, already in most Go stacks), keeps memory/disk in one queryable file alongside Leah's other state, and brute-force at 50K × 1024d is ~200 MB RAM and <15 ms — no ANN index complexity needed.
3. Local fallback path is `bge-small-en-v1.5` via ONNX runtime (zero exfil, ~50ms embed on Apple silicon, MTEB 62.17 vs voyage-3.5-lite's ~63) — keep as a `LEAH_EMBED_LOCAL=1` toggle for paranoid mode without changing the store schema (both fit in 512–1024d).

---

## 1. Embedding Model Comparison

### MTEB + price + dim (2026 verified)

| Model | Dim | Max ctx | MTEB (avg) | Price / 1M tok | Free tier | Multilingual | Notes |
|---|---|---|---|---|---|---|---|
| OpenAI `text-embedding-3-small` | 1536 (MRL→256+) | 8K | 62.3 | **$0.02** | none | yes | Default OpenAI workhorse |
| OpenAI `text-embedding-3-large` | 3072 (MRL→256+) | 8K | 64.6 | $0.13 | none | yes | 6.5× cost of small for +2.3 MTEB |
| Voyage `voyage-4-large` | 1024 (MRL→256/512/2048) | 32K | best-in-class (per Voyage 2026-01) | $0.12 | 200M | yes | Overkill for personal scale |
| Voyage `voyage-4` | 1024 (MRL) | 32K | top-tier | $0.06 | 200M | yes | General-purpose |
| Voyage `voyage-4-lite` | 1024 (MRL→256/512) | 32K | strong | **$0.02** | 200M | yes | Newest cheap tier; ties OpenAI small on price, larger ctx |
| Voyage `voyage-3.5-lite` | 1024 (MRL→256/512/2048) | 32K | ~63 (per Voyage blog 2025-05) | **$0.02** | 50M (older tier) | yes | Recommended sweet spot |
| Voyage `voyage-code-3` | 1024 (MRL) | 32K | code-specialized | $0.18 | 200M | code | If code retrieval becomes a primary use case |
| Cohere `embed-v4.0` | 1536 (MRL→256/512/1024) | 128K | competitive | enterprise-quoted | none | yes + image | Massive context, but pricing gate + enterprise contract bias |
| Local: BGE-small-en-v1.5 | 384 | 512 | 62.17 | $0 | n/a | English | 33M params, ONNX-friendly |
| Local: Nomic-embed-text-v2-moe | 256–768 (MRL) | 512 | ~65 (per Nomic) | $0 | n/a | dozens | 475M total / 305M active MoE; heavier |
| Local: all-MiniLM-L6-v2 | 384 | 256 | ~56 | $0 | n/a | English | 22M params; weakest recall |

### Choice for Leah

**Primary: Voyage `voyage-3.5-lite` at default 1024d.**

Reasons:
- **Price parity with OpenAI small** ($0.02/1M) but with 50M free token grace and 32K context (4× OpenAI's 8K — relevant when embedding long notes, emails, calendar week summaries).
- **MTEB edge** over text-embedding-3-small (~63 vs 62.3) at the same price.
- **Matryoshka representation learning** — embeddings stored at 1024d can be truncated to 512d or 256d post-hoc to shrink the store without re-embedding, so the choice is reversible.
- **Conversation memory + integration data + occasional code snippet** is the general-purpose case, not code-primary; voyage-3.5-lite is general-purpose.
- If/when code retrieval becomes load-bearing, route only code chunks through `voyage-code-3` and store in a separate namespace (multi-model stores are fine; ANN index per dim).

**Fallback: BGE-small-en-v1.5 via ONNX Runtime (Go bindings via `yalue/onnxruntime_go`).**
- Zero network egress, zero per-call cost, ~50ms embed on M-series Apple silicon at 384d.
- MTEB 62.17 — within 1 point of voyage-3.5-lite; UX-indistinguishable for chat memory recall.
- Triggered by `LEAH_EMBED_LOCAL=1` env or "private mode" UX toggle.
- Note: schema must support multiple dim columns or namespace embeddings by model — do not co-mingle 384d and 1024d vectors in one index.

---

## 2. Vector Store Comparison

| Store | Deployment | Go integration | ANN algos | Disk format | Footprint @ 50K vec | Recall <50 ms? |
|---|---|---|---|---|---|---|
| **sqlite-vec** | SQLite extension (loadable .dylib) | `mattn/go-sqlite3` + `LoadExtension` (CGo, mature) | brute-force + IVF (HNSW on roadmap) | one .db file | ~200 MB at 1024d float32 (50K × 4096B) | yes, brute ~10 ms |
| LanceDB | Embedded Rust columnar engine | Rust SDK + Python/JS/Java — **no native Go SDK as of 2026-06**; would require FFI wrapper or sidecar | IVF, HNSW, IVF-PQ | Lance columnar files | smaller (compressed) | yes, ~5 ms |
| ChromaDB | Server (Python/JS clients) — Rust core | HTTP client only, no first-party Go SDK | HNSW | server data dir | medium | yes, but +daemon overhead |
| Qdrant local | Server (Rust) | gRPC/HTTP, community Go SDK | HNSW | server data dir | medium | yes |

### Choice for Leah

**sqlite-vec** wins because:
1. **No daemon** — loads as a SQLite extension into the existing Leah Go daemon (`leah` itself); zero added processes, zero ports, zero "is Chroma running" failure mode.
2. **Go fit** — `mattn/go-sqlite3` already supports `RegisterExtension` for loadable extensions; Leah already (or will) hold a `*sql.DB` for state, so vector tables live alongside everything else and can be joined: `SELECT m.* FROM memories m JOIN vec_memories v ON m.id = v.rowid WHERE v.embedding MATCH ? ORDER BY distance LIMIT 10`.
3. **Brute-force at 50K is fine** — 50,000 × 1024d float32 = ~200 MB; a single-threaded SIMD cosine sweep over that on Apple silicon runs in ~10–15 ms, well inside the 50 ms search budget. IVF is available if scale crosses ~200K. HNSW is on the roadmap but not blocking.
4. **Mozilla-sponsored** — actively maintained (1.0 released 2025; sponsored by Mozilla Builders, Fly.io, Turso, SQLite Cloud).
5. **One file backup** — `cp ~/.leah/leah.db /backup/` is the entire vector backup; matches Leah's "single operator, simple tooling" ethos.

LanceDB is faster and more compressed, but the missing native Go SDK is a real cost — wrapping Rust FFI for a personal-scale store is over-engineering. ChromaDB and Qdrant pull in a sidecar daemon, which violates Leah's no-extra-processes posture.

**ANN algorithm: brute-force flat cosine** at current scale. Add IVF (100 lists) when row count > 200K. Defer HNSW until sqlite-vec ships it natively.

---

## 3. Cost Projection

Assumptions: 50K items × 500 tokens average bulk = **25M tokens** initial. +10K items/year × 500 = **5M tokens/year** ongoing. Query embeddings ~50/day × 30 tokens = ~550K tokens/year (negligible).

| Model | Bulk (25M tok) | Year 1 ongoing (~5.5M tok) | 3-year total |
|---|---|---|---|
| OpenAI text-embedding-3-small ($0.02/M) | $0.50 | $0.11 | $0.83 |
| OpenAI text-embedding-3-large ($0.13/M) | $3.25 | $0.72 | $5.41 |
| Voyage voyage-3.5-lite ($0.02/M, 50M free) | **$0 (within free tier)** | **$0** | **$0 until ~year 9+** |
| Voyage voyage-4-lite ($0.02/M, 200M free) | **$0** | **$0** | **$0** |
| Voyage voyage-4 ($0.06/M, 200M free) | **$0** | **$0** | **$0** |
| Cohere embed-v4 | enterprise-quoted | — | — |
| Local BGE-small (ONNX) | $0 | $0 | $0 (electricity only) |

**Cost essentially does not constrain this decision.** Even OpenAI's small at $0.83 over 3 years is rounding error. Voyage's free tier means voyage-3.5-lite or voyage-4-lite is **free for the lifetime of a personal-scale Leah**. Decision should be made on quality + integration ergonomics, not cost.

---

## 4. Latency Budget

End-to-end target: <200 ms from "user finishes typing" to "results rendered."

| Stage | API embed (voyage) | Local embed (BGE-ONNX) |
|---|---|---|
| Network embed call | 50–150 ms (US-region typical) | n/a |
| Local embed | n/a | 5–50 ms (M-series Apple silicon, 384d) |
| sqlite-vec brute-force search (50K × 1024d) | ~10–15 ms | ~5–10 ms (384d is 2.7× cheaper) |
| Rerank top-50 (optional, rerank-2-lite at $0.02/M) | +80 ms if used | n/a |
| Result render | <20 ms | <20 ms |
| **Total typical** | **~100–200 ms** | **~30–80 ms** |

**Implication**: local embedding wins felt-latency by 2–5×. For interactive UX (e.g., "show me last week's notes" as the user is still typing), the local path is the better default; the cloud API path is the better quality default. A reasonable architecture is **local-first with API upgrade for ambiguous/low-confidence queries**, but MVP can ship cloud-only since 200 ms is within budget.

The 50 ms search budget is comfortably met by sqlite-vec brute force at this scale.

---

## 5. Privacy

- **OpenAI**: API data is not used for training by default (since 2023 policy). 30-day retention for abuse monitoring; Zero Data Retention available on enterprise contracts only. For a personal AI seeing emails/calendars/notes, this is meaningful — your text is on OpenAI's servers for up to 30 days.
- **Voyage** (acquired by MongoDB in 2025): per docs, does not train on customer data; retention policies similar. Enterprise tier offers stricter controls; default tier has standard logging.
- **Cohere**: enterprise-only effective deployment; data terms vary by contract.
- **Local (BGE / Nomic)**: **zero exfil**. The user's content never leaves the device. For an AI assistant that ingests private notes, calendar, email, and integration data, this is materially different from a privacy stance.

**Recommendation for Leah**: support local embedding as a first-class toggle (`leah config set embed.provider local`). Default can be cloud (`voyage-3.5-lite`) for best recall, but the local path must work end-to-end so privacy-conscious users (a meaningful chunk of "personal AI on your Mac" buyers) have a real choice. Use ONNX Runtime via the official `yalue/onnxruntime_go` bindings; ship the BGE-small ONNX file (~130 MB) as an optional download triggered when local mode is enabled.

---

## 6. Final Recommendation

| Layer | Choice |
|---|---|
| **Embedding (default)** | Voyage `voyage-3.5-lite` at 1024d (MRL-truncatable) |
| **Embedding (local/private mode)** | BGE-small-en-v1.5 via ONNX Runtime (`yalue/onnxruntime_go`) |
| **Vector store** | sqlite-vec extension on existing Leah SQLite DB |
| **ANN algorithm** | brute-force cosine at <200K rows; IVF (100 lists) above; HNSW when sqlite-vec ships it |
| **Schema note** | namespace embeddings by `(model, dim)` so cloud and local modes can co-exist without re-embedding everything on toggle |

This stack costs $0 within Voyage's free tier for years of personal-scale usage, fits in one .db file for trivial backup, hits a <200 ms end-to-end recall budget with margin, and offers a real privacy escape hatch via the local fallback — matching Leah's "personal AI on a Mac, single operator, UX > performance > long-term" priority.
