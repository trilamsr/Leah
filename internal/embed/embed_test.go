package embed

import (
	"context"
	"math"
	"path/filepath"
	"testing"

	"github.com/trilam/leah/internal/memory"
)

// TestHashGeneratorEmbedsConsistentVector asserts the offline hash generator
// is deterministic: same text in → identical vector out (#semantic-recall).
//
// HashGenerator is the no-network fallback used when LEAH_EMBED_BACKEND is
// unset or the operator runs offline. Determinism is the load-bearing
// property — without it, "did we already embed this?" + reproducible tests
// fall apart.
func TestHashGeneratorEmbedsConsistentVector(t *testing.T) {
	g := NewHashGenerator(256)
	ctx := context.Background()
	in := []string{"the quick brown fox", "leah recall semantic search"}

	a, err := g.Embed(ctx, in)
	if err != nil {
		t.Fatalf("Embed pass 1: %v", err)
	}
	b, err := g.Embed(ctx, in)
	if err != nil {
		t.Fatalf("Embed pass 2: %v", err)
	}
	if len(a) != len(in) || len(b) != len(in) {
		t.Fatalf("want %d vectors per call, got a=%d b=%d", len(in), len(a), len(b))
	}
	for i := range a {
		if len(a[i]) != g.Dim() {
			t.Fatalf("vector[%d] dim = %d, want %d", i, len(a[i]), g.Dim())
		}
		for j := range a[i] {
			if a[i][j] != b[i][j] {
				t.Fatalf("vector[%d][%d] not deterministic: %v vs %v", i, j, a[i][j], b[i][j])
			}
		}
		if !approxEq(vectorNorm(a[i]), 1.0, 1e-5) {
			t.Fatalf("vector[%d] not L2-normalized; norm=%v", i, vectorNorm(a[i]))
		}
	}
}

// TestHashGeneratorDistinguishesText asserts unrelated strings produce
// distinguishable vectors (cosine < 0.99 — sloppy but catches "everything
// embeds to the same thing" regressions).
func TestHashGeneratorDistinguishesText(t *testing.T) {
	g := NewHashGenerator(256)
	ctx := context.Background()
	out, err := g.Embed(ctx, []string{"alpha bravo charlie", "xylophone whisky zulu"})
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	sim := Cosine(out[0], out[1])
	if sim > 0.99 {
		t.Fatalf("unrelated strings cosine=%v, want <0.99 (hash collapsed)", sim)
	}
}

// TestCosineOrthogonalAndIdentical asserts the sim primitive against two
// load-bearing endpoints: identical vectors → 1, orthogonal → ~0.
func TestCosineOrthogonalAndIdentical(t *testing.T) {
	a := []float32{1, 0, 0, 0}
	b := []float32{1, 0, 0, 0}
	if got := Cosine(a, b); !approxEq(float64(got), 1.0, 1e-6) {
		t.Fatalf("Cosine(a,a) = %v, want 1.0", got)
	}
	c := []float32{0, 1, 0, 0}
	if got := Cosine(a, c); !approxEq(float64(got), 0.0, 1e-6) {
		t.Fatalf("Cosine(orthogonal) = %v, want 0.0", got)
	}
}

// TestStoreRoundTripsViaSQLite asserts a put → search cycle against a real
// SQLite-backed schema-v5 embedding table returns the seeded item at top
// rank. This is the integration test the spec calls out
// (TestEmbedRoundTripsViaSqliteVec) — renamed for the simpler JSON-blob
// path documented in docs/specs.
func TestStoreRoundTripsViaSQLite(t *testing.T) {
	dir := t.TempDir()
	ms, err := memory.NewStore(filepath.Join(dir, "memory.db"))
	if err != nil {
		t.Fatalf("memory.NewStore: %v", err)
	}
	t.Cleanup(func() { _ = ms.Close() })

	g := NewHashGenerator(64)
	es := NewStore(ms.DB(), g)
	ctx := context.Background()

	docs := []Item{
		{ID: "1", Type: "audit", Content: "ran make ci-check pre-push"},
		{ID: "2", Type: "audit", Content: "merged PR 1148 dashboard pipeline panel"},
		{ID: "3", Type: "decision", Content: "switched to sonnet-4-6 for cost"},
	}
	if err := es.Put(ctx, docs); err != nil {
		t.Fatalf("Put: %v", err)
	}

	hits, err := es.Search(ctx, "dashboard pipeline", 2)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) == 0 {
		t.Fatalf("Search returned 0 hits; want at least 1")
	}
	if hits[0].Item.ID != "2" {
		t.Fatalf("top hit ID=%q, want %q", hits[0].Item.ID, "2")
	}
}

// TestStorePutIdempotent asserts re-Putting the same item updates the row
// instead of duplicating — the embedding table is keyed by (item_id, item_type).
func TestStorePutIdempotent(t *testing.T) {
	dir := t.TempDir()
	ms, err := memory.NewStore(filepath.Join(dir, "memory.db"))
	if err != nil {
		t.Fatalf("memory.NewStore: %v", err)
	}
	t.Cleanup(func() { _ = ms.Close() })

	g := NewHashGenerator(32)
	es := NewStore(ms.DB(), g)
	ctx := context.Background()

	item := Item{ID: "x", Type: "audit", Content: "first version"}
	if err := es.Put(ctx, []Item{item}); err != nil {
		t.Fatalf("Put 1: %v", err)
	}
	item.Content = "second version"
	if err := es.Put(ctx, []Item{item}); err != nil {
		t.Fatalf("Put 2: %v", err)
	}

	var n int
	if err := ms.DB().QueryRow(`SELECT COUNT(*) FROM embedding WHERE item_id=? AND item_type=?`,
		item.ID, item.Type).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Fatalf("row count=%d, want 1 (upsert broken)", n)
	}
}

// TestStoreSearchSkipsForeignModel asserts rows written by generator A do
// not surface in a Search run by generator B. Cross-model vectors are
// incomparable; surfacing them would return nonsense scores.
func TestStoreSearchSkipsForeignModel(t *testing.T) {
	dir := t.TempDir()
	ms, err := memory.NewStore(filepath.Join(dir, "memory.db"))
	if err != nil {
		t.Fatalf("memory.NewStore: %v", err)
	}
	t.Cleanup(func() { _ = ms.Close() })

	gA := NewHashGenerator(64)
	gB := NewHashGenerator(128) // different Name() because different dim
	ctx := context.Background()

	if err := NewStore(ms.DB(), gA).Put(ctx, []Item{
		{ID: "1", Type: "audit", Content: "wrote by generator A"},
	}); err != nil {
		t.Fatalf("Put A: %v", err)
	}
	hits, err := NewStore(ms.DB(), gB).Search(ctx, "generator A", 5)
	if err != nil {
		t.Fatalf("Search B: %v", err)
	}
	if len(hits) != 0 {
		t.Fatalf("Search B returned %d hits, want 0 (cross-model leak)", len(hits))
	}
}

// approxEq is a sloppy float compare for test thresholds.
func approxEq(a, b, eps float64) bool { return math.Abs(a-b) <= eps }

// vectorNorm computes the L2 norm of v (for test assertions only).
func vectorNorm(v []float32) float64 {
	var s float64
	for _, x := range v {
		s += float64(x) * float64(x)
	}
	return math.Sqrt(s)
}
