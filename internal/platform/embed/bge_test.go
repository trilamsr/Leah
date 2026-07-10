package embed

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/trilam/leah/internal/memory/store"
)

// TestBGEGenerator_NameAndDim asserts the BGE generator self-identifies as
// bge-small-en-v1.5 / 384d so the embedding-store row filter (model, dim)
// stays correct across cloud<->local toggles (spec §17.15, decision #126).
func TestBGEGenerator_NameAndDim(t *testing.T) {
	modelPath := os.Getenv("LEAH_EMBED_MODEL_PATH")
	if modelPath == "" {
		t.Skip("LEAH_EMBED_MODEL_PATH not set — skipping ONNX test")
	}
	g, err := NewBGEGenerator(modelPath)
	if err != nil {
		t.Fatalf("NewBGEGenerator: %v", err)
	}
	if g.Name() != "bge-small-en-v1.5" {
		t.Fatalf("name: %q", g.Name())
	}
	if g.Dim() != 384 {
		t.Fatalf("dim: %d", g.Dim())
	}
}

// TestBGEGenerator_Embed asserts ONNX inference yields a 384d L2-normalized
// vector — cosine == dot only holds for unit vectors so the contract is
// load-bearing for every downstream cosine search.
func TestBGEGenerator_Embed(t *testing.T) {
	modelPath := os.Getenv("LEAH_EMBED_MODEL_PATH")
	if modelPath == "" {
		t.Skip("LEAH_EMBED_MODEL_PATH not set — skipping ONNX test")
	}
	g, err := NewBGEGenerator(modelPath)
	if err != nil {
		t.Fatalf("NewBGEGenerator: %v", err)
	}
	vecs, err := g.Embed(context.Background(), []string{"hello world"})
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(vecs) != 1 || len(vecs[0]) != 384 {
		t.Fatalf("unexpected shape: %d vecs, dim=%d", len(vecs), len(vecs[0]))
	}
	var sum float64
	for _, x := range vecs[0] {
		sum += float64(x) * float64(x)
	}
	if math.Abs(sum-1.0) > 1e-5 {
		t.Fatalf("not L2-normalized: |v|^2 = %f (want |1-|v|^2| <= 1e-5)", sum)
	}
}

// TestSelectGenerator_BGECase asserts LEAH_EMBED_BACKEND=bge routes through
// NewBGEGenerator with a real model path → non-nil *BGEGenerator. The
// previous double-negative ("if err matches 'unknown backend' fail") silently
// passed when the bge case was missing from the switch.
func TestSelectGenerator_BGECase(t *testing.T) {
	// Materialize a stub .onnx file so NewBGEGenerator's os.Stat passes.
	// Session load is lazy (sync.Once) — no ONNX runtime call here.
	dir := t.TempDir()
	stub := filepath.Join(dir, "bge-small-en-v1.5.onnx")
	if err := os.WriteFile(stub, []byte("stub"), 0o600); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	t.Setenv("LEAH_EMBED_BACKEND", "bge")
	t.Setenv("LEAH_EMBED_MODEL_PATH", stub)

	g, err := SelectGenerator()
	if err != nil {
		t.Fatalf("SelectGenerator: %v", err)
	}
	if g == nil {
		t.Fatal("SelectGenerator returned nil generator")
	}
	if g.Name() != "bge-small-en-v1.5" {
		t.Fatalf("Name()=%q, want bge-small-en-v1.5 (bge case not wired)", g.Name())
	}
	if g.Dim() != 384 {
		t.Fatalf("Dim()=%d, want 384", g.Dim())
	}
}

// TestSelectGenerator_BGEMissingModelNoVoyageFallback asserts that when the
// operator opts into bge but no model file is on disk AND no VOYAGE_API_KEY
// is set, SelectGenerator returns nil + a non-nil error rather than silently
// degrading. Fail-closed is the load-bearing invariant (spec §17.15).
func TestSelectGenerator_BGEMissingModelNoVoyageFallback(t *testing.T) {
	t.Setenv("LEAH_EMBED_BACKEND", "bge")
	t.Setenv("LEAH_EMBED_MODEL_PATH", filepath.Join(t.TempDir(), "missing.onnx"))
	t.Setenv("VOYAGE_API_KEY", "")

	g, err := SelectGenerator()
	if err == nil {
		t.Fatal("want error for missing bge model + no voyage key, got nil")
	}
	if g != nil {
		t.Fatalf("want nil generator on error, got %T", g)
	}
}

// TestBGEGenerator_MissingModelFailsClosed asserts an empty model path is
// rejected at construction — the daemon must NOT silently fall through to
// hash embeddings if the bundle's bge-small-en-v1.5.onnx is missing.
func TestBGEGenerator_MissingModelFailsClosed(t *testing.T) {
	if _, err := NewBGEGenerator(""); err == nil {
		t.Fatal("want error for empty modelPath, got nil")
	}
}

// TestTableNamespacing_CloudLocalToggleNoReembed asserts the spec §17.15
// invariant: vectors written under one (model, dim) live in their own physical
// table and remain untouched when the operator toggles to a different backend.
// Without this the cloud<->local toggle would silently force a re-embed.
func TestTableNamespacing_CloudLocalToggleNoReembed(t *testing.T) {
	dir := t.TempDir()
	ms, err := memory.NewStore(filepath.Join(dir, "memory.db"))
	if err != nil {
		t.Fatalf("memory.NewStore: %v", err)
	}
	t.Cleanup(func() { _ = ms.Close() })

	// Two distinct (model, dim) tuples standing in for cloud (voyage) and
	// local (bge). Using HashGenerator with bespoke names keeps the test
	// cgo-free + ONNX-free.
	cloudGen := &namedHashGen{HashGenerator: NewHashGenerator(1024), name: "voyage-3.5-lite"}
	localGen := &namedHashGen{HashGenerator: NewHashGenerator(384), name: "bge-small-en-v1.5"}

	ctx := context.Background()
	cloudStore := NewStore(ms.DB(), cloudGen)
	localStore := NewStore(ms.DB(), localGen)

	if err := cloudStore.Put(ctx, []Item{{ID: "c1", Type: "audit", Content: "cloud row"}}); err != nil {
		t.Fatalf("cloud Put: %v", err)
	}
	if err := localStore.Put(ctx, []Item{{ID: "l1", Type: "audit", Content: "local row"}}); err != nil {
		t.Fatalf("local Put: %v", err)
	}

	cloudTable := tableName(cloudGen.Name(), cloudGen.Dim())
	localTable := tableName(localGen.Name(), localGen.Dim())
	if cloudTable == localTable {
		t.Fatalf("tableName collision: %s == %s", cloudTable, localTable)
	}

	// Each physical table holds exactly its own row — no cross-write.
	var nCloud, nLocal int
	if err := ms.DB().QueryRow("SELECT COUNT(*) FROM " + cloudTable).Scan(&nCloud); err != nil {
		t.Fatalf("count cloud: %v", err)
	}
	if err := ms.DB().QueryRow("SELECT COUNT(*) FROM " + localTable).Scan(&nLocal); err != nil {
		t.Fatalf("count local: %v", err)
	}
	if nCloud != 1 || nLocal != 1 {
		t.Fatalf("counts cloud=%d local=%d, want 1/1", nCloud, nLocal)
	}

	// Each Search reads only its own table — the other row is invisible AND
	// untouched (no implicit re-embed).
	cloudHits, err := cloudStore.Search(ctx, "cloud row", 5)
	if err != nil {
		t.Fatalf("cloud Search: %v", err)
	}
	if len(cloudHits) != 1 || cloudHits[0].Item.ID != "c1" {
		t.Fatalf("cloud Search hits=%+v, want [c1]", cloudHits)
	}
	localHits, err := localStore.Search(ctx, "local row", 5)
	if err != nil {
		t.Fatalf("local Search: %v", err)
	}
	if len(localHits) != 1 || localHits[0].Item.ID != "l1" {
		t.Fatalf("local Search hits=%+v, want [l1]", localHits)
	}

	// After toggle, both rows STILL exist in their original tables. This is
	// the "no forced re-embed" invariant: switching the active backend does
	// not delete or rewrite vectors keyed under the previous (model, dim).
	if err := ms.DB().QueryRow("SELECT COUNT(*) FROM " + cloudTable).Scan(&nCloud); err != nil {
		t.Fatalf("recount cloud: %v", err)
	}
	if err := ms.DB().QueryRow("SELECT COUNT(*) FROM " + localTable).Scan(&nLocal); err != nil {
		t.Fatalf("recount local: %v", err)
	}
	if nCloud != 1 || nLocal != 1 {
		t.Fatalf("post-toggle counts cloud=%d local=%d, want 1/1", nCloud, nLocal)
	}
}

// namedHashGen wraps HashGenerator with a custom Name() so a single test can
// stand in for two distinct (model, dim) generators without needing cgo/ONNX.
type namedHashGen struct {
	*HashGenerator
	name string
}

func (g *namedHashGen) Name() string { return g.name }

// Compile-time guard: namedHashGen must satisfy Generator.
var _ Generator = (*namedHashGen)(nil)
