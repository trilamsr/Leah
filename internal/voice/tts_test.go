package voice

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeExec records every Run/LookPath call and returns canned answers.
// Used to drive backend tests without requiring kokoro / say / afplay on
// the host.
type fakeExec struct {
	mu       sync.Mutex
	runs     []runCall
	lookups  []string
	lookupOK map[string]bool // binary name → exists on PATH
	runErr   map[string]error
	runOut   map[string][]byte
	// runHook fires inside Run before the canned reply is returned. Tests
	// use it to simulate side-effects (e.g. sox writing a wav file) without
	// touching the production code path.
	runHook func(name string, args []string)
}

type runCall struct {
	name string
	args []string
}

func newFakeExec() *fakeExec {
	return &fakeExec{
		lookupOK: map[string]bool{},
		runErr:   map[string]error{},
		runOut:   map[string][]byte{},
	}
}

func (f *fakeExec) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.runs = append(f.runs, runCall{name: name, args: args})
	if f.runHook != nil {
		f.runHook(name, args)
	}
	return f.runOut[name], f.runErr[name]
}

func (f *fakeExec) LookPath(name string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lookups = append(f.lookups, name)
	if f.lookupOK[name] {
		return "/usr/local/bin/" + name, nil
	}
	return "", errors.New("not found")
}

// stubTTS counts Speak calls and returns a canned error.
type stubTTS struct {
	name  string
	err   error
	calls int
}

func (s *stubTTS) Speak(ctx context.Context, text string) error {
	s.calls++
	return s.err
}

// TestChainPicksFirstAvailableBackend asserts ChainTTS calls the first
// backend; downstream backends are NOT invoked when the first succeeds.
func TestChainPicksFirstAvailableBackend(t *testing.T) {
	first := &stubTTS{name: "first"}
	second := &stubTTS{name: "second"}
	third := &stubTTS{name: "third"}

	c := NewChain(first, second, third)
	if err := c.Speak(context.Background(), "hello"); err != nil {
		t.Fatalf("speak: %v", err)
	}
	if first.calls != 1 {
		t.Errorf("first.calls = %d, want 1", first.calls)
	}
	if second.calls != 0 || third.calls != 0 {
		t.Errorf("downstream invoked: second=%d third=%d", second.calls, third.calls)
	}
}

// TestChainFallsThroughOnError asserts that when backend N errors the
// chain tries N+1.
func TestChainFallsThroughOnError(t *testing.T) {
	first := &stubTTS{name: "first", err: errors.New("boom")}
	second := &stubTTS{name: "second"} // succeeds
	third := &stubTTS{name: "third"}

	c := NewChain(first, second, third)
	if err := c.Speak(context.Background(), "hi"); err != nil {
		t.Fatalf("speak: %v", err)
	}
	if first.calls != 1 || second.calls != 1 {
		t.Errorf("call counts: first=%d second=%d", first.calls, second.calls)
	}
	if third.calls != 0 {
		t.Errorf("third invoked despite second success: %d", third.calls)
	}
}

// TestChainAllFail surfaces the last backend's error wrapped.
func TestChainAllFail(t *testing.T) {
	first := &stubTTS{err: errors.New("a")}
	second := &stubTTS{err: errors.New("b")}
	c := NewChain(first, second)
	err := c.Speak(context.Background(), "x")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "all backends failed") {
		t.Errorf("error missing context: %v", err)
	}
}

// TestChainEmpty returns an error rather than panicking on no backends.
func TestChainEmpty(t *testing.T) {
	c := NewChain()
	if err := c.Speak(context.Background(), "x"); err == nil {
		t.Fatal("expected error on empty chain")
	}
}

// TestSayBackendShellsOut asserts SayTTS invokes `say <text>`.
func TestSayBackendShellsOut(t *testing.T) {
	fe := newFakeExec()
	s := &SayTTS{Exec: fe}
	if err := s.Speak(context.Background(), "hello world"); err != nil {
		t.Fatalf("speak: %v", err)
	}
	if len(fe.runs) != 1 {
		t.Fatalf("runs = %d, want 1", len(fe.runs))
	}
	if fe.runs[0].name != "say" {
		t.Errorf("binary = %q, want say", fe.runs[0].name)
	}
	if len(fe.runs[0].args) != 1 || fe.runs[0].args[0] != "hello world" {
		t.Errorf("args = %v, want [hello world]", fe.runs[0].args)
	}
}

// TestSayBackendEmptyTextNoop asserts empty text does not shell out.
func TestSayBackendEmptyTextNoop(t *testing.T) {
	fe := newFakeExec()
	s := &SayTTS{Exec: fe}
	if err := s.Speak(context.Background(), ""); err != nil {
		t.Fatalf("speak: %v", err)
	}
	if len(fe.runs) != 0 {
		t.Errorf("empty text shelled out: %v", fe.runs)
	}
}

// TestKokoroBackendNotAvailable asserts pickBackends omits Kokoro when
// the binary is not on PATH.
func TestKokoroBackendNotAvailable(t *testing.T) {
	fe := newFakeExec()
	// No binary configured as present → all LookPath fail.
	getenv := func(string) string { return "" }
	bs := pickBackends(fe, getenv)
	if len(bs) != 0 {
		t.Errorf("expected 0 backends, got %d", len(bs))
	}
}

// TestPickBackendsKokoroOnly asserts Kokoro included when binary present
// and no other signals.
func TestPickBackendsKokoroOnly(t *testing.T) {
	fe := newFakeExec()
	fe.lookupOK["kokoro"] = true
	getenv := func(string) string { return "" }
	bs := pickBackends(fe, getenv)
	if len(bs) != 1 {
		t.Fatalf("len = %d, want 1", len(bs))
	}
	if _, ok := bs[0].(*KokoroTTS); !ok {
		t.Errorf("first backend = %T, want *KokoroTTS", bs[0])
	}
}

// TestPickBackendsFullChain asserts ordering: Kokoro, OpenAI, Say. The
// OpenAI tier requires both OPENAI_API_KEY and LEAH_VOICE_ALLOW_OPENAI=1
// (third-party egress opt-in; H5 in 2026-06-09 audit).
func TestPickBackendsFullChain(t *testing.T) {
	fe := newFakeExec()
	fe.lookupOK["kokoro"] = true
	fe.lookupOK["say"] = true
	getenv := func(k string) string {
		switch k {
		case "OPENAI_API_KEY":
			return "sk-test"
		case "LEAH_VOICE_ALLOW_OPENAI":
			return "1"
		}
		return ""
	}
	bs := pickBackends(fe, getenv)
	if len(bs) != 3 {
		t.Fatalf("len = %d, want 3", len(bs))
	}
	if _, ok := bs[0].(*KokoroTTS); !ok {
		t.Errorf("backend[0] = %T, want *KokoroTTS", bs[0])
	}
	if _, ok := bs[1].(*OpenAITTS); !ok {
		t.Errorf("backend[1] = %T, want *OpenAITTS", bs[1])
	}
	if _, ok := bs[2].(*SayTTS); !ok {
		t.Errorf("backend[2] = %T, want *SayTTS", bs[2])
	}
}

// TestPickBackendsOpenAIRequiresOptIn asserts that OPENAI_API_KEY alone
// is insufficient: without LEAH_VOICE_ALLOW_OPENAI=1 the OpenAI tier is
// dropped from the chain so notification bodies (intent strings, PR
// titles, agent IDs) are not silently round-tripped to a third party.
// Defensive default per H5 in the 2026-06-09 audit.
func TestPickBackendsOpenAIRequiresOptIn(t *testing.T) {
	fe := newFakeExec()
	fe.lookupOK["kokoro"] = true
	fe.lookupOK["say"] = true
	getenv := func(k string) string {
		if k == "OPENAI_API_KEY" {
			return "sk-test"
		}
		return ""
	}
	bs := pickBackends(fe, getenv)
	for i, b := range bs {
		if _, ok := b.(*OpenAITTS); ok {
			t.Fatalf("backend[%d] is OpenAITTS without LEAH_VOICE_ALLOW_OPENAI opt-in", i)
		}
	}
}

// TestPickBackendsOpenAIOptInWithoutKey asserts the opt-in env alone does
// nothing without the api key.
func TestPickBackendsOpenAIOptInWithoutKey(t *testing.T) {
	fe := newFakeExec()
	fe.lookupOK["say"] = true
	getenv := func(k string) string {
		if k == "LEAH_VOICE_ALLOW_OPENAI" {
			return "1"
		}
		return ""
	}
	bs := pickBackends(fe, getenv)
	for i, b := range bs {
		if _, ok := b.(*OpenAITTS); ok {
			t.Fatalf("backend[%d] is OpenAITTS without API key", i)
		}
	}
}

// TestPickBackendsSayOnly — neither kokoro nor api key, only say.
func TestPickBackendsSayOnly(t *testing.T) {
	fe := newFakeExec()
	fe.lookupOK["say"] = true
	getenv := func(string) string { return "" }
	bs := pickBackends(fe, getenv)
	if len(bs) != 1 {
		t.Fatalf("len = %d, want 1", len(bs))
	}
	if _, ok := bs[0].(*SayTTS); !ok {
		t.Errorf("backend = %T, want *SayTTS", bs[0])
	}
}

// TestOpenAIMissingKey returns a clear error before any HTTP work.
func TestOpenAIMissingKey(t *testing.T) {
	o := &OpenAITTS{Exec: newFakeExec()}
	err := o.Speak(context.Background(), "hi")
	if err == nil || !strings.Contains(err.Error(), "missing api key") {
		t.Errorf("expected missing-key error, got %v", err)
	}
}

// TestOpenAIEmptyTextNoop — symmetry with SayTTS.
func TestOpenAIEmptyTextNoop(t *testing.T) {
	fe := newFakeExec()
	o := &OpenAITTS{APIKey: "sk", Exec: fe}
	if err := o.Speak(context.Background(), ""); err != nil {
		t.Fatalf("speak: %v", err)
	}
	if len(fe.runs) != 0 {
		t.Errorf("empty input made calls: %v", fe.runs)
	}
}

// TestKokoroEmptyWavErrorIncludesFullPath asserts the empty-wav error
// returned by KokoroTTS.Speak includes the full temp-file path so the
// operator can find the missing wav (audit L5 — filepath.Base stripped
// the directory the caller needed to investigate).
func TestKokoroEmptyWavErrorIncludesFullPath(t *testing.T) {
	fe := newFakeExec()
	// kokoro returns success but never writes the wav; afplay never gets
	// reached because the size==0 check fires first.
	k := &KokoroTTS{Exec: fe}
	err := k.Speak(context.Background(), "hello")
	if err == nil {
		t.Fatal("expected empty-wav error")
	}
	if !strings.Contains(err.Error(), "empty wav") {
		t.Errorf("error missing 'empty wav': %v", err)
	}
	// Path must start with the temp dir (a directory component), not just
	// the basename.
	if !strings.Contains(err.Error(), string(os.PathSeparator)) {
		t.Errorf("error missing directory component: %v", err)
	}
}

// TestKokoroEmptyTextNoop — symmetry across backends.
func TestKokoroEmptyTextNoop(t *testing.T) {
	fe := newFakeExec()
	k := &KokoroTTS{Exec: fe}
	if err := k.Speak(context.Background(), ""); err != nil {
		t.Fatalf("speak: %v", err)
	}
	if len(fe.runs) != 0 {
		t.Errorf("empty text shelled out: %v", fe.runs)
	}
}

// slowSynthTTS sleeps in Speak to model OpenAI HTTP latency, then records
// a play() callback. Used to assert ChainTTS no longer serializes synth.
type slowSynthTTS struct {
	delay time.Duration
	play  func()
}

func (s *slowSynthTTS) Speak(ctx context.Context, text string) error {
	time.Sleep(s.delay) // allow-sleep: wall-clock fixture for parallel-synth assertion
	if s.play != nil {
		s.play()
	}
	return nil
}

// TestChainTTS_SynthParallel asserts two concurrent Speak calls finish in
// roughly one synth-delay, not two — proof the chain lock no longer wraps
// synthesis.
func TestChainTTS_SynthParallel(t *testing.T) {
	const delay = 200 * time.Millisecond
	c := NewChain(&slowSynthTTS{delay: delay})

	start := time.Now()
	var wg sync.WaitGroup
	wg.Add(2)
	for i := 0; i < 2; i++ {
		go func() {
			defer wg.Done()
			if err := c.Speak(context.Background(), "x"); err != nil {
				t.Errorf("speak: %v", err)
			}
		}()
	}
	wg.Wait()
	elapsed := time.Since(start)

	// Serial = 2*delay = 400ms; parallel ≈ delay = 200ms. 1.8x leaves
	// generous CI slack but still rejects the serial baseline.
	if elapsed >= 2*delay*9/10 {
		t.Fatalf("two parallel Speak took %v ; expected < %v (synth serialized)", elapsed, 2*delay*9/10)
	}
}

// TestChainTTS_PlaybackSerial asserts the audio device step is observed
// in serial order even when synth runs in parallel.
func TestChainTTS_PlaybackSerial(t *testing.T) {
	var active int32
	var maxActive int32

	play := func() {
		n := atomic.AddInt32(&active, 1)
		for {
			m := atomic.LoadInt32(&maxActive)
			if n <= m || atomic.CompareAndSwapInt32(&maxActive, m, n) {
				break
			}
		}
		time.Sleep(20 * time.Millisecond) // allow-sleep: hold device long enough that an overlap would be observable
		atomic.AddInt32(&active, -1)
	}

	c := NewChain(&slowSynthTTS{delay: 50 * time.Millisecond, play: play})

	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := c.Speak(context.Background(), "x"); err != nil {
				t.Errorf("speak: %v", err)
			}
		}()
	}
	wg.Wait()

	if got := atomic.LoadInt32(&maxActive); got != 1 {
		t.Fatalf("max concurrent playback = %d ; want 1 (device must serialize)", got)
	}
}

// TestChainTTS_Race exercises ChainTTS under -race; bare run, no asserts.
func TestChainTTS_Race(t *testing.T) {
	c := NewChain(&slowSynthTTS{delay: 5 * time.Millisecond})
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = c.Speak(context.Background(), "x")
		}()
	}
	wg.Wait()
}
