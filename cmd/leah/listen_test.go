package main

import "testing"

// TestExtractPRNumber covers the common voice-transcript shapes the intent
// classifier maps to KindReview ("review pr 123", "please review #456").
func TestExtractPRNumber(t *testing.T) {
	cases := []struct {
		in   string
		want int
		ok   bool
	}{
		{"review pr 123", 123, true},
		{"please review #456", 456, true},
		{"review 789", 789, true},
		{"review the latest", 0, false},
		{"", 0, false},
		{"check 12 issues and pr 34", 12, true}, // first run wins
	}
	for _, c := range cases {
		got, ok := extractPRNumber(c.in)
		if got != c.want || ok != c.ok {
			t.Errorf("extractPRNumber(%q) = (%d,%v), want (%d,%v)", c.in, got, ok, c.want, c.ok)
		}
	}
}

// TestTruncateTranscript asserts rune-safe clipping so multi-byte chars
// (whisper.cpp unicode punctuation) cannot be split mid-byte.
func TestTruncateTranscript(t *testing.T) {
	if got := truncateTranscript("hello", 10); got != "hello" {
		t.Errorf("short string mutated: %q", got)
	}
	if got := truncateTranscript("abcdef", 3); got != "abc…" {
		t.Errorf("ascii truncate: %q", got)
	}
	// Five unicode em-dashes (3 bytes each = 15 bytes); clip to 2 runes.
	got := truncateTranscript("———————", 2)
	want := "——…"
	if got != want {
		t.Errorf("unicode truncate: %q, want %q", got, want)
	}
}
