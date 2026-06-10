package session

import "testing"

// TestChunker_EmitsAtSentenceBoundary covers the [.!?]\s + MinChars contract.
func TestChunker_EmitsAtSentenceBoundary(t *testing.T) {
	c := &SentenceChunker{}
	if chunk, ok := c.Push("Hello "); ok || chunk != "" {
		t.Errorf("no-terminator push must not emit; got %q,%v", chunk, ok)
	}
	// "Hello world." → 11 speakable chars, just below the 12-char floor.
	if chunk, ok := c.Push("world. "); ok {
		t.Errorf("just-below-floor must not emit; got %q", chunk)
	}
	chunk, ok := c.Push("Welcome. ")
	if !ok || chunk != "Hello world. Welcome. " {
		t.Errorf("expected boundary emit; got %q,%v", chunk, ok)
	}
}

// TestChunker_MinCharsSuppressesShort: "OK. " is below MinChars=12 floor.
func TestChunker_MinCharsSuppressesShort(t *testing.T) {
	c := &SentenceChunker{}
	if chunk, ok := c.Push("OK. "); ok {
		t.Errorf("short utterance should not flush; got %q", chunk)
	}
	// Continuation that pushes past the floor must then emit.
	chunk, ok := c.Push("Then more text here. ")
	if !ok || chunk == "" {
		t.Errorf("post-floor continuation should emit; got %q,%v", chunk, ok)
	}
}

// TestChunker_FlushReturnsTail surfaces buffered fragment on stream end.
func TestChunker_FlushReturnsTail(t *testing.T) {
	c := &SentenceChunker{}
	c.Push("Tail without terminator")
	if got := c.Flush(); got != "Tail without terminator" {
		t.Errorf("flush: got %q", got)
	}
	if got := c.Flush(); got != "" {
		t.Errorf("flush after reset must be empty; got %q", got)
	}
}

// TestChunker_MultipleSentencesInOneDelta emits the last boundary and
// preserves the trailing fragment in the buffer.
func TestChunker_MultipleSentencesInOneDelta(t *testing.T) {
	c := &SentenceChunker{}
	chunk, ok := c.Push("One sentence here. Two more words. trail")
	if !ok || chunk != "One sentence here. Two more words. " {
		t.Errorf("multi-sentence emit; got %q,%v", chunk, ok)
	}
	if got := c.Flush(); got != "trail" {
		t.Errorf("residual tail; got %q", got)
	}
}
