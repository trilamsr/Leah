package inbound

import (
	"context"
	"testing"
	"time"
)

func newStore(t *testing.T) *TokenStore {
	t.Helper()
	return NewTokenStore(InMemory())
}

func TestIssueToken_PlainShownOnce_HashPersisted(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	tok, err := s.Issue(ctx, "ci-bot", []Scope{ScopeMemoryRead})
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if tok.Plain == "" {
		t.Fatal("issue must return plain token once")
	}
	got, err := s.Verify(ctx, tok.Plain)
	if err != nil {
		t.Fatalf("Verify(fresh plain): %v", err)
	}
	if got.ID != tok.ID {
		t.Fatalf("Verify returned id=%d, want %d", got.ID, tok.ID)
	}
	listed, err := s.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, r := range listed {
		if r.Plain != "" {
			t.Fatalf("List leaked plain token for id=%d", r.ID)
		}
	}
}

func TestVerify_Mismatch_Returns401(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	if _, err := s.Issue(ctx, "n1", []Scope{ScopeMemoryRead}); err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if _, err := s.Verify(ctx, "nope-wrong-secret"); err != ErrUnauthorized {
		t.Fatalf("Verify(bad) err=%v, want ErrUnauthorized", err)
	}
}

func TestRateLimit_FiresAfterFiveFailsInOneMinute(t *testing.T) {
	clock := newFakeClock(time.Unix(1700000000, 0))
	s := NewTokenStore(InMemory(), WithClock(clock.Now))
	ctx := context.Background()
	if _, err := s.Issue(ctx, "n1", []Scope{ScopeMemoryRead}); err != nil {
		t.Fatalf("Issue: %v", err)
	}
	for i := 0; i < 5; i++ {
		if err := s.RecordFailure(ctx, "127.0.0.1"); err != nil {
			t.Fatalf("RecordFailure[%d]: %v", i, err)
		}
	}
	if err := s.RecordFailure(ctx, "127.0.0.1"); err != ErrRateLimited {
		t.Fatalf("6th failure: err=%v, want ErrRateLimited", err)
	}
	clock.Advance(61 * time.Second)
	if err := s.RecordFailure(ctx, "127.0.0.1"); err == ErrRateLimited {
		t.Fatalf("fresh window should accept failure, got rate-limit")
	}
}

func TestVerify_RevokedToken_FailsAuthorized(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	tok, err := s.Issue(ctx, "n1", []Scope{ScopeMemoryRead})
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if err := s.Revoke(ctx, tok.ID); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if _, err := s.Verify(ctx, tok.Plain); err != ErrUnauthorized {
		t.Fatalf("Verify(revoked) err=%v, want ErrUnauthorized", err)
	}
}

func TestVerify_SameLengthMismatch_StillRejected(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	tok, err := s.Issue(ctx, "n1", []Scope{ScopeMemoryRead})
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	bad := tok.Plain[:len(tok.Plain)-1] + flip(tok.Plain[len(tok.Plain)-1])
	if _, err := s.Verify(ctx, bad); err != ErrUnauthorized {
		t.Fatalf("Verify(same-len-wrong) err=%v, want ErrUnauthorized", err)
	}
}

func flip(b byte) string {
	if b == 'a' {
		return "b"
	}
	return "a"
}

type fakeClock struct{ now time.Time }

func newFakeClock(t time.Time) *fakeClock { return &fakeClock{now: t} }
func (c *fakeClock) Now() time.Time       { return c.now }
func (c *fakeClock) Advance(d time.Duration) {
	c.now = c.now.Add(d)
}
