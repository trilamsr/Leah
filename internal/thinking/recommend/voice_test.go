package recommend_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/trilam/leah/internal/thinking/recommend"
)

// fakeTTS captures spoken text so tests can assert what reached the chain.
type fakeTTS struct {
	mu     sync.Mutex
	spoken []string
	err    error
}

func (f *fakeTTS) Speak(_ context.Context, text string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	f.spoken = append(f.spoken, text)
	return nil
}

func (f *fakeTTS) all() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.spoken))
	copy(out, f.spoken)
	return out
}

// fakeAttestor denies when deny is set; otherwise records the scope it saw.
type fakeAttestor struct {
	deny  bool
	scope string
}

func (a *fakeAttestor) Attest(_ context.Context, scope string) error {
	a.scope = scope
	if a.deny {
		return errors.New("denied")
	}
	return nil
}

func rec(id, pattern string, tier recommend.Tier, conf float64) recommend.Recommendation {
	return recommend.Recommendation{
		ID:         id,
		Pattern:    pattern,
		Tier:       tier,
		Confidence: conf,
		CreatedAt:  time.Unix(0, 0).UTC(),
	}
}

func TestAnnounce_Top3_Spoken(t *testing.T) {
	tts := &fakeTTS{}
	att := &fakeAttestor{}
	a := recommend.NewVoiceAnnouncer(tts, att)

	// Five recs out of order; expect top 3 by confidence, all auto/confirm.
	recs := []recommend.Recommendation{
		rec("a", "lint-on-save", recommend.TierAuto, 0.55),
		rec("b", "morning-brief", recommend.TierConfirm, 0.92),
		rec("c", "commit-at-focus-end", recommend.TierAuto, 0.81),
		rec("d", "fmt-on-save", recommend.TierAuto, 0.70),
		rec("e", "deploy", recommend.TierConfirm, 0.40),
	}

	if err := a.Announce(context.Background(), recs); err != nil {
		t.Fatalf("Announce: %v", err)
	}

	got := tts.all()
	if len(got) != 1 {
		t.Fatalf("want 1 Speak call (single utterance), got %d: %#v", len(got), got)
	}
	utter := got[0]
	// Top 3 by confidence: b (0.92), c (0.81), d (0.70).
	for _, want := range []string{"morning-brief", "commit-at-focus-end", "fmt-on-save"} {
		if !strings.Contains(utter, want) {
			t.Errorf("utterance missing %q: %q", want, utter)
		}
	}
	for _, omitted := range []string{"lint-on-save", "deploy"} {
		if strings.Contains(utter, omitted) {
			t.Errorf("utterance unexpectedly included %q: %q", omitted, utter)
		}
	}
	if att.scope != "recommend:announce" {
		t.Errorf("attestor scope = %q, want recommend:announce", att.scope)
	}
}

func TestAnnounce_AttestationDenied_NoSpeak(t *testing.T) {
	tts := &fakeTTS{}
	att := &fakeAttestor{deny: true}
	a := recommend.NewVoiceAnnouncer(tts, att)

	err := a.Announce(context.Background(), []recommend.Recommendation{
		rec("a", "p", recommend.TierAuto, 0.9),
	})
	if err == nil {
		t.Fatalf("Announce: want error on denied attestation, got nil")
	}
	if len(tts.all()) != 0 {
		t.Fatalf("denied attestation must not speak; got %#v", tts.all())
	}
}

func TestAnnounce_EmptyRecs_NoSpeak(t *testing.T) {
	tts := &fakeTTS{}
	att := &fakeAttestor{}
	a := recommend.NewVoiceAnnouncer(tts, att)

	if err := a.Announce(context.Background(), nil); err != nil {
		t.Fatalf("Announce(nil): %v", err)
	}
	if err := a.Announce(context.Background(), []recommend.Recommendation{}); err != nil {
		t.Fatalf("Announce([]): %v", err)
	}
	if len(tts.all()) != 0 {
		t.Fatalf("empty recs must not speak; got %#v", tts.all())
	}
	// Empty input is a graceful skip — attestation MUST NOT be consumed
	// since no privileged action occurs.
	if att.scope != "" {
		t.Errorf("empty recs must not call Attest; scope = %q", att.scope)
	}
}

func TestAnnounce_BlockedTier_Omitted(t *testing.T) {
	tts := &fakeTTS{}
	att := &fakeAttestor{}
	a := recommend.NewVoiceAnnouncer(tts, att)

	recs := []recommend.Recommendation{
		rec("a", "secret-rotate", recommend.TierBlocked, 0.99),
		rec("b", "silent-archive", recommend.TierSilent, 0.95),
		rec("c", "fmt-on-save", recommend.TierAuto, 0.60),
	}
	if err := a.Announce(context.Background(), recs); err != nil {
		t.Fatalf("Announce: %v", err)
	}

	got := tts.all()
	if len(got) != 1 {
		t.Fatalf("want 1 Speak call, got %d", len(got))
	}
	utter := got[0]
	if !strings.Contains(utter, "fmt-on-save") {
		t.Errorf("utterance missing surviving auto-tier rec: %q", utter)
	}
	for _, omitted := range []string{"secret-rotate", "silent-archive"} {
		if strings.Contains(utter, omitted) {
			t.Errorf("utterance includes filtered pattern %q: %q", omitted, utter)
		}
	}
}
