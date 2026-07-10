package router

import (
	"testing"

	"github.com/trilam/leah/internal/thinking/vision"
)

func img(w, h int) vision.Image {
	return vision.Image{Pixels: make([]byte, w*h*4), Width: w, Height: h, MIME: "image/rgba"}
}

func TestShouldEscalate_ReasoningPromptForcesEscalate(t *testing.T) {
	// Small image but a reasoning prompt — caller wants explanation, not just
	// the extracted glyphs. Local OCR can't synthesize an answer.
	for _, prompt := range []string{
		"why is this red",
		"explain this chart",
		"compare these two columns",
		"what's wrong here",
		"summarize the page",
		"describe what's happening",
		"translate this to french",
	} {
		if !shouldEscalate(img(64, 64), prompt) {
			t.Fatalf("reasoning prompt %q must escalate", prompt)
		}
	}
}

func TestShouldEscalate_TinyImageStaysLocal(t *testing.T) {
	// Icons / cursors — local OCR is more than enough and Sonnet round-trip
	// dwarfs the value of one or two glyphs.
	if shouldEscalate(img(24, 24), "read text") {
		t.Fatal("tiny image must not escalate")
	}
}

func TestShouldEscalate_LargeImageEscalates(t *testing.T) {
	// Roughly 2MP — full-screen capture on retina. Even an extraction prompt
	// benefits from layout-aware vision.
	if !shouldEscalate(img(1920, 1080), "read text") {
		t.Fatal("large image must escalate")
	}
}

func TestShouldEscalate_MidImagePlainPromptStaysLocal(t *testing.T) {
	// 200x200 window, extraction-only prompt — local OCR wins.
	if shouldEscalate(img(200, 200), "read text") {
		t.Fatal("mid image + plain extract prompt must not escalate")
	}
}

func TestShouldEscalate_EmptyPromptStaysLocal(t *testing.T) {
	if shouldEscalate(img(200, 200), "") {
		t.Fatal("empty prompt has no reasoning signal — must not escalate")
	}
}

func TestShouldEscalate_ReasoningPromptIsCaseInsensitive(t *testing.T) {
	if !shouldEscalate(img(64, 64), "WHY is this red") {
		t.Fatal("case-insensitive reasoning match must escalate")
	}
}

func TestAsk_VisionAuto_PlainPromptStaysLocal(t *testing.T) {
	// VisionAuto with extraction-only prompt + mid frame → shouldEscalate
	// returns false → Sonnet never dialed, consent never prompted.
	sonnet := &fakeSonnet{}
	r := New(&fakeOCR{}, sonnet, newMemConsent(), nil, func(string) bool {
		t.Fatal("plain auto-mode prompt must not trigger consent")
		return false
	})
	ch, err := r.Ask(t.Context(), img(200, 200), "read text", VisionAuto)
	if err != nil {
		t.Fatalf("Ask: %v", err)
	}
	drain(t, ch)
	if sonnet.calls.Load() != 0 {
		t.Fatalf("VisionAuto on plain prompt must skip Sonnet; got %d", sonnet.calls.Load())
	}
}
