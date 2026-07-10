// Package embed is leah's semantic-search layer for `leah recall --semantic`.
//
// Storage shape: a single `embedding` table (schema v5) keyed by
// (item_id, item_type), with the raw vector encoded as a little-endian
// float32 BLOB. Search is brute-force cosine in Go — at single-operator
// scale (audit + memory < 10K rows for years) the dot-product over a
// few thousand 256-dim vectors is sub-millisecond and avoids the
// packaging cost of sqlite-vec (cgo + native extension load).
//
// Why not sqlite-vec? Leah uses modernc.org/sqlite (pure-Go, no cgo). The
// sqlite-vec Go binding ships in two flavors: cgo (forces cgo on every
// build target) and ncruces/go-sqlite3 (swaps the entire driver, affecting
// every existing table — contact, project, decision, mistake_log,
// operator_profile, ctxmgr). The adopt-vs-build survey
// (docs/research/2026-06-09-adopt-vs-build-survey.md §7) explicitly names
// chromem-go-style brute-force cosine as the acceptable fallback "if the
// cgo SQLite extension load becomes a packaging problem on macOS." It is,
// so we take the fallback. Reopen-trigger: > 10M embeddings OR p99 search
// latency > 100 ms.
//
// Why HashGenerator default? BGE-code-v1 via ONNX requires cgo +
// `ort.SetSharedLibraryPath(...)` against a system onnxruntime shared
// library — same packaging trap as sqlite-vec. The hash backend is
// deterministic (good for tests + idempotent Put) and runs offline;
// it is NOT semantically meaningful — it groups by lexical-bigram
// overlap, which is enough to beat substring grep at near-miss queries
// (typos, stemming, word reorder) but not enough to match the survey's
// "voyage-3-large remote + bge-m3 local" target. Operators with an
// OpenAI key flip to `text-embedding-3-small` via LEAH_EMBED_BACKEND=openai
// for real semantic recall. Reopen-trigger: operator asks for local
// semantic quality, OR Ollama embeddings (LEAH_EMBED_BACKEND=ollama)
// become tractable.
package embed

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/trilam/leah/internal/keychain"
)

// Generator turns text into fixed-dimension vectors. Implementations MUST
// L2-normalize so cosine reduces to a dot product downstream.
type Generator interface {
	// Embed returns one vector per input string, in order. Each vector
	// has length Dim().
	Embed(ctx context.Context, inputs []string) ([][]float32, error)
	// Name identifies the backend in the embedding row (e.g. "hash-256",
	// "openai-text-embedding-3-small"). Used to skip rows produced by a
	// different model — vectors from different models are incomparable.
	Name() string
	// Dim is the fixed output dimension.
	Dim() int
}

// Item is a unit of recallable content. ID + Type form the upsert key.
type Item struct {
	ID      string
	Type    string // "audit" | "contact" | "project" | "decision"
	Content string // the text that gets embedded
}

// Hit is one search result, top-K ranked by Score descending.
type Hit struct {
	Item  Item
	Score float32
}

// SelectGenerator picks a Generator based on LEAH_EMBED_BACKEND. Default
// is "hash" (offline, deterministic, dim 256). "openai" requires
// OPENAI_API_KEY and produces 1536-dim text-embedding-3-small vectors.
// "bge" loads BGE-small-en-v1.5 via ONNX Runtime from LEAH_EMBED_MODEL_PATH
// (cgo build only — see internal/embed/bge.go).
func SelectGenerator() (Generator, error) {
	switch strings.ToLower(os.Getenv("LEAH_EMBED_BACKEND")) {
	case "", "hash":
		return NewHashGenerator(256), nil
	case "openai":
		g, err := NewOpenAIGenerator()
		if err != nil {
			return nil, err
		}
		return g, nil
	case "bge":
		g, err := NewBGEGenerator(bgeModelPathFromEnv())
		if err != nil {
			return nil, err
		}
		return g, nil
	default:
		return nil, fmt.Errorf("embed: unknown LEAH_EMBED_BACKEND=%q (want hash|openai|bge)", os.Getenv("LEAH_EMBED_BACKEND"))
	}
}

// bgeModelPathFromEnv picks the ONNX model path: explicit LEAH_EMBED_MODEL_PATH
// overrides LEAH_MODEL_DIR/<name>.onnx. Empty result lets NewBGEGenerator
// surface the missing-path error rather than the daemon silently degrading.
func bgeModelPathFromEnv() string {
	if p := os.Getenv("LEAH_EMBED_MODEL_PATH"); p != "" {
		return p
	}
	if dir := os.Getenv("LEAH_MODEL_DIR"); dir != "" {
		return dir + "/bge-small-en-v1.5.onnx"
	}
	return ""
}

// Cosine is dot(a,b) / (|a||b|). When both inputs are L2-normalized
// (every Generator MUST normalize), it reduces to plain dot.
func Cosine(a, b []float32) float32 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		x, y := float64(a[i]), float64(b[i])
		dot += x * y
		na += x * x
		nb += y * y
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return float32(dot / (math.Sqrt(na) * math.Sqrt(nb)))
}

// l2Normalize returns v scaled to unit length. Zero vector returned as-is.
func l2Normalize(v []float32) []float32 {
	var s float64
	for _, x := range v {
		s += float64(x) * float64(x)
	}
	if s == 0 {
		return v
	}
	n := float32(math.Sqrt(s))
	out := make([]float32, len(v))
	for i, x := range v {
		out[i] = x / n
	}
	return out
}

// encodeVector packs []float32 as little-endian bytes for BLOB storage.
func encodeVector(v []float32) []byte {
	buf := new(bytes.Buffer)
	buf.Grow(len(v) * 4)
	_ = binary.Write(buf, binary.LittleEndian, v)
	return buf.Bytes()
}

// decodeVector inverts encodeVector. Wrong-length blobs return an error.
func decodeVector(b []byte, dim int) ([]float32, error) {
	if len(b) != dim*4 {
		return nil, fmt.Errorf("embed: blob len %d != dim %d * 4", len(b), dim)
	}
	v := make([]float32, dim)
	if err := binary.Read(bytes.NewReader(b), binary.LittleEndian, v); err != nil {
		return nil, fmt.Errorf("embed: decode blob: %w", err)
	}
	return v, nil
}

// --- HashGenerator -------------------------------------------------------

// HashGenerator is a deterministic, network-free embedding. Each input is
// hashed bigram-by-bigram into a fixed-dim accumulator, then L2-normalized.
// Quality: enough to beat plain LIKE %q% on typos/reorder; NOT a semantic
// model. Default backend until operator opts into a real model.
type HashGenerator struct{ dim int }

// NewHashGenerator builds a HashGenerator producing dim-element vectors.
func NewHashGenerator(dim int) *HashGenerator {
	if dim <= 0 {
		dim = 256
	}
	return &HashGenerator{dim: dim}
}

// Embed implements Generator.
func (g *HashGenerator) Embed(_ context.Context, inputs []string) ([][]float32, error) {
	out := make([][]float32, len(inputs))
	for i, s := range inputs {
		out[i] = g.embedOne(s)
	}
	return out, nil
}

// Name implements Generator.
func (g *HashGenerator) Name() string { return fmt.Sprintf("hash-%d", g.dim) }

// Dim implements Generator.
func (g *HashGenerator) Dim() int { return g.dim }

// embedOne lower-cases, slides a 3-char window, and hashes each window
// into one of dim buckets with a signed contribution. Stop-word noise is
// not filtered — for personal-scale text it costs more to filter than
// it pays back in retrieval quality.
func (g *HashGenerator) embedOne(s string) []float32 {
	v := make([]float32, g.dim)
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return v
	}
	// 3-char sliding window; tolerant of short strings via window=len(s).
	w := 3
	if len(s) < w {
		w = len(s)
	}
	for i := 0; i+w <= len(s); i++ {
		gram := s[i : i+w]
		sum := sha256.Sum256([]byte(gram))
		bucket := int(binary.BigEndian.Uint32(sum[:4])) % g.dim
		if bucket < 0 {
			bucket += g.dim
		}
		// Sign from the next byte: spreads buckets across +/- so unrelated
		// content does not all collapse to the positive orthant.
		if sum[4]&1 == 0 {
			v[bucket] += 1
		} else {
			v[bucket] -= 1
		}
	}
	return l2Normalize(v)
}

// --- OpenAIGenerator -----------------------------------------------------

const openAIEmbedURL = "https://api.openai.com/v1/embeddings"
const openAIEmbedModel = "text-embedding-3-small"
const openAIEmbedDim = 1536

// OpenAIGenerator calls the OpenAI embeddings endpoint. text-embedding-3-small
// at 1536 dims is the cheapest cloud option per the survey ($0.02 / 1M tokens
// at posting time). HTTP-only — no SDK dep to keep go.mod lean.
type OpenAIGenerator struct {
	apiKey string
	model  string
	http   *http.Client
}

// NewOpenAIGenerator builds an OpenAIGenerator, returning an error if
// OPENAI_API_KEY is unset (fail-fast beats a runtime 401).
func NewOpenAIGenerator() (*OpenAIGenerator, error) {
	key, err := keychain.LoadOpenAIKey()
	if err != nil {
		return nil, fmt.Errorf("embed: load openai key: %w", err)
	}
	if key == "" {
		return nil, fmt.Errorf("embed: OPENAI_API_KEY not set (env or Keychain slot %s/%s; required for LEAH_EMBED_BACKEND=openai)", keychain.OpenAIService, keychain.DefaultAccount)
	}
	return &OpenAIGenerator{
		apiKey: key,
		model:  openAIEmbedModel,
		http:   &http.Client{Timeout: 30 * time.Second},
	}, nil
}

// Name implements Generator.
func (g *OpenAIGenerator) Name() string { return "openai-" + g.model }

// Dim implements Generator.
func (g *OpenAIGenerator) Dim() int { return openAIEmbedDim }

// openAIEmbedReq is the JSON request shape.
type openAIEmbedReq struct {
	Input          []string `json:"input"`
	Model          string   `json:"model"`
	EncodingFormat string   `json:"encoding_format"`
}

// openAIEmbedResp is the (subset of) JSON response shape.
type openAIEmbedResp struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

// Embed batches the inputs into one POST. OpenAI accepts up to 2048 inputs
// per call; leah's recall corpora are 10s-100s of rows, well under the cap.
func (g *OpenAIGenerator) Embed(ctx context.Context, inputs []string) ([][]float32, error) {
	if len(inputs) == 0 {
		return nil, nil
	}
	body, err := json.Marshal(openAIEmbedReq{
		Input: inputs, Model: g.model, EncodingFormat: "float",
	})
	if err != nil {
		return nil, fmt.Errorf("embed: marshal req: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, openAIEmbedURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("embed: build req: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+g.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := g.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embed: openai POST: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("embed: read resp: %w", err)
	}
	var parsed openAIEmbedResp
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("embed: decode resp: %w (body: %s)", err, truncate(string(raw), 200))
	}
	if parsed.Error != nil {
		return nil, fmt.Errorf("embed: openai %s: %s", parsed.Error.Type, parsed.Error.Message)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("embed: openai status %d: %s", resp.StatusCode, truncate(string(raw), 200))
	}
	if len(parsed.Data) != len(inputs) {
		return nil, fmt.Errorf("embed: openai returned %d vectors for %d inputs", len(parsed.Data), len(inputs))
	}
	// OpenAI returns vectors keyed by index (already sorted in practice, but
	// honor the contract).
	out := make([][]float32, len(inputs))
	for _, d := range parsed.Data {
		if d.Index < 0 || d.Index >= len(out) {
			return nil, fmt.Errorf("embed: openai index %d out of range", d.Index)
		}
		// text-embedding-3 vectors arrive already L2-normalized per OpenAI
		// docs; re-normalize defensively so Cosine == dot stays exact.
		out[d.Index] = l2Normalize(d.Embedding)
	}
	return out, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// --- Store ---------------------------------------------------------------

// Store persists + queries embeddings against the shared memory DB. The
// `embedding` table is created by memory.schema.sql (schema v5).
type Store struct {
	db  *sql.DB
	gen Generator
}

// NewStore wires a Generator to the shared DB handle. Callers are
// expected to share memory.Store.DB() rather than open a second connection.
func NewStore(db *sql.DB, gen Generator) *Store {
	return &Store{db: db, gen: gen}
}

// tableName computes the per-(model, dim) physical table name. Spec §17.15
// keeps cloud↔local toggles re-embed-free by parking each backend's vectors
// in its own table — `embeddings_voyage_3_5_lite_1024`,
// `embeddings_bge_small_en_v1_5_384`, etc. The legacy `embedding` table
// (schema v5) is preserved; new writes route here.
func tableName(modelID string, dim int) string {
	var b strings.Builder
	b.Grow(len("embeddings_") + len(modelID) + 8)
	b.WriteString("embeddings_")
	for _, r := range strings.ToLower(modelID) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	fmt.Fprintf(&b, "_%d", dim)
	return b.String()
}

// ensureTable lazy-creates the namespaced embedding table for (model, dim).
// IF NOT EXISTS makes the call idempotent + cheap on the hot path.
func ensureTable(ctx context.Context, tx *sql.Tx, table string) error {
	ddl := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %[1]s (
		  item_id    TEXT NOT NULL,
		  item_type  TEXT NOT NULL,
		  model      TEXT NOT NULL,
		  dim        INTEGER NOT NULL,
		  vector     BLOB NOT NULL,
		  content    TEXT NOT NULL,
		  updated_at TEXT NOT NULL,
		  PRIMARY KEY (item_id, item_type)
		);
		CREATE INDEX IF NOT EXISTS idx_%[1]s_model ON %[1]s(model, dim);`, table)
	if _, err := tx.ExecContext(ctx, ddl); err != nil {
		return fmt.Errorf("embed: create %s: %w", table, err)
	}
	return nil
}

// Put upserts an embedding row per input Item. Re-running with the same
// (ID, Type) replaces the prior row in-place. Rows land in the (model, dim)-
// namespaced table so cloud↔local toggles do not cross-contaminate.
func (s *Store) Put(ctx context.Context, items []Item) error {
	if len(items) == 0 {
		return nil
	}
	texts := make([]string, len(items))
	for i, it := range items {
		texts[i] = it.Content
	}
	vecs, err := s.gen.Embed(ctx, texts)
	if err != nil {
		return fmt.Errorf("embed.Put: generate: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	table := tableName(s.gen.Name(), s.gen.Dim())
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("embed.Put: begin: %w", err)
	}
	// Umbrella rollback — survives Commit (no-op) + a future-added error
	// return that forgets explicit cleanup (R6 SQLite audit).
	defer func() { _ = tx.Rollback() }()
	if err := ensureTable(ctx, tx, table); err != nil {
		return fmt.Errorf("embed.Put: %w", err)
	}
	stmt, err := tx.PrepareContext(ctx, fmt.Sprintf(`
		INSERT INTO %[1]s(item_id, item_type, model, dim, vector, content, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(item_id, item_type) DO UPDATE SET
		  model=excluded.model, dim=excluded.dim, vector=excluded.vector,
		  content=excluded.content, updated_at=excluded.updated_at`, table))
	if err != nil {
		return fmt.Errorf("embed.Put: prepare: %w", err)
	}
	defer func() { _ = stmt.Close() }()
	for i, it := range items {
		if _, err := stmt.ExecContext(ctx,
			it.ID, it.Type, s.gen.Name(), s.gen.Dim(),
			encodeVector(vecs[i]), it.Content, now,
		); err != nil {
			return fmt.Errorf("embed.Put: exec %s/%s: %w", it.Type, it.ID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("embed.Put: commit: %w", err)
	}
	return nil
}

// Search returns the top-K rows ranked by cosine similarity to the query.
// Rows produced by a different model (Name() mismatch) are skipped —
// cross-model vectors are not comparable. With normalized vectors the
// dot product IS cosine; brute-forcing in Go costs ~dim multiplies per row.
func (s *Store) Search(ctx context.Context, query string, k int) ([]Hit, error) {
	if k <= 0 {
		k = 5
	}
	vecs, err := s.gen.Embed(ctx, []string{query})
	if err != nil {
		return nil, fmt.Errorf("embed.Search: embed query: %w", err)
	}
	q := vecs[0]
	table := tableName(s.gen.Name(), s.gen.Dim())
	// Search must not auto-create — read against an absent namespaced table
	// is a clean "no hits" rather than a write side effect on read path.
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(`
		SELECT item_id, item_type, content, vector
		FROM %s WHERE model=? AND dim=?`, table),
		s.gen.Name(), s.gen.Dim())
	if err != nil {
		// Missing namespaced table on first Search before any Put → empty.
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, fmt.Errorf("embed.Search: query: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var hits []Hit
	for rows.Next() {
		var id, typ, content string
		var blob []byte
		if err := rows.Scan(&id, &typ, &content, &blob); err != nil {
			return nil, fmt.Errorf("embed.Search: scan: %w", err)
		}
		v, err := decodeVector(blob, s.gen.Dim())
		if err != nil {
			return nil, fmt.Errorf("embed.Search: %s/%s: %w", typ, id, err)
		}
		hits = append(hits, Hit{
			Item:  Item{ID: id, Type: typ, Content: content},
			Score: Cosine(q, v),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("embed.Search: rows: %w", err)
	}
	sort.Slice(hits, func(i, j int) bool { return hits[i].Score > hits[j].Score })
	if len(hits) > k {
		hits = hits[:k]
	}
	return hits, nil
}
