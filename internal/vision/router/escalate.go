package router

import (
	"strings"

	"github.com/trilam/leah/internal/vision"
)

// reasoningCues are lowercase substrings whose presence in the prompt implies
// the caller wants synthesis (cause, comparison, summarization, translation),
// not just glyph extraction. Local OCR only emits text + boxes — it cannot
// answer "why" or "explain". Keep this list small and high-precision; false
// positives push us onto the cloud path and burn cloud.vision.bytes budget.
var reasoningCues = []string{
	"why",
	"explain",
	"compare",
	"what's wrong",
	"whats wrong",
	"summarize",
	"summary",
	"describe",
	"translate",
	"identify",
	"how does",
	"how do i",
}

// minPixelsForCloud is the smallest image we'll bother sending to Sonnet on a
// non-reasoning prompt. Below this, the Sonnet round-trip cost dwarfs any
// signal lift over local OCR. 32x32 captures icon-class frames.
const minPixelsForCloud = 32 * 32

// largePixelThreshold escalates anything >= ~2MP regardless of prompt shape.
// At that size the frame is almost certainly a full-screen capture with
// multi-region layout that local OCR will read but not interpret. 1.92M
// matches a 1600x1200 window — Retina half-res screenshots land above this.
const largePixelThreshold = 1_920_000

// shouldEscalate is the VisionAuto heuristic. Pure function — testable without
// network or CGO. Three signals:
//
//  1. Reasoning prompt → always escalate. The caller wants synthesis; local
//     OCR returns boxes, not answers.
//  2. Large frame (≥ ~2MP) → escalate. Full-screen captures need layout-aware
//     vision; OCR alone misses structure.
//  3. Tiny frame (< 32×32) → never escalate. Icon-class, OCR is sufficient
//     and Sonnet latency would dominate.
//
// OCR-confidence and face-detection signals (suggested in T04 spec) require a
// platform pre-pass that would re-enter CGO — deferred until the heuristic
// shows itself insufficient against real traffic.
func shouldEscalate(frame vision.Image, prompt string) bool {
	pixels := frame.Width * frame.Height
	if pixels < minPixelsForCloud {
		return false
	}
	if pixels >= largePixelThreshold {
		return true
	}
	lower := strings.ToLower(prompt)
	for _, cue := range reasoningCues {
		if strings.Contains(lower, cue) {
			return true
		}
	}
	return false
}
