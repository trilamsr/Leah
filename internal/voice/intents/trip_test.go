package intents

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
)

type fakePlanner struct {
	onTheWayPlaces  []Place
	onTheWayErr     error
	midPlaces       []Place
	midErr          error
	lastOrigin      string
	lastDest        string
	lastOpts        CorridorOpts
	lastCategory    string
	lastMidPartyA   string
	lastMidPartyB   string
	onTheWayCalls   int
	meetMiddleCalls int
}

func (f *fakePlanner) OnTheWay(_ context.Context, origin, dest string, opts CorridorOpts) ([]Place, error) {
	f.onTheWayCalls++
	f.lastOrigin, f.lastDest, f.lastOpts = origin, dest, opts
	return f.onTheWayPlaces, f.onTheWayErr
}

func (f *fakePlanner) MeetInMiddle(_ context.Context, partyA, partyB, category string) ([]Place, error) {
	f.meetMiddleCalls++
	f.lastMidPartyA, f.lastMidPartyB, f.lastCategory = partyA, partyB, category
	return f.midPlaces, f.midErr
}

type fakeTTS struct {
	mu       sync.Mutex
	uttered  []string
	speakErr error
}

func (t *fakeTTS) Speak(_ context.Context, text string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.uttered = append(t.uttered, text)
	return t.speakErr
}

func (t *fakeTTS) last() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.uttered) == 0 {
		return ""
	}
	return t.uttered[len(t.uttered)-1]
}

func TestRecognize_OnTheWay_HappyPath(t *testing.T) {
	r := NewRegexRouter()
	intent, ok, err := r.Recognize(context.Background(), "what's on my drive to Bellevue")
	if err != nil || !ok {
		t.Fatalf("expected recognition, got ok=%v err=%v", ok, err)
	}
	if intent.Kind != IntentOnTheWay {
		t.Fatalf("expected IntentOnTheWay, got %v", intent.Kind)
	}
	if intent.Dest != "Bellevue" {
		t.Fatalf("expected Dest=Bellevue, got %q", intent.Dest)
	}
}

func TestRecognize_MeetInMiddle(t *testing.T) {
	r := NewRegexRouter()
	intent, ok, err := r.Recognize(context.Background(), "find coffee midway between me and Sarah")
	if err != nil || !ok {
		t.Fatalf("expected recognition, got ok=%v err=%v", ok, err)
	}
	if intent.Kind != IntentMeetInMiddle {
		t.Fatalf("expected IntentMeetInMiddle, got %v", intent.Kind)
	}
	if intent.Dest != "Sarah" {
		t.Fatalf("expected Dest=Sarah, got %q", intent.Dest)
	}
	if len(intent.Categories) != 1 || intent.Categories[0] != "coffee" {
		t.Fatalf("expected Categories=[coffee], got %v", intent.Categories)
	}
}

func TestRecognize_Unknown_ReturnsFalse(t *testing.T) {
	r := NewRegexRouter()
	intent, ok, err := r.Recognize(context.Background(), "what's the weather")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatalf("expected ok=false, got intent=%+v", intent)
	}
}

func TestHandler_Dispatch_OnTheWay_AnnouncesTopK(t *testing.T) {
	planner := &fakePlanner{onTheWayPlaces: []Place{
		{Name: "Snoqualmie Falls"},
		{Name: "Issaquah Coffee"},
		{Name: "Bellevue Square"},
		{Name: "Should-Not-Speak"},
	}}
	tts := &fakeTTS{}
	h := &Handler{Planner: planner, Announcer: tts, DefaultOrigin: "Seattle"}

	err := h.Dispatch(context.Background(), TripIntent{Kind: IntentOnTheWay, Dest: "Bellevue"})
	if err != nil {
		t.Fatalf("Dispatch err: %v", err)
	}
	if planner.onTheWayCalls != 1 {
		t.Fatalf("expected 1 planner call, got %d", planner.onTheWayCalls)
	}
	if planner.lastOrigin != "Seattle" || planner.lastDest != "Bellevue" {
		t.Fatalf("planner got origin=%q dest=%q", planner.lastOrigin, planner.lastDest)
	}
	uttered := tts.last()
	for _, want := range []string{"Snoqualmie Falls", "Issaquah Coffee", "Bellevue Square"} {
		if !strings.Contains(uttered, want) {
			t.Fatalf("expected utterance to contain %q, got %q", want, uttered)
		}
	}
	if strings.Contains(uttered, "Should-Not-Speak") {
		t.Fatalf("4th place leaked into utterance: %q", uttered)
	}
}

func TestHandler_Dispatch_NoResults_AnnouncesEmpty(t *testing.T) {
	planner := &fakePlanner{onTheWayPlaces: nil}
	tts := &fakeTTS{}
	h := &Handler{Planner: planner, Announcer: tts}
	if err := h.Dispatch(context.Background(), TripIntent{Kind: IntentOnTheWay, Dest: "Bellevue"}); err != nil {
		t.Fatalf("Dispatch err: %v", err)
	}
	if !strings.Contains(strings.ToLower(tts.last()), "nothing") {
		t.Fatalf("expected empty-results announcement, got %q", tts.last())
	}
}

func TestHandler_PlannerError_AnnouncesError(t *testing.T) {
	bad := errors.New("boom")
	planner := &fakePlanner{onTheWayErr: bad}
	tts := &fakeTTS{}
	h := &Handler{Planner: planner, Announcer: tts}
	err := h.Dispatch(context.Background(), TripIntent{Kind: IntentOnTheWay, Dest: "Bellevue"})
	if !errors.Is(err, bad) {
		t.Fatalf("expected planner error returned, got %v", err)
	}
	if !strings.Contains(strings.ToLower(tts.last()), "error") {
		t.Fatalf("expected error announcement, got %q", tts.last())
	}
}

func TestHandler_NoLatLngSpoken(t *testing.T) {
	planner := &fakePlanner{onTheWayPlaces: []Place{{Name: "Place A"}, {Name: "Place B"}}}
	tts := &fakeTTS{}
	h := &Handler{Planner: planner, Announcer: tts}
	if err := h.Dispatch(context.Background(), TripIntent{Kind: IntentOnTheWay, Dest: "X"}); err != nil {
		t.Fatalf("Dispatch err: %v", err)
	}
	// no digit-then-dot-then-digit (decimal coord) appears in the spoken text.
	s := tts.last()
	for i := 0; i < len(s)-2; i++ {
		if isDigit(s[i]) && s[i+1] == '.' && isDigit(s[i+2]) {
			t.Fatalf("decimal coord leaked into utterance: %q", s)
		}
	}
}

func TestHandler_NilSeams_ReturnError(t *testing.T) {
	h := &Handler{}
	if err := h.Dispatch(context.Background(), TripIntent{Kind: IntentOnTheWay}); err == nil {
		t.Fatal("expected error for nil Planner")
	}
	h = &Handler{Planner: &fakePlanner{}}
	if err := h.Dispatch(context.Background(), TripIntent{Kind: IntentOnTheWay}); err == nil {
		t.Fatal("expected error for nil Announcer")
	}
}

func TestHandler_Dispatch_MeetInMiddle(t *testing.T) {
	planner := &fakePlanner{midPlaces: []Place{{Name: "Mercer Coffee"}, {Name: "Halfway Brew"}}}
	tts := &fakeTTS{}
	h := &Handler{Planner: planner, Announcer: tts, DefaultOrigin: "Seattle"}
	intent := TripIntent{Kind: IntentMeetInMiddle, Dest: "Sarah", Categories: []string{"coffee"}}
	if err := h.Dispatch(context.Background(), intent); err != nil {
		t.Fatalf("Dispatch err: %v", err)
	}
	if planner.meetMiddleCalls != 1 {
		t.Fatalf("expected 1 MeetInMiddle call, got %d", planner.meetMiddleCalls)
	}
	if planner.lastCategory != "coffee" {
		t.Fatalf("expected category=coffee, got %q", planner.lastCategory)
	}
	if !strings.Contains(tts.last(), "Mercer Coffee") {
		t.Fatalf("expected midpoint utterance to name Mercer Coffee, got %q", tts.last())
	}
}

func isDigit(b byte) bool { return b >= '0' && b <= '9' }
