package router

import (
	"context"
	"errors"

	"github.com/trilam/leah/internal/vision"
)

// ErrSonnetNotWired is returned by the placeholder SonnetClient until T04.b
// extends reasoner.AnthropicClient with a Base64ImageSource Messages path.
// Surfaces as a terminal ReasonerEvent with the error embedded — callers
// fall back to local OCR per §8.4 ActionSwitchLocal.
var ErrSonnetNotWired = errors.New("vision: Sonnet vision client not wired (deferred to T04.b)")

// NotWiredSonnetClient is the production wiring placeholder. Returning a
// terminal-error stream (rather than a hard caller-side error) keeps the
// router's degrade-to-local path symmetrical between budget-overrun and
// missing-implementation conditions — both surface as ReasonerEvent.Err.
type NotWiredSonnetClient struct{}

func (NotWiredSonnetClient) StreamVision(_ context.Context, _ vision.Image, _ string) (<-chan string, error) {
	return nil, ErrSonnetNotWired
}
