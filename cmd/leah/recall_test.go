package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// fakeRecallStreamReasoner emits scripted deltas on AskStream; satisfies the
// recallStreamReasoner interface used by recall's --llm path.
type fakeRecallStreamReasoner struct {
	deltas  []string
	gotUser string
}

func (f *fakeRecallStreamReasoner) AskStream(ctx context.Context, user string) (<-chan string, error) {
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

func TestStreamRecallSynthesis_WritesDeltasInOrderWithSingleTrailingNewline(t *testing.T) {
	sr := &fakeRecallStreamReasoner{deltas: []string{"You ", "shipped ", "PR #42."}}
	results := []recallResult{
		{Source: "audit", Timestamp: "2026-06-20T10:00:00Z", Text: "ship: merged"},
	}
	var buf bytes.Buffer
	if err := streamRecallSynthesis(context.Background(), sr, &buf, "ship", results); err != nil {
		t.Fatalf("streamRecallSynthesis: %v", err)
	}
	got := buf.String()
	want := "You shipped PR #42.\n"
	if got != want {
		t.Fatalf("output mismatch:\n got: %q\nwant: %q", got, want)
	}
	if strings.Count(got, "\n") != 1 {
		t.Errorf("want exactly one trailing newline, got %d", strings.Count(got, "\n"))
	}
	if !strings.Contains(sr.gotUser, "ship") || !strings.Contains(sr.gotUser, "audit") {
		t.Errorf("prompt missing query/source context: %q", sr.gotUser)
	}
}
