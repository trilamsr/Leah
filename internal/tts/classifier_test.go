package tts_test

import (
	"testing"
	"time"

	"github.com/trilam/leah/internal/tts"
)

func TestBlockwordClassifier_DefaultCorpus_FlagsCalendar(t *testing.T) {
	c := tts.NewBlockwordClassifier()
	if got := c.Route("Meeting with Sarah at 3pm Tuesday"); got != tts.RouteLocal {
		t.Fatalf("calendar text must route LOCAL, got %v", got)
	}
}

func TestBlockwordClassifier_DefaultCorpus_FlagsFinance(t *testing.T) {
	c := tts.NewBlockwordClassifier()
	if got := c.Route("Your Chase balance is $4,237.18"); got != tts.RouteLocal {
		t.Fatalf("finance text must route LOCAL, got %v", got)
	}
}

func TestBlockwordClassifier_DefaultCorpus_FlagsEmail(t *testing.T) {
	c := tts.NewBlockwordClassifier()
	if got := c.Route("Re: project status — sent from my iPhone"); got != tts.RouteLocal {
		t.Fatalf("email cues must route LOCAL, got %v", got)
	}
}

func TestBlockwordClassifier_DefaultCorpus_FlagsMemoryWords(t *testing.T) {
	c := tts.NewBlockwordClassifier()
	for _, txt := range []string{
		"My password is hunter2",
		"SSN on file",
		"routing number changed",
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

// §17.17 budget: classifier p99 < 5 ms even on long inputs.
func TestBlockwordClassifier_Budget_Under5ms(t *testing.T) {
	c := tts.NewBlockwordClassifier()
	long := make([]byte, 8192)
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
