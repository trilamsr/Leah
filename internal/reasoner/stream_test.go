package reasoner

import (
	"context"
	"strings"
	"testing"

	"github.com/trilam/leah/internal/budget"
)

// streamingFakeClient drives StreamingClient: emits each chunk in order to
// the callback before returning the joined text. Used to verify the
// Reasoner surfaces partial chunks to the operator before the final
// completion lands.
type streamingFakeClient struct {
	chunks []string
}

func (f *streamingFakeClient) Complete(ctx context.Context, system, user string) (CompleteResult, error) {
	return CompleteResult{Text: strings.Join(f.chunks, ""), CostUSD: 0.001}, nil
}

func (f *streamingFakeClient) CompleteStream(ctx context.Context, system, user string, onChunk func(string)) (CompleteResult, error) {
	for _, c := range f.chunks {
		onChunk(c)
	}
	return CompleteResult{Text: strings.Join(f.chunks, ""), CostUSD: 0.001}, nil
}

// TestAskStreamsChunksViaCallback asserts a Reasoner with Stream set
// receives each partial chunk via callback in order before Ask returns.
func TestAskStreamsChunksViaCallback(t *testing.T) {
	c := &streamingFakeClient{chunks: []string{"hel", "lo ", "tri"}}
	var got []string
	r := &Reasoner{
		Client:       c,
		Budget:       &budget.Budget{Ceiling: 1.0},
		SystemPrompt: "you are leah",
		Stream:       func(chunk string) { got = append(got, chunk) },
	}
	out, err := r.Ask(context.Background(), "say hi")
	if err != nil {
		t.Fatalf("ask: %v", err)
	}
	if out != "hello tri" {
		t.Errorf("final text: got %q", out)
	}
	if len(got) != 3 || got[0] != "hel" || got[1] != "lo " || got[2] != "tri" {
		t.Errorf("chunks not streamed in order: %v", got)
	}
}
