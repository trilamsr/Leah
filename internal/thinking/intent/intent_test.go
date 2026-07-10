package intent

import "testing"

func TestClassifyRecognizesShipPattern(t *testing.T) {
	cases := []struct {
		in   string
		want Kind
	}{
		{"ship the fix for bug 1086", KindShip},
		{"ship a typo fix in regatta", KindShip},
		{"please ship the prwatch refactor", KindShip},
		{"deploy the new dashboard polish", KindShip},
		{"fix bug 1099 in regatta", KindShip},
	}
	for _, c := range cases {
		got := Classify(c.in)
		if got != c.want {
			t.Errorf("Classify(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestClassifyRecognizesReviewPattern(t *testing.T) {
	for _, in := range []string{
		"review pr 1112",
		"review PR #1112",
		"please review 1112",
	} {
		if got := Classify(in); got != KindReview {
			t.Errorf("Classify(%q) = %v, want KindReview", in, got)
		}
	}
}

func TestClassifyRecognizesStatusPattern(t *testing.T) {
	for _, in := range []string{
		"status",
		"what's running",
		"any open prs",
		"regatta status",
	} {
		if got := Classify(in); got != KindStatus {
			t.Errorf("Classify(%q) = %v, want KindStatus", in, got)
		}
	}
}

func TestClassifyDefaultsToAsk(t *testing.T) {
	for _, in := range []string{
		"what does regatta currently support",
		"how does the reviewer-verdict gate work",
		"hello",
		"",
		"fix 3 typos in readme",
	} {
		if got := Classify(in); got != KindAsk {
			t.Errorf("Classify(%q) = %v, want KindAsk", in, got)
		}
	}
}
