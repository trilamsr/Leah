package reasoner

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/trilam/leah/internal/budget"
)

// streamingFakeClient drives StreamingClient: emits each chunk in order to
// the callback before returning the joined text.
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

// fakeStream implements anthropicStreamer; records Close() invocations so
// the defer in consumeStream can be verified.
type fakeStream struct {
	events    []anthropic.MessageStreamEventUnion
	idx       int
	err       error
	closed    bool
	closeHits int
}

func (f *fakeStream) Next() bool {
	if f.err != nil || f.idx >= len(f.events) {
		return false
	}
	f.idx++
	return true
}

func (f *fakeStream) Current() anthropic.MessageStreamEventUnion {
	return f.events[f.idx-1]
}

func (f *fakeStream) Err() error { return f.err }

func (f *fakeStream) Close() error {
	f.closed = true
	f.closeHits++
	return nil
}

// TestConsumeStreamClosesStream asserts consumeStream invokes Close exactly
// once on the underlying stream — guards reviewer #1: NewStreaming was
// never followed by Close, leaking the SSE response body.
func TestConsumeStreamClosesStream(t *testing.T) {
	s := &fakeStream{}
	_, err := consumeStream(s, nil)
	if err != nil {
		t.Fatalf("consumeStream: %v", err)
	}
	if !s.closed {
		t.Fatal("stream.Close() not called — would leak SSE body")
	}
	if s.closeHits != 1 {
		t.Errorf("Close called %d times, want 1", s.closeHits)
	}
}

// TestConsumeStreamClosesOnError still calls Close when stream.Err returns
// non-nil — defer fires regardless of the early-return path.
func TestConsumeStreamClosesOnError(t *testing.T) {
	s := &fakeStream{err: errors.New("network blew up")}
	_, err := consumeStream(s, nil)
	if err == nil {
		t.Fatal("expected error from consumeStream")
	}
	if !s.closed {
		t.Fatal("stream.Close() not called on error path")
	}
}

// TestAccumulatePreservesCacheUsage asserts CacheCreationInputTokens and
// CacheReadInputTokens populated by message_start survive a message_delta
// that only carries OutputTokens. Guards reviewer #2: SDK's per-field
// accumulation must (and does) keep cache fields. A future regression that
// switched to `usage = event.Usage` would fail this test.
func TestAccumulatePreservesCacheUsage(t *testing.T) {
	startRaw := []byte(`{
		"type": "message_start",
		"message": {
			"id": "msg_01",
			"type": "message",
			"role": "assistant",
			"model": "claude-sonnet-4-6",
			"content": [],
			"stop_reason": null,
			"stop_sequence": null,
			"usage": {
				"input_tokens": 1200,
				"cache_creation_input_tokens": 1000,
				"cache_read_input_tokens": 200,
				"output_tokens": 1
			}
		}
	}`)
	deltaRaw := []byte(`{
		"type": "message_delta",
		"delta": {"stop_reason": "end_turn", "stop_sequence": null},
		"usage": {"output_tokens": 42}
	}`)
	var startEv, deltaEv anthropic.MessageStreamEventUnion
	if err := json.Unmarshal(startRaw, &startEv); err != nil {
		t.Fatalf("unmarshal start: %v", err)
	}
	if err := json.Unmarshal(deltaRaw, &deltaEv); err != nil {
		t.Fatalf("unmarshal delta: %v", err)
	}
	s := &fakeStream{events: []anthropic.MessageStreamEventUnion{startEv, deltaEv}}
	msg, err := consumeStream(s, nil)
	if err != nil {
		t.Fatalf("consumeStream: %v", err)
	}
	if msg.Usage.CacheCreationInputTokens != 1000 {
		t.Errorf("CacheCreationInputTokens dropped: got %d want 1000", msg.Usage.CacheCreationInputTokens)
	}
	if msg.Usage.CacheReadInputTokens != 200 {
		t.Errorf("CacheReadInputTokens dropped: got %d want 200", msg.Usage.CacheReadInputTokens)
	}
	if msg.Usage.OutputTokens != 42 {
		t.Errorf("OutputTokens: got %d want 42", msg.Usage.OutputTokens)
	}
}

// TestChunkWriterTrailingNewline asserts the chunk writer used by runAsk
// emits exactly one trailing newline regardless of whether the last chunk
// ended with \n. Guards reviewer #6: piped stdout swallows the response
// without a final newline.
func TestChunkWriterTrailingNewline(t *testing.T) {
	cases := []struct {
		name   string
		chunks []string
		want   string
	}{
		{"no trailing nl", []string{"hello", " world"}, "hello world\n"},
		{"already trailing nl", []string{"hello\n"}, "hello\n"},
		{"empty stream", nil, ""},
		{"multi chunk mixed", []string{"a", "b\n", "c"}, "ab\nc\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf strings.Builder
			w := NewChunkWriter(&buf)
			for _, c := range tc.chunks {
				if _, err := w.WriteChunk(c); err != nil {
					t.Fatalf("write: %v", err)
				}
			}
			if err := w.Finish(); err != nil {
				t.Fatalf("finish: %v", err)
			}
			if got := buf.String(); got != tc.want {
				t.Errorf("got %q want %q", got, tc.want)
			}
		})
	}
}

// flushTracker counts Sync() calls — chunkWriter flushes per chunk when
// the underlying writer is Sync-capable (e.g. *os.File). Without per-chunk
// flush, piped stdout buffers tokens past the operator's terminal until
// the process exits.
type flushTracker struct {
	strings.Builder
	flushes int
}

func (f *flushTracker) Sync() error {
	f.flushes++
	return nil
}

func TestChunkWriterFlushesPerChunk(t *testing.T) {
	var ft flushTracker
	w := NewChunkWriter(&ft)
	chunks := []string{"a", "b", "c"}
	for _, c := range chunks {
		if _, err := w.WriteChunk(c); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	if ft.flushes != len(chunks) {
		t.Errorf("flushes per chunk: got %d want %d", ft.flushes, len(chunks))
	}
}

// TestChunkWriterFinishEmptyStreamNoNewline guards against printing a bare
// "\n" when the stream produced zero bytes — runAsk treats the empty case
// elsewhere via the `streamed` flag, so the writer itself must stay silent.
func TestChunkWriterFinishEmptyStreamNoNewline(t *testing.T) {
	var buf strings.Builder
	w := NewChunkWriter(&buf)
	if err := w.Finish(); err != nil {
		t.Fatalf("finish: %v", err)
	}
	if got := buf.String(); got != "" {
		t.Errorf("empty stream wrote %q", got)
	}
}
