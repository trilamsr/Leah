package inbound

import (
	"context"
	"errors"
	"testing"
)

// TestRegexClassifier_FastPath exercises the spec §5 vocabulary table. The
// regex tier must cover the one-word reply majority with zero LLM cost.
func TestRegexClassifier_FastPath(t *testing.T) {
	cases := []struct {
		in   string
		want Intent
	}{
		{"y", IntentAccept},
		{"yes", IntentAccept},
		{"YES", IntentAccept},
		{"ok", IntentAccept},
		{"okay", IntentAccept},
		{"approve", IntentAccept},
		{"do it", IntentAccept},
		{"go", IntentAccept},
		{"ship it", IntentAccept},
		{"👍", IntentAccept},
		{"✅", IntentAccept},
		{"yes please", IntentAccept},

		{"n", IntentReject},
		{"no", IntentReject},
		{"nope", IntentReject},
		{"reject", IntentReject},
		{"stop", IntentReject},
		{"don't", IntentReject},
		{"dont", IntentReject},
		{"skip", IntentReject},
		{"👎", IntentReject},
		{"❌", IntentReject},
		{"no thanks", IntentReject},

		{"later", IntentDefer},
		{"snooze", IntentDefer},
		{"not now", IntentDefer},
		{"defer", IntentDefer},
		{"tomorrow", IntentDefer},

		{"  yes  ", IntentAccept},
	}
	c := NewRegexClassifier()
	for _, tc := range cases {
		got, err := c.Classify(context.Background(), tc.in)
		if err != nil {
			t.Fatalf("Classify(%q) err: %v", tc.in, err)
		}
		if got != tc.want {
			t.Errorf("Classify(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

// TestRegexClassifier_NoFallbackReturnsUnknown — without an LLM seam, gibberish
// is IntentUnknown. The router treats Unknown as no-op (spec §5 fail-safe).
func TestRegexClassifier_NoFallbackReturnsUnknown(t *testing.T) {
	c := NewRegexClassifier()
	cases := []string{"hmm", "what?", "kjasdfh", "maybe", ""}
	for _, in := range cases {
		got, err := c.Classify(context.Background(), in)
		if err != nil {
			t.Fatalf("Classify(%q) err: %v", in, err)
		}
		if got != IntentUnknown {
			t.Errorf("Classify(%q) = %v, want IntentUnknown", in, got)
		}
	}
}

type stubReasoner struct {
	reply string
	err   error
	calls int
	last  string
}

func (s *stubReasoner) Ask(_ context.Context, prompt string) (string, error) {
	s.calls++
	s.last = prompt
	return s.reply, s.err
}

// TestFallbackClassifier_RegexShortCircuits — a regex hit must NEVER call the
// LLM. The LLM tier is a cost surface; spec §5 mandates fast-path-first.
func TestFallbackClassifier_RegexShortCircuits(t *testing.T) {
	r := &stubReasoner{reply: "accept"}
	c := NewFallbackClassifier(r)
	got, err := c.Classify(context.Background(), "yes")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != IntentAccept {
		t.Errorf("got %v want IntentAccept", got)
	}
	if r.calls != 0 {
		t.Errorf("reasoner called %d times on a regex hit; want 0", r.calls)
	}
}

// TestFallbackClassifier_LLMResolvesAmbiguous — ambiguous text falls through
// to the LLM seam, which returns one of the four labels.
func TestFallbackClassifier_LLMResolvesAmbiguous(t *testing.T) {
	cases := []struct {
		reply string
		want  Intent
	}{
		{"accept", IntentAccept},
		{"  ACCEPT  ", IntentAccept},
		{"reject", IntentReject},
		{"defer", IntentDefer},
		{"unknown", IntentUnknown},
	}
	for _, tc := range cases {
		r := &stubReasoner{reply: tc.reply}
		c := NewFallbackClassifier(r)
		got, err := c.Classify(context.Background(), "let me think about it")
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if got != tc.want {
			t.Errorf("reply=%q got %v want %v", tc.reply, got, tc.want)
		}
		if r.calls != 1 {
			t.Errorf("reasoner calls=%d want 1", r.calls)
		}
	}
}

// TestFallbackClassifier_LLMGarbageIsUnknown — any label outside the four is
// treated as IntentUnknown (spec §5: "default unknown on any ambiguity").
func TestFallbackClassifier_LLMGarbageIsUnknown(t *testing.T) {
	r := &stubReasoner{reply: "maybe later if you want"}
	c := NewFallbackClassifier(r)
	got, err := c.Classify(context.Background(), "hmm")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != IntentUnknown {
		t.Errorf("got %v want IntentUnknown", got)
	}
}

// TestFallbackClassifier_LLMErrorIsUnknown — a reasoner transport failure
// must NEVER escalate to an accept; fail-safe to Unknown so the router no-ops.
func TestFallbackClassifier_LLMErrorIsUnknown(t *testing.T) {
	r := &stubReasoner{err: errors.New("transport down")}
	c := NewFallbackClassifier(r)
	got, err := c.Classify(context.Background(), "hmm")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != IntentUnknown {
		t.Errorf("got %v want IntentUnknown on reasoner error", got)
	}
}

// TestFallbackClassifier_NilReasonerFallsBackToRegex — a nil reasoner is the
// daemon's "LLM not wired yet" state. Must degrade gracefully to regex-only
// (no panic, ambiguous -> Unknown).
func TestFallbackClassifier_NilReasonerFallsBackToRegex(t *testing.T) {
	c := NewFallbackClassifier(nil)
	got, err := c.Classify(context.Background(), "yes")
	if err != nil || got != IntentAccept {
		t.Fatalf("regex hit broken: got=%v err=%v", got, err)
	}
	got, err = c.Classify(context.Background(), "hmm")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != IntentUnknown {
		t.Errorf("nil reasoner ambiguous: got %v want IntentUnknown", got)
	}
}

// TestClassifier_RejectAndDeferNeverAuthorize is the load-bearing safety
// assertion: reject/defer intents emitted by the classifier never reach
// ConsentGate.Authorize. This pins the gate-only-on-act invariant (spec §4.3)
// at the classifier+router seam.
func TestClassifier_RejectAndDeferNeverAuthorize(t *testing.T) {
	store := NewMemoryPendingStore()
	_ = store.Put(Pending{RecID: "rec-1", Channel: "discord", ConvID: "c1", PeerID: "p1"})
	_ = store.Put(Pending{RecID: "rec-2", Channel: "discord", ConvID: "c2", PeerID: "p1"})
	gate := &fakeGate{enrolled: map[string]bool{"discord|p1": true}}
	eng := &fakeEngine{}
	r := &Router{
		Pending:  store,
		Consent:  gate,
		Classify: NewRegexClassifier(),
		Engine:   eng,
	}
	if err := r.Handle(context.Background(), Reply{Channel: "discord", ConvID: "c1", PeerID: "p1", Text: "no"}); err != nil {
		t.Fatalf("reject path err: %v", err)
	}
	if err := r.Handle(context.Background(), Reply{Channel: "discord", ConvID: "c2", PeerID: "p1", Text: "later"}); err != nil {
		t.Fatalf("defer path err: %v", err)
	}
	if gate.authorizeCalls != 0 {
		t.Errorf("Authorize called %d times; reject/defer must skip the gate", gate.authorizeCalls)
	}
	if eng.applyCalls != 0 {
		t.Errorf("Engine.Apply called %d times; reject/defer must not apply", eng.applyCalls)
	}
}
