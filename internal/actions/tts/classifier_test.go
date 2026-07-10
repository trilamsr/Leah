package tts_test

import (
	"testing"
	"time"

	"github.com/trilam/leah/internal/tts"
)

func TestBlockwordClassifier_DefaultCorpus_FlagsCalendar(t *testing.T) {
	c := tts.NewBlockwordClassifier()
	for _, txt := range []string{
		"Meeting with Sarah at 3pm Tuesday",
		"Coffee with Sarah at 3pm Tuesday",
		"Lunch reservation at 12:30",
		"Dinner at 7 with the team",
		"Standup at 9am",
	} {
		if got := c.Route(txt); got != tts.RouteLocal {
			t.Fatalf("calendar %q must route LOCAL, got %v", txt, got)
		}
	}
}

func TestBlockwordClassifier_DefaultCorpus_FlagsFinance(t *testing.T) {
	c := tts.NewBlockwordClassifier()
	for _, txt := range []string{
		"Your Chase balance is $4,237.18",
		"Rent is £1,200 this month",
		"That dinner was €85",
		"Salary ¥4500000",
		"Wire usd 500 to the vendor",
		"Settled the invoice for EUR 230",
	} {
		if got := c.Route(txt); got != tts.RouteLocal {
			t.Fatalf("finance %q must route LOCAL, got %v", txt, got)
		}
	}
}

func TestBlockwordClassifier_DefaultCorpus_FlagsEmail(t *testing.T) {
	c := tts.NewBlockwordClassifier()
	for _, txt := range []string{
		"Re: project status — sent from my iPhone",
		"Please send the invoice to alice@acme.com",
		"Loop in bob.smith+work@example.co.uk on this",
	} {
		if got := c.Route(txt); got != tts.RouteLocal {
			t.Fatalf("email %q must route LOCAL, got %v", txt, got)
		}
	}
}

func TestBlockwordClassifier_DefaultCorpus_FlagsMemoryWords(t *testing.T) {
	c := tts.NewBlockwordClassifier()
	for _, txt := range []string{
		"My password is hunter2",
		"SSN on file",
		"routing number changed",
		"Rotate the API key in the vault",
		"Lab diagnosis came back yesterday",
		"Refill the prescription next week",
		"New access token expires Friday",
	} {
		if got := c.Route(txt); got != tts.RouteLocal {
			t.Fatalf("memory blockword %q must route LOCAL, got %v", txt, got)
		}
	}
}

func TestBlockwordClassifier_PublicFact_RoutesCloud(t *testing.T) {
	c := tts.NewBlockwordClassifier()
	if got := c.Route("The capital of France is Paris."); got != tts.RouteCloud {
		t.Fatalf("public-fact text must route CLOUD, got %v", got)
	}
}

func TestBlockwordClassifier_EmptyText_RoutesCloud(t *testing.T) {
	c := tts.NewBlockwordClassifier()
	if got := c.Route(""); got != tts.RouteCloud {
		t.Fatalf("empty text must route CLOUD (no sensitive signal), got %v", got)
	}
}

// §17.17 budget: per-call < 5 ms; skipped under -race (regex amplified ~10x).
func TestBlockwordClassifier_Budget_Under5ms(t *testing.T) {
	if raceEnabled {
		t.Skip("race detector amplifies regex cost; benchmark separately")
	}
	c := tts.NewBlockwordClassifier()
	long := make([]byte, 8192) // worst-case widget text per plan §17.17
	for i := range long {
		long[i] = 'a'
	}
	text := string(long)
	start := time.Now()
	for i := 0; i < 1000; i++ {
		_ = c.Route(text)
	}
	per := time.Since(start) / 1000
	if per > 5*time.Millisecond {
		t.Fatalf("classifier per-call %v exceeds 5 ms budget", per)
	}
}

// Classifier interface satisfied by BlockwordClassifier — guards future swaps.
func TestBlockwordClassifier_SatisfiesClassifier(t *testing.T) {
	var _ tts.Classifier = tts.NewBlockwordClassifier()
}
