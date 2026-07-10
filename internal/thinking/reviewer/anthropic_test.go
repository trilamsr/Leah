package reviewer

import (
	"bytes"
	"errors"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

// fakeStream feeds canned MessageStreamEventUnion events and records Close().
type fakeStream struct {
	events []anthropic.MessageStreamEventUnion
	idx    int
	err    error
	closed bool
}

func (f *fakeStream) Next() bool {
	if f.err != nil {
		return false
	}
	if f.idx >= len(f.events) {
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
	return nil
}

// eventStart builds a MessageStartEvent union with the given usage fields.
func eventStart(input, cacheCreate, cacheRead, output int64) anthropic.MessageStreamEventUnion {
	raw := `{"type":"message_start","message":{"id":"msg_x","type":"message","role":"assistant","model":"x","content":[],"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":` +
		itoa(input) + `,"cache_creation_input_tokens":` + itoa(cacheCreate) +
		`,"cache_read_input_tokens":` + itoa(cacheRead) +
		`,"output_tokens":` + itoa(output) + `}}}`
	var u anthropic.MessageStreamEventUnion
	if err := u.UnmarshalJSON([]byte(raw)); err != nil {
		panic(err)
	}
	return u
}

// eventContentBlockStart opens content block index 0 as a text block —
// required by Message.Accumulate before any ContentBlockDelta.
func eventContentBlockStart() anthropic.MessageStreamEventUnion {
	raw := `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":"","citations":null}}`
	var u anthropic.MessageStreamEventUnion
	if err := u.UnmarshalJSON([]byte(raw)); err != nil {
		panic(err)
	}
	return u
}

// eventTextDelta builds a content_block_delta event carrying text.
func eventTextDelta(text string) anthropic.MessageStreamEventUnion {
	raw := `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":` + quote(text) + `}}`
	var u anthropic.MessageStreamEventUnion
	if err := u.UnmarshalJSON([]byte(raw)); err != nil {
		panic(err)
	}
	return u
}

// eventMessageDelta builds a message_delta with output_tokens only (cache
// fields absent — the failure mode being tested).
func eventMessageDelta(output int64) anthropic.MessageStreamEventUnion {
	raw := `{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":` + itoa(output) + `}}`
	var u anthropic.MessageStreamEventUnion
	if err := u.UnmarshalJSON([]byte(raw)); err != nil {
		panic(err)
	}
	return u
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func quote(s string) string { return `"` + s + `"` }

// TestProcessStream_ClosesStream guards finding #1: the SDK Stream owns a
// reader that must be closed even when iteration completes normally.
func TestProcessStream_ClosesStream(t *testing.T) {
	fs := &fakeStream{events: []anthropic.MessageStreamEventUnion{
		eventStart(100, 50, 200, 1),
		eventContentBlockStart(),
		eventTextDelta("hi"),
		eventMessageDelta(7),
	}}
	if _, _, err := processStream(fs, nil); err != nil {
		t.Fatalf("processStream: %v", err)
	}
	if !fs.closed {
		t.Errorf("stream not closed — leak")
	}
}

// TestProcessStream_ClosesStreamOnError guards finding #1: Close must still
// fire when the stream surfaces an error mid-flight.
func TestProcessStream_ClosesStreamOnError(t *testing.T) {
	fs := &fakeStream{
		events: []anthropic.MessageStreamEventUnion{eventStart(1, 0, 0, 0)},
		err:    errors.New("boom"),
	}
	_, _, _ = processStream(fs, nil)
	if !fs.closed {
		t.Errorf("stream not closed on error — leak")
	}
}

// TestProcessStream_PreservesCacheFieldsAcrossDelta guards finding #2: a
// MessageDeltaEvent that omits CacheCreationInputTokens / CacheReadInputTokens
// must NOT zero out the cache counters MessageStart populated.
func TestProcessStream_PreservesCacheFieldsAcrossDelta(t *testing.T) {
	fs := &fakeStream{events: []anthropic.MessageStreamEventUnion{
		eventStart(100, 50, 200, 1),
		eventContentBlockStart(),
		eventTextDelta("ok"),
		eventMessageDelta(42),
	}}
	_, msg, err := processStream(fs, nil)
	if err != nil {
		t.Fatalf("processStream: %v", err)
	}
	if msg.Usage.CacheCreationInputTokens != 50 {
		t.Errorf("CacheCreationInputTokens lost: got %d want 50", msg.Usage.CacheCreationInputTokens)
	}
	if msg.Usage.CacheReadInputTokens != 200 {
		t.Errorf("CacheReadInputTokens lost: got %d want 200", msg.Usage.CacheReadInputTokens)
	}
	if msg.Usage.InputTokens != 100 {
		t.Errorf("InputTokens: got %d want 100", msg.Usage.InputTokens)
	}
	if msg.Usage.OutputTokens != 42 {
		t.Errorf("OutputTokens: got %d want 42 (delta should overwrite cumulative)", msg.Usage.OutputTokens)
	}
}

// TestProcessStream_StreamsTextToSink asserts each text delta reaches sink in
// order — the user-visible reason streaming exists.
func TestProcessStream_StreamsTextToSink(t *testing.T) {
	sink := &bytes.Buffer{}
	fs := &fakeStream{events: []anthropic.MessageStreamEventUnion{
		eventStart(1, 0, 0, 0),
		eventContentBlockStart(),
		eventTextDelta("hello "),
		eventTextDelta("world"),
	}}
	text, _, err := processStream(fs, sink)
	if err != nil {
		t.Fatalf("processStream: %v", err)
	}
	if text != "hello world" {
		t.Errorf("text: %q", text)
	}
	if sink.String() != "hello world" {
		t.Errorf("sink: %q", sink.String())
	}
}
