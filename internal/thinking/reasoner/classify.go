package reasoner

import (
	"context"
	"encoding/json"
	"fmt"
)

// Intent is the classifier output: Kind ∈ {"chat","widget"},
// Widget ∈ {"stat","table","list","citation",""}, Confidence in 0..1.
// URL is set only for citation intent — the URL the user wants summarised.
type Intent struct {
	Kind       string  `json:"kind"`
	Widget     string  `json:"widget"`
	URL        string  `json:"url,omitempty"`
	Confidence float64 `json:"confidence"`
}

const classifySystem = `You are a router. Reply with ONE line of JSON:
{"kind":"chat"} for conversational replies.
{"kind":"widget","widget":"stat|table|list","confidence":0..1} when the user
is asking for a metric (stat), tabular data (table), or an enumeration (list).
{"kind":"widget","widget":"citation","url":"<url>","confidence":0..1} when
the user pastes a URL they want summarised as a source tile.
Never include prose, only the JSON line.`

// oneShot is the interface the classifier calls — satisfied by AnthropicClient
// in production and haikuStub in tests.
type oneShot interface {
	OneShot(ctx context.Context, system, user string) (string, error)
}

// Classify runs the widget-intent classifier using a real AnthropicClient
// initialised with claude-haiku-4-5. Callers that need a test double use
// classifyWith directly.
func Classify(ctx context.Context, haiku *AnthropicClient, userText string) (Intent, error) {
	return classifyWith(ctx, haiku, userText)
}

// classifyWith is the testable core: it accepts the oneShot interface so
// unit tests can inject haikuStub without a live API key.
func classifyWith(ctx context.Context, s oneShot, userText string) (Intent, error) {
	raw, err := s.OneShot(ctx, classifySystem, userText)
	if err != nil {
		return Intent{}, fmt.Errorf("haiku oneshot: %w", err)
	}
	var i Intent
	if err := json.Unmarshal([]byte(raw), &i); err != nil {
		// Degrade silently to chat on malformed Haiku output (spec §10).
		return Intent{Kind: "chat"}, nil
	}
	if i.Kind == "" {
		i.Kind = "chat"
	}
	return i, nil
}
