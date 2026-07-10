package tts

import (
	"regexp"
	"strings"

	"golang.org/x/text/unicode/norm"
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
		// Currency sigils incl. full-width ＄ ￡ ￥ ＄ + extended ISO codes.
		// Also tail-form "100 USD" — number BEFORE code.
		moneyRe: regexp.MustCompile(`(?i)[$£€¥₹₩₽元＄￡￥]\d[\d,]*(\.\d+)?|\b(usd|eur|gbp|jpy|cny|chf|cad|aud|krw|inr|rub|hkd|sgd|nzd|sek|nok|dkk|brl|mxn|zar)\s*\d|\d[\d,]*(\.\d+)?\s*(usd|eur|gbp|jpy|cny|chf|cad|aud|krw|inr|rub|hkd|sgd|nzd|sek|nok|dkk|brl|mxn|zar)\b`),
		// Calendar: event-verb (en + common short non-en) at TIME or date.
		// Non-English: réunion (fr), 会議 (ja), junta (es), Termin (de), riunione (it).
		// Time pattern is either 12h with am/pm OR 24h HH:MM OR JP 14時.
		calendarRe: regexp.MustCompile(`(?i)\b(meeting|call|standup|sync|1:1|interview|coffee|lunch|dinner|drinks|appointment|reservation|réunion|junta|termin|riunione|会議)\b|\b\d{1,2}:\d{2}\s*(am|pm)?\b|\b\d{1,2}\s*(am|pm)\b|\d{1,2}時`),
		// Email cues incl. common greeting openings.
		emailRe:     regexp.MustCompile(`(?i)\b(re|fwd|fw):\s|sent from my|best regards|sincerely|^\s*(hi|hello|dear|hey)\s+\w+,`),
		emailAddrRe: regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`),
		// Memory blockwords: finance + contact + auth-secrets + medical + IDs.
		memoryWords: []string{
			"password", "passcode", "pin code", "ssn", "social security",
			"credit card", "routing number", "account number", "iban", "swift code", "sort code",
			"tax id", "ein", "tin", "passport", "license number", "drivers license", "driver's license",
			"date of birth", "dob", "mother's maiden", "mothers maiden",
			"home address", "phone number",
			"api key", "api token", "access token", "secret key", "private key", "bearer token", "session token",
			// Medical (PHI):
			"diagnosis", "prescription", "medication",
			"hiv", "cancer", "pregnant", "pregnancy", "miscarriage",
			"therapist", "psychiatrist", "anxiety", "depression",
			"blood pressure", "insulin", "chemo", "biopsy", "mri",
		},
	}
}

// Route applies each detector in increasing-cost order; first hit wins.
// NFKC-normalize input first so full-width ＄ collapses to ASCII $ before regex.
func (c *BlockwordClassifier) Route(text string) Route {
	text = norm.NFKC.String(text)
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
