package voice

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
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

// TestPickBackendsFullChain asserts ordering: Kokoro, OpenAI, Say.
func TestPickBackendsFullChain(t *testing.T) {
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
