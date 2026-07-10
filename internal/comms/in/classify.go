package commsin

import (
	"context"
	"regexp"
	"strings"
)

// ReasonerSeam matches voice/loop.ReasonerSeam structurally so this package
// stays free of an LLM-package import. Nil is the daemon's "LLM not wired"
// state — fallback degrades to regex-only.
type ReasonerSeam interface {
	Ask(ctx context.Context, prompt string) (string, error)
}

// Spec §5 mandates `\b` (word boundary) on the ASCII tokens — "yes please"
// hits accept but "yesterday" doesn't. Emoji tokens are split out because
// Go's `\b` is ASCII-only and a non-word→non-word transition (emoji-end →
// EOS) never satisfies it; they match on their own with a whitespace-or-EOS
// tail instead.
var (
	reAccept      = regexp.MustCompile(`(?i)^(y|yes|ok|okay|approve|do it|go|ship it)\b`)
	reReject      = regexp.MustCompile(`(?i)^(n|no|nope|reject|stop|don'?t|skip)\b`)
	reDefer       = regexp.MustCompile(`(?i)^(later|snooze|not now|defer|tomorrow)\b`)
	reAcceptEmoji = regexp.MustCompile(`^(👍|✅)(\s|$)`)
	reRejectEmoji = regexp.MustCompile(`^(👎|❌)(\s|$)`)
)

// RegexClassifier is the zero-cost fast path (spec §5). Unknown is returned
// for any input the table misses; the router treats Unknown as no-op, so a
// misclassification fails safe.
type RegexClassifier struct{}

func NewRegexClassifier() *RegexClassifier { return &RegexClassifier{} }

func (*RegexClassifier) Classify(_ context.Context, text string) (Intent, error) {
	return classifyRegex(text), nil
}

// FallbackClassifier runs the regex tier first and only consults the LLM seam
// on a miss. Spec §5 fail-safe: LLM transport error or any out-of-vocabulary
// label collapses to IntentUnknown — ambiguity never escalates to accept.
type FallbackClassifier struct {
	Reasoner ReasonerSeam
}

func NewFallbackClassifier(r ReasonerSeam) *FallbackClassifier {
	return &FallbackClassifier{Reasoner: r}
}

func (c *FallbackClassifier) Classify(ctx context.Context, text string) (Intent, error) {
	if intent := classifyRegex(text); intent != IntentUnknown {
		return intent, nil
	}
	if c.Reasoner == nil {
		return IntentUnknown, nil
	}
	reply, err := c.Reasoner.Ask(ctx, llmPrompt(text))
	if err != nil {
		return IntentUnknown, nil
	}
	return parseLabel(reply), nil
}

func classifyRegex(text string) Intent {
	t := strings.TrimSpace(text)
	if t == "" {
		return IntentUnknown
	}
	if reAccept.MatchString(t) || reAcceptEmoji.MatchString(t) {
		return IntentAccept
	}
	if reReject.MatchString(t) || reRejectEmoji.MatchString(t) {
		return IntentReject
	}
	if reDefer.MatchString(t) {
		return IntentDefer
	}
	return IntentUnknown
}

func parseLabel(reply string) Intent {
	switch strings.ToLower(strings.TrimSpace(reply)) {
	case "accept":
		return IntentAccept
	case "reject":
		return IntentReject
	case "defer":
		return IntentDefer
	}
	return IntentUnknown
}

// llmPrompt instructs the reasoner to emit exactly one of four labels.
// Default-unknown framing comes straight from spec §5.
func llmPrompt(text string) string {
	return "Classify the following reply to a recommendation prompt into exactly one label: accept, reject, defer, unknown. Respond with the label only, no punctuation. Default to unknown on any ambiguity.\n\nReply: " + text
}

var (
	_ Classifier = (*RegexClassifier)(nil)
	_ Classifier = (*FallbackClassifier)(nil)
)
