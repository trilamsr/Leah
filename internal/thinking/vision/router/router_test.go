package router

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/trilam/leah/internal/thinking/vision"
)

type fakeOCR struct {
	blocks []vision.TextBlock
	err    error
}

func (f *fakeOCR) Recognize(_ context.Context, _ vision.Image) ([]vision.TextBlock, error) {
	return f.blocks, f.err
}

type fakeSonnet struct {
	chunks    []string
	err       error
	streamErr error
	calls     atomic.Int32
}

func (f *fakeSonnet) StreamVision(_ context.Context, _ vision.Image, _ string) (<-chan VisionChunk, error) {
	f.calls.Add(1)
	if f.err != nil {
		return nil, f.err
	}
	out := make(chan VisionChunk, len(f.chunks)+1)
	for _, c := range f.chunks {
		out <- VisionChunk{Text: c}
	}
	if f.streamErr != nil {
		out <- VisionChunk{Err: f.streamErr}
	}
	close(out)
	return out, nil
}

type fakeMeter struct {
	chargeErr error
	calls     int
	last      int64
}

func (m *fakeMeter) Charge(_ context.Context, _ string, n int64) error {
	m.calls++
	m.last = n
	return m.chargeErr
}

func newTestImage() vision.Image {
	return vision.Image{Pixels: make([]byte, 16), Width: 2, Height: 2, MIME: "image/rgba"}
}

func drain(t *testing.T, ch <-chan ReasonerEvent) []ReasonerEvent {
	t.Helper()
	var got []ReasonerEvent
	for ev := range ch {
		got = append(got, ev)
	}
	return got
}

func TestAsk_LocalMode_NoSonnetCallNoConsent(t *testing.T) {
	sonnet := &fakeSonnet{}
	consent := newMemConsent()
	r := New(&fakeOCR{}, sonnet, consent, nil, func(string) bool {
		t.Fatal("prompt must not fire for VisionLocal")
		return false
	})
	ch, err := r.Ask(context.Background(), newTestImage(), "what is this", VisionLocal)
	if err != nil {
		t.Fatalf("Ask: %v", err)
	}
	drain(t, ch)
	if sonnet.calls.Load() != 0 {
		t.Fatalf("VisionLocal must not call Sonnet; got %d", sonnet.calls.Load())
	}
}

// VisionLocal must surface OCR text via the Ask channel rather than returning
// an empty stream — handleVisionSnap (cmd/leah-daemon/ipc_vision.go) routes
// every mode through Ask, so a silent empty channel renders as a blank HUD
// reply. Regression guard for the router-OCR gap.
func TestAsk_LocalMode_StreamsOCRText(t *testing.T) {
	ocr := &fakeOCR{blocks: []vision.TextBlock{
		{Text: "hello"},
		{Text: "world"},
	}}
	r := New(ocr, &fakeSonnet{}, newMemConsent(), nil, nil)
	ch, err := r.Ask(context.Background(), newTestImage(), "read text", VisionLocal)
	if err != nil {
		t.Fatalf("Ask: %v", err)
	}
	got := drain(t, ch)
	if len(got) == 0 {
		t.Fatal("VisionLocal returned zero events; HUD would render blank reply")
	}
	var text string
	var sawFinal bool
	for _, ev := range got {
		text += ev.Text
		if ev.IsFinal {
			sawFinal = true
		}
	}
	if text == "" {
		t.Fatalf("VisionLocal stream had no text; got %+v", got)
	}
	if !sawFinal {
		t.Fatalf("VisionLocal stream missing IsFinal terminator; got %+v", got)
	}
}

func TestAsk_LocalMode_OCRError_SurfacesAsTerminalEvent(t *testing.T) {
	ocr := &fakeOCR{err: errors.New("ocr boom")}
	r := New(ocr, &fakeSonnet{}, newMemConsent(), nil, nil)
	ch, err := r.Ask(context.Background(), newTestImage(), "p", VisionLocal)
	if err != nil {
		t.Fatalf("Ask: %v", err)
	}
	got := drain(t, ch)
	if len(got) == 0 || got[len(got)-1].Err == nil {
		t.Fatalf("VisionLocal OCR error must surface as terminal Err event; got %+v", got)
	}
}

func TestAsk_SonnetWithoutConsent_PromptsAndDenies(t *testing.T) {
	sonnet := &fakeSonnet{}
	consent := newMemConsent()
	r := New(&fakeOCR{}, sonnet, consent, nil, func(mode string) bool {
		if mode != "screenshot" {
			t.Fatalf("expected screenshot mode, got %q", mode)
		}
		return false
	})
	_, err := r.Ask(context.Background(), newTestImage(), "p", VisionSonnet)
	if !errors.Is(err, ErrConsentDenied) {
		t.Fatalf("expected ErrConsentDenied, got %v", err)
	}
	if sonnet.calls.Load() != 0 {
		t.Fatal("denied consent must not call Sonnet")
	}
}

func TestAsk_SonnetWithConsent_StreamsAndChargesBudget(t *testing.T) {
	sonnet := &fakeSonnet{chunks: []string{"hello", " world"}}
	consent := newMemConsent()
	consent.Grant("screenshot", ScopeThisSession)
	meter := &fakeMeter{}
	r := New(&fakeOCR{}, sonnet, consent, meter, func(string) bool {
		t.Fatal("prompt must not fire when consent already granted")
		return false
	})
	ch, err := r.Ask(context.Background(), newTestImage(), "p", VisionSonnet)
	if err != nil {
		t.Fatalf("Ask: %v", err)
	}
	got := drain(t, ch)
	if len(got) != 3 || got[0].Text != "hello" || got[1].Text != " world" || !got[2].IsFinal {
		t.Fatalf("unexpected event sequence: %+v", got)
	}
	if meter.calls != 1 || meter.last != 16 {
		t.Fatalf("expected one Charge(16); got calls=%d last=%d", meter.calls, meter.last)
	}
}

func TestAsk_SonnetPromptGrantsForRestOfSession(t *testing.T) {
	sonnet := &fakeSonnet{chunks: []string{"ok"}}
	consent := newMemConsent()
	prompts := 0
	r := New(&fakeOCR{}, sonnet, consent, nil, func(string) bool {
		prompts++
		return true
	})
	for i := 0; i < 3; i++ {
		ch, err := r.Ask(context.Background(), newTestImage(), "p", VisionSonnet)
		if err != nil {
			t.Fatalf("Ask[%d]: %v", i, err)
		}
		drain(t, ch)
	}
	if prompts != 1 {
		t.Fatalf("prompt must fire exactly once across the session; got %d", prompts)
	}
}

func TestAsk_BudgetOverCap_DegradesToLocal(t *testing.T) {
	sonnet := &fakeSonnet{chunks: []string{"X"}}
	consent := newMemConsent()
	consent.Grant("screenshot", ScopeThisSession)
	meter := &fakeMeter{chargeErr: errors.New("budget: over cap")}
	r := New(&fakeOCR{}, sonnet, consent, meter, func(string) bool { return true })
	ch, err := r.Ask(context.Background(), newTestImage(), "p", VisionSonnet)
	if err != nil {
		t.Fatalf("Ask: %v", err)
	}
	got := drain(t, ch)
	if sonnet.calls.Load() != 0 {
		t.Fatal("over-cap must skip Sonnet")
	}
	if len(got) != 1 || !got[0].IsFinal || got[0].Err == nil {
		t.Fatalf("expected single terminal error event; got %+v", got)
	}
}

func TestAsk_SonnetStreamError_PropagatesAsTerminalEvent(t *testing.T) {
	sonnet := &fakeSonnet{err: errors.New("network down")}
	consent := newMemConsent()
	consent.Grant("screenshot", ScopeThisSession)
	r := New(&fakeOCR{}, sonnet, consent, nil, nil)
	ch, err := r.Ask(context.Background(), newTestImage(), "p", VisionSonnet)
	if err != nil {
		t.Fatalf("Ask: %v", err)
	}
	got := drain(t, ch)
	if len(got) != 1 || !got[0].IsFinal || got[0].Err == nil {
		t.Fatalf("expected terminal error event; got %+v", got)
	}
}

func TestAsk_SonnetMidStreamError_PropagatesAsTerminalEvent(t *testing.T) {
	// StreamVision returns nil err on entry, then the underlying SDK stream
	// fails partway (rate-limit, timeout, auth). The router MUST surface that
	// as a terminal Err event — otherwise the HUD renders a truncated answer.
	sonnet := &fakeSonnet{chunks: []string{"partial"}, streamErr: errors.New("rate limit")}
	consent := newMemConsent()
	consent.Grant("screenshot", ScopeThisSession)
	r := New(&fakeOCR{}, sonnet, consent, nil, nil)
	ch, err := r.Ask(context.Background(), newTestImage(), "p", VisionSonnet)
	if err != nil {
		t.Fatalf("Ask: %v", err)
	}
	got := drain(t, ch)
	if len(got) != 2 || got[0].Text != "partial" || !got[1].IsFinal || got[1].Err == nil {
		t.Fatalf("expected partial text then terminal error; got %+v", got)
	}
}

func TestAsk_ConcurrentSonnetAsks_PromptFiresOnce(t *testing.T) {
	sonnet := &fakeSonnet{chunks: []string{"ok"}}
	consent := newMemConsent()
	var prompts int32
	// gate blocks the prompt callback so every racing Ask piles up on the
	// gateMu — without the mutex they'd all observe !Granted and each fire.
	release := make(chan struct{})
	r := New(&fakeOCR{}, sonnet, consent, nil, func(string) bool {
		atomic.AddInt32(&prompts, 1)
		<-release
		return true
	})
	const N = 8
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func() {
			defer wg.Done()
			ch, err := r.Ask(context.Background(), newTestImage(), "p", VisionSonnet)
			if err != nil {
				t.Errorf("Ask: %v", err)
				return
			}
			drain(t, ch)
		}()
	}
	close(release)
	wg.Wait()
	if got := atomic.LoadInt32(&prompts); got != 1 {
		t.Fatalf("prompt must fire exactly once under concurrent Asks; got %d", got)
	}
}

func TestOCR_DelegatesToEngine(t *testing.T) {
	want := []vision.TextBlock{{Text: "hi"}}
	r := New(&fakeOCR{blocks: want}, nil, newMemConsent(), nil, nil)
	got, err := r.OCR(context.Background(), newTestImage())
	if err != nil {
		t.Fatalf("OCR: %v", err)
	}
	if len(got) != 1 || got[0].Text != "hi" {
		t.Fatalf("OCR passthrough failed: %+v", got)
	}
}
