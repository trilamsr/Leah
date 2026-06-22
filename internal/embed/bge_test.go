package embed

import (
	"context"
	"os"
	"testing"
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
	if sum < 0.99 || sum > 1.01 {
		t.Fatalf("not L2-normalized: |v|^2 = %f", sum)
	}
}

// TestSelectGenerator_BGECase asserts LEAH_EMBED_BACKEND=bge routes through
// NewBGEGenerator instead of returning the unknown-backend error.
func TestSelectGenerator_BGECase(t *testing.T) {
	t.Setenv("LEAH_EMBED_BACKEND", "bge")
	t.Setenv("LEAH_EMBED_MODEL_PATH", "/nonexistent.onnx")
	_, err := SelectGenerator()
	if err != nil && err.Error() == `embed: unknown LEAH_EMBED_BACKEND="bge" (want hash|openai|bge)` {
		t.Fatal("SelectGenerator does not handle bge case")
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
