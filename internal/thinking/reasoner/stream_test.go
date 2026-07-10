package reasoner

import (
	"context"
	"testing"
	"time"

	"github.com/trilam/leah/internal/budget"
)

// fakeStreamClient drives AskStream through a scripted Delta channel.
type fakeStreamClient struct {
	fakeClient
	deltas []Delta
	err    error
}

func (f *fakeStreamClient) Stream(ctx context.Context, system, user string) (<-chan Delta, error) {
	if f.err != nil {
		return nil, f.err
	}
	out := make(chan Delta, len(f.deltas))
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

func TestAskStream_EmitsTextDeltas(t *testing.T) {
	c := &fakeStreamClient{
		deltas: []Delta{
			{Text: "Hello "},
			{Text: "world."},
			{Final: true, InputTok: 5, OutputTok: 3},
		},
	}
	r := &Reasoner{Client: c, Budget: &budget.Budget{Ceiling: 1.0}, SystemPrompt: "x"}

	ch, err := r.AskStream(context.Background(), "say hi")
	if err != nil {
		t.Fatalf("askstream: %v", err)
	}
	var got []string
	for s := range ch {
		got = append(got, s)
	}
	if len(got) != 2 || got[0] != "Hello " || got[1] != "world." {
		t.Errorf("got %q, want [Hello , world.]", got)
	}
}

func TestAskStream_DiscriminatesToolUseDeltas(t *testing.T) {
	c := &fakeStreamClient{
		deltas: []Delta{
			{Text: "let me check. "},
			{ToolUse: &ToolUseEvent{Name: "search"}}, // must NOT appear on text channel
			{Text: "done."},
			{Final: true},
		},
	}
	r := &Reasoner{Client: c, Budget: &budget.Budget{Ceiling: 1.0}, SystemPrompt: "x"}

	ch, err := r.AskStream(context.Background(), "search the web")
	if err != nil {
		t.Fatalf("askstream: %v", err)
	}
	var got []string
	for s := range ch {
		got = append(got, s)
	}
	// Tool-use delta is suppressed from text stream; only text deltas surface.
	if len(got) != 2 || got[0] != "let me check. " || got[1] != "done." {
		t.Errorf("got %q, want [let me check. , done.]", got)
	}
}

// TestAskStream_CancelClosesChannel asserts ctx-cancel propagates: the
// channel must close so consumers don't deadlock.
func TestAskStream_CancelClosesChannel(t *testing.T) {
	blocker := make(chan struct{})
	c := &blockingStreamClient{block: blocker}
	r := &Reasoner{Client: c, Budget: &budget.Budget{Ceiling: 1.0}, SystemPrompt: "x"}

	ctx, cancel := context.WithCancel(context.Background())
	ch, err := r.AskStream(ctx, "x")
	if err != nil {
		t.Fatalf("askstream: %v", err)
	}
	cancel()
	close(blocker)
	select {
	case _, ok := <-ch:
		if ok {
			// drain
			for range ch {
			}
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("channel did not close after cancel")
	}
}

type blockingStreamClient struct {
	fakeClient
	block chan struct{}
}

func (b *blockingStreamClient) Stream(ctx context.Context, system, user string) (<-chan Delta, error) {
	out := make(chan Delta)
	go func() {
		defer close(out)
		select {
		case <-ctx.Done():
			return
		case <-b.block:
			return
		}
	}()
	return out, nil
}
