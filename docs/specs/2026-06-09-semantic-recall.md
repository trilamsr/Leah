# Semantic recall (`leah recall --semantic`)

Tier 1.5 between the substring grep (Tier 1) and Reasoner synthesis (Tier 2).
Cosine search over an `embedding` table keyed by `(item_id, item_type)`.

## Why brute-force cosine (sqlite-vec deferred)

The adopt-vs-build survey (`docs/research/2026-06-09-adopt-vs-build-survey.md`
§7) recommends `sqlite-vec` for the embedding store and explicitly names
brute-force cosine over SQLite blobs as the acceptable fallback "if the cgo
SQLite extension load becomes a packaging problem on macOS."

It is. Leah uses `modernc.org/sqlite` (pure-Go, no cgo). The two
`sqlite-vec` Go binding options both break leah's build profile:

1. `sqlite-vec-go-bindings/cgo` — forces cgo onto every build target.
2. `sqlite-vec-go-bindings/ncruces` — swaps the entire driver, affecting
   every existing table (contact, project, decision, mistake_log,
   operator_profile, ctxmgr, workspace_persona).

At single-operator scale (audit + memory < 10K rows for years), a
brute-force dot product over a few thousand 256-dim normalized vectors
runs in sub-millisecond Go code. Cost paid: ~dim multiplies per row.

**Reopen-trigger**: > 10M embeddings OR p99 search latency > 100 ms in
production audit.

## Why hash generator default (BGE-code-v1 local deferred)

BGE-code-v1 via ONNX requires cgo + `ort.SetSharedLibraryPath(...)`
against a system onnxruntime shared library — same packaging trap as
sqlite-vec. The hash backend is:

- **Deterministic** — same input → identical vector; required for
  idempotent `Put` + reproducible tests.
- **Offline** — no API key, no network.
- **L2-normalized** — cosine reduces to dot product.
- **Sloppy** — lexical-bigram overlap, NOT semantic. Enough to beat
  `LIKE %q%` on typos / word-reorder, NOT enough to match
  voyage-3-large or bge-m3.

Operators with semantic-quality needs flip `LEAH_EMBED_BACKEND=openai`
to use `text-embedding-3-small` (1536-dim, $0.02/1M tokens). API-key
absence is a fail-fast error.

**Reopen-trigger**: operator asks for local semantic quality, OR Ollama
embeddings (`LEAH_EMBED_BACKEND=ollama`) become tractable without cgo.

## Schema (v5)

```sql
CREATE TABLE embedding (
  item_id    TEXT NOT NULL,
  item_type  TEXT NOT NULL,    -- 'audit' | 'contact' | 'project' | 'decision'
  model      TEXT NOT NULL,    -- generator Name() — cross-model rows incomparable
  dim        INTEGER NOT NULL, -- vector length; defends against model-swap drift
  vector     BLOB NOT NULL,    -- little-endian float32, length = dim*4
  content    TEXT NOT NULL,    -- the embedded text (result display)
  updated_at TEXT NOT NULL,
  PRIMARY KEY (item_id, item_type)
);
CREATE INDEX idx_embedding_model ON embedding(model, dim);
```

## CLI shape

```
leah recall <query>                 # Tier 1 (grep, default)
leah recall --semantic <query>      # Tier 1.5 (cosine)
leah recall --llm <query>           # Tier 2 (grep + synth)
leah recall --semantic --llm <query># Tier 1.5 + synth
```

## Generator interface

```go
type Generator interface {
    Embed(ctx context.Context, inputs []string) ([][]float32, error)
    Name() string  // e.g. "hash-256", "openai-text-embedding-3-small"
    Dim() int
}
```

Backend selector: `LEAH_EMBED_BACKEND` env var.

- unset / `hash` → `HashGenerator` (default, offline, 256-dim)
- `openai` → `OpenAIGenerator` (requires `OPENAI_API_KEY`, 1536-dim)

## Followups (not in this PR)

- `leah ingest` command + Put hooks on existing audit/memory writes.
  Schema v5 + the API are landed; the ingest pipeline is the next wedge.
- Reopen-trigger for sqlite-vec adoption (see above).
- Reopen-trigger for local BGE backend (see above).
