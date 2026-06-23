package tts

import (
	"regexp"
	"strings"
)

// BlockwordClassifier flags text that names sensitive content domains per
// §2.7: calendar event titles, email subjects/bodies, finance amounts/account
// names, memory items. Hit → RouteLocal (Apple); miss → RouteCloud
// (ElevenLabs). Runs under the 5 ms budget §17.17 mandates.
//
// The default corpus is baked in. A future revision will let the operator
// override via ~/Library/Application Support/Leah/tts-blockwords.json; the
// hook lives in NewBlockwordClassifier so callers do not change.
type BlockwordClassifier struct {
	moneyRe     *regexp.Regexp
	calendarRe  *regexp.Regexp
	emailRe     *regexp.Regexp
	memoryWords []string
}

// NewBlockwordClassifier returns a classifier seeded with the default corpus.
func NewBlockwordClassifier() *BlockwordClassifier {
	return &BlockwordClassifier{
		// Currency: $1,234.56 or $4237 or USD 100 — any of these flags finance.
		moneyRe: regexp.MustCompile(`\$\d[\d,]*(\.\d+)?|USD\s*\d`),
		// Calendar pattern: "meeting/call/standup/... at TIME [am|pm]".
		calendarRe: regexp.MustCompile(`(?i)\b(meeting|call|standup|sync|1:1|interview)\b.*\b\d{1,2}(:\d{2})?\s*(am|pm)?\b`),
		// Email cues: Re:/Fwd: subject lines or common signature footers.
		emailRe: regexp.MustCompile(`(?i)\b(re|fwd|fw):\s|sent from my|best regards|sincerely`),
		// Memory blockwords: lowercase substrings of stored personal facts.
		memoryWords: []string{
			"password", "ssn", "social security",
			"credit card", "routing number", "account number",
			"home address", "phone number",
		},
	}
}

// Route applies each detector in increasing-cost order; first hit wins.
func (c *BlockwordClassifier) Route(text string) Route {
	low := strings.ToLower(text)
	for _, w := range c.memoryWords {
		if strings.Contains(low, w) {
			return RouteLocal
		}
	}
	if c.moneyRe.MatchString(text) {
		return RouteLocal
	}
	if c.calendarRe.MatchString(text) {
		return RouteLocal
	}
	if c.emailRe.MatchString(text) {
		return RouteLocal
	}
	return RouteCloud
}
