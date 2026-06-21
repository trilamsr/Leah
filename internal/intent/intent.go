// Package intent is the regex classifier that maps a free-text utterance
// to one of the four CLI verbs (ask/ship/review/status). Kept regex-only
// (no LLM) so intent dispatch stays deterministic + zero-cost.
package intent

import (
	"regexp"
	"strings"
)

// Kind enumerates the canonical dispatcher verbs. KindAsk is the default
// fallback when no other regex matches.
type Kind int

// Canonical verbs the classifier resolves to. kindMax bounds enumeration so
// RegisterMetrics seeds every kind without drifting when new verbs land.
const (
	KindAsk Kind = iota
	KindShip
	KindReview
	KindStatus
	kindMax
)

// String returns the lowercase verb name (matches the CLI subcommand).
func (k Kind) String() string {
	switch k {
	case KindAsk:
		return "ask"
	case KindShip:
		return "ship"
	case KindReview:
		return "review"
	case KindStatus:
		return "status"
	}
	return "unknown"
}

var (
	shipRe   = regexp.MustCompile(`(?i)^(please\s+)?(ship|deploy|fix\s+bug\s+\d+)`)
	reviewRe = regexp.MustCompile(`(?i)^(please\s+)?review\s+(pr\s+)?#?\d+`)
	statusRe = regexp.MustCompile(`(?i)^(status|what['’]s\s+running|any\s+open\s+prs|regatta\s+status)`)
)

// Classify returns the first verb whose regex matches s (trimmed),
// defaulting to KindAsk when none match.
func Classify(s string) Kind {
	s = strings.TrimSpace(s)
	if shipRe.MatchString(s) {
		return KindShip
	}
	if reviewRe.MatchString(s) {
		return KindReview
	}
	if statusRe.MatchString(s) {
		return KindStatus
	}
	return KindAsk
}
