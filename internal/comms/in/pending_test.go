package in

import (
	"testing"
	"time"
)

func TestPendingStore_PutThenTakeReturnsEntry(t *testing.T) {
	s := NewMemoryPendingStore()
	p := Pending{RecID: "r1", Channel: "discord", ConvID: "c1", PeerID: "u1", SentAt: time.Now()}
	if err := s.Put(p); err != nil {
		t.Fatalf("Put: %v", err)
	}
	got, ok := s.Take("discord", "c1")
	if !ok {
		t.Fatalf("Take: want ok, got miss")
	}
	if got.RecID != "r1" || got.PeerID != "u1" {
		t.Fatalf("Take: got %+v, want RecID=r1 PeerID=u1", got)
	}
}

// Take is single-use: replay defense per spec §3.2.
func TestPendingStore_TakeIsSingleUse(t *testing.T) {
	s := NewMemoryPendingStore()
	_ = s.Put(Pending{RecID: "r1", Channel: "discord", ConvID: "c1", PeerID: "u1", SentAt: time.Now()})
	if _, ok := s.Take("discord", "c1"); !ok {
		t.Fatalf("first Take: want ok")
	}
	if _, ok := s.Take("discord", "c1"); ok {
		t.Fatalf("second Take: want miss (single-use), got hit")
	}
}

func TestPendingStore_TakeUnknownKeyMisses(t *testing.T) {
	s := NewMemoryPendingStore()
	if _, ok := s.Take("discord", "nope"); ok {
		t.Fatalf("Take unknown: want miss")
	}
}

// Different (channel, convID) keys do not collide.
func TestPendingStore_KeyIsChannelPlusConvID(t *testing.T) {
	s := NewMemoryPendingStore()
	_ = s.Put(Pending{RecID: "rD", Channel: "discord", ConvID: "x", PeerID: "u", SentAt: time.Now()})
	_ = s.Put(Pending{RecID: "rW", Channel: "whatsapp", ConvID: "x", PeerID: "u", SentAt: time.Now()})
	d, _ := s.Take("discord", "x")
	w, _ := s.Take("whatsapp", "x")
	if d.RecID != "rD" || w.RecID != "rW" {
		t.Fatalf("cross-channel collision: discord=%q whatsapp=%q", d.RecID, w.RecID)
	}
}

// Put on the same key overwrites — newest prompt wins (a re-prompt replaces the stale entry).
func TestPendingStore_PutOverwritesSameKey(t *testing.T) {
	s := NewMemoryPendingStore()
	_ = s.Put(Pending{RecID: "old", Channel: "discord", ConvID: "c", PeerID: "u", SentAt: time.Now()})
	_ = s.Put(Pending{RecID: "new", Channel: "discord", ConvID: "c", PeerID: "u", SentAt: time.Now()})
	got, _ := s.Take("discord", "c")
	if got.RecID != "new" {
		t.Fatalf("overwrite: got RecID=%q, want new", got.RecID)
	}
}
