package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/trilam/leah/internal/operatormodel"
)

// fakeStreamReasoner emits scripted deltas on AskStream; satisfies the
// streamReasoner interface used by suggest's --llm path.
type fakeStreamReasoner struct {
	deltas  []string
	gotUser string // last user prompt seen (for prompt-shape assertions)
}

func (f *fakeStreamReasoner) AskStream(ctx context.Context, user string) (<-chan string, error) {
	f.gotUser = user
	out := make(chan string, len(f.deltas))
	go func() {
		defer close(out)
		for _, d := range f.deltas {
			select {
			case <-ctx.Done():
				return
			case out <- d:
			}
		}
	}()
	return out, nil
}

func TestStreamLLMPhrasing_WritesDeltasInOrderWithSingleTrailingNewline(t *testing.T) {
	sr := &fakeStreamReasoner{deltas: []string{"Try ", "checking ", "the queue."}}
	recs := []operatormodel.Recommendation{
		{Kind: "ship", Reason: "you usually ship around now", Weight: 0.7},
	}
	var buf bytes.Buffer
	if err := streamLLMPhrasing(context.Background(), sr, &buf, recs); err != nil {
		t.Fatalf("streamLLMPhrasing: %v", err)
	}
	got := buf.String()
	want := "Try checking the queue.\n"
	if got != want {
		t.Fatalf("output mismatch:\n got: %q\nwant: %q", got, want)
	}
	if strings.Count(got, "\n") != 1 {
		t.Errorf("want exactly one trailing newline, got %d", strings.Count(got, "\n"))
	}
	if !strings.Contains(sr.gotUser, "ship") || !strings.Contains(sr.gotUser, "you usually ship around now") {
		t.Errorf("prompt missing template context: %q", sr.gotUser)
	}
}

func TestStreamLLMPhrasing_MultipleRecsEachOnOwnLine(t *testing.T) {
	sr := &fakeStreamReasoner{deltas: []string{"A", "B"}}
	recs := []operatormodel.Recommendation{
		{Kind: "ship", Reason: "r1", Weight: 0.5},
		{Kind: "ask", Reason: "r2", Weight: 0.3},
	}
	var buf bytes.Buffer
	if err := streamLLMPhrasing(context.Background(), sr, &buf, recs); err != nil {
		t.Fatalf("streamLLMPhrasing: %v", err)
	}
	// Two recs → two streamed phrasings, each terminated by \n.
	want := "AB\nAB\n"
	if got := buf.String(); got != want {
		t.Fatalf("output mismatch:\n got: %q\nwant: %q", got, want)
	}
}
