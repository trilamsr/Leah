package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
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

// Unlike suggest --llm (one AskStream per rec), recall packs every hit into a
// single combined prompt and streams once — the synthesis spans all hits. Pin
// that shape: many results still yield one stream + one trailing newline.
func TestStreamRecallSynthesis_ManyResultsOneStreamOneNewline(t *testing.T) {
	sr := &fakeRecallStreamReasoner{deltas: []string{"summary ", "across hits."}}
	results := []recallResult{
		{Source: "audit", Timestamp: "2026-06-20T10:00:00Z", Text: "ship: merged PR #1"},
		{Source: "project", Timestamp: "2026-06-19T09:00:00Z", Text: "[active] leah"},
		{Source: "decision", Timestamp: "2026-06-18T08:00:00Z", Text: "tracker -> Linear"},
	}
	var buf bytes.Buffer
	if err := streamRecallSynthesis(context.Background(), sr, &buf, "ship", results); err != nil {
		t.Fatalf("streamRecallSynthesis: %v", err)
	}
	got := buf.String()
	want := "summary across hits.\n"
	if got != want {
		t.Fatalf("output mismatch:\n got: %q\nwant: %q", got, want)
	}
	if strings.Count(got, "\n") != 1 {
		t.Errorf("want exactly one trailing newline (single combined stream), got %d", strings.Count(got, "\n"))
	}
	for _, hit := range results {
		if !strings.Contains(sr.gotUser, hit.Text) {
			t.Errorf("combined prompt missing hit %q; got %q", hit.Text, sr.gotUser)
		}
	}
}

// filterResultsSince drops rows whose Timestamp parses to before the cutoff.
// Rows with empty/unparseable Timestamp are KEPT — they are mostly semantic
// hits that have no semantic-timestamp by design (see semanticRecall doc).
func TestFilterResultsSince_KeepsAtAndAfterCutoff(t *testing.T) {
	since, _ := time.Parse(time.RFC3339, "2026-06-20T00:00:00Z")
	in := []recallResult{
		{Source: "audit", Timestamp: "2026-06-21T10:00:00Z", Text: "after"},
		{Source: "audit", Timestamp: "2026-06-20T00:00:00Z", Text: "at"},
		{Source: "audit", Timestamp: "2026-06-19T23:59:59Z", Text: "before"},
		{Source: "contact", Timestamp: "2026-06-18T00:00:00Z", Text: "older"},
		{Source: "semantic", Timestamp: "", Text: "no-ts kept"},
		{Source: "audit", Timestamp: "not-rfc3339", Text: "garbled kept"},
	}
	out := filterResultsSince(in, since)
	got := make([]string, 0, len(out))
	for _, r := range out {
		got = append(got, r.Text)
	}
	want := []string{"after", "at", "no-ts kept", "garbled kept"}
	if len(got) != len(want) {
		t.Fatalf("len mismatch: got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("row %d: got %q want %q", i, got[i], want[i])
		}
	}
}

func TestFilterResultsSince_EmptyInputReturnsEmpty(t *testing.T) {
	since, _ := time.Parse(time.RFC3339, "2026-06-20T00:00:00Z")
	if out := filterResultsSince(nil, since); len(out) != 0 {
		t.Errorf("want empty, got %v", out)
	}
}
