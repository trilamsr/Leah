package intent

import (
	"regexp"
	"strings"
)

type Kind int

const (
	KindAsk Kind = iota
	KindShip
	KindReview
	KindStatus
)

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
