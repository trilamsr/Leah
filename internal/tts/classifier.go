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
	emailAddrRe *regexp.Regexp
	memoryWords []string
}

// NewBlockwordClassifier returns a classifier seeded with the default corpus.
func NewBlockwordClassifier() *BlockwordClassifier {
	return &BlockwordClassifier{
		// Currency sigils ($ £ € ¥) or ISO codes (USD/EUR/GBP/JPY/CNY/CHF/CAD/AUD).
		moneyRe: regexp.MustCompile(`(?i)[$£€¥]\d[\d,]*(\.\d+)?|\b(usd|eur|gbp|jpy|cny|chf|cad|aud)\s*\d`),
		// Calendar pattern: event verb at TIME [am|pm].
		calendarRe: regexp.MustCompile(`(?i)\b(meeting|call|standup|sync|1:1|interview|coffee|lunch|dinner|drinks|appointment|reservation)\b.*\b\d{1,2}(:\d{2})?\s*(am|pm)?\b`),
		// Email cues: Re:/Fwd: subject lines or common signature footers.
		emailRe: regexp.MustCompile(`(?i)\b(re|fwd|fw):\s|sent from my|best regards|sincerely`),
		// Bare email address — anything containing one is treated as email body.
		emailAddrRe: regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`),
		// Memory blockwords: lowercase substrings of stored personal facts.
		// Covers finance/contact + auth-secrets + medical per §2.7 "memory items".
		memoryWords: []string{
			"password", "ssn", "social security",
			"credit card", "routing number", "account number",
			"home address", "phone number",
			"api key", "api token", "access token", "secret key", "private key",
			"diagnosis", "prescription", "medication",
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
	if c.emailRe.MatchString(text) || c.emailAddrRe.MatchString(text) {
		return RouteLocal
	}
	return RouteCloud
}
