package facetime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"
)

// fakeAttestor records scope invocations and optionally rejects them; the
// adapter MUST route every URL launch through Attest() to gate operator consent.
type fakeAttestor struct {
	called int
	scopes []string
	err    error
}

func (f *fakeAttestor) Attest(_ context.Context, scope string) error {
	f.called++
	f.scopes = append(f.scopes, scope)
	if !strings.HasPrefix(scope, "facetime:") {
		return errors.New("unexpected scope: " + scope)
	}
	return f.err
}

// fakeOSExec captures the subprocess invocation without spawning open(1); the
// hermetic suite asserts on (name, args) instead of touching the host UI.
type fakeOSExec struct {
	called   int
	lastName string
	lastArgs []string
	err      error
	exitOut  []byte
}

func (f *fakeOSExec) Run(_ context.Context, name string, args []string, _ []byte) ([]byte, error) {
	f.called++
	f.lastName = name
	f.lastArgs = args
	return f.exitOut, f.err
}

// fakeSink captures audit rows so tests can assert hashing + mode without
// touching the real JSONL append path.
type fakeSink struct {
	rows []AuditRow
}

func (f *fakeSink) Audit(r AuditRow) { f.rows = append(f.rows, r) }

func newTestAdapter(t *testing.T, att Attestor, ex OSExec, sink Sink, now func() time.Time) *Adapter {
	t.Helper()
	a, err := New(Config{Attestor: att, OSExec: ex, Sink: sink, Now: now})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return a
}

func TestInitiateVideo_HappyPath(t *testing.T) {
	t.Parallel()
	att := &fakeAttestor{}
	ex := &fakeOSExec{}
	a := newTestAdapter(t, att, ex, &fakeSink{}, nil)
	if err := a.InitiateVideo(context.Background(), "alice@example.com"); err != nil {
		t.Fatalf("InitiateVideo: %v", err)
	}
	if ex.called != 1 || ex.lastName != "open" {
		t.Fatalf("exec=%d name=%q, want 1/open", ex.called, ex.lastName)
	}
	want := "facetime://alice@example.com"
	if len(ex.lastArgs) != 1 || ex.lastArgs[0] != want {
		t.Fatalf("args=%v, want [%q]", ex.lastArgs, want)
	}
	if att.called != 1 || att.scopes[0] != ScopeInitiateVideo {
		t.Fatalf("scope=%v, want %q invoked once", att.scopes, ScopeInitiateVideo)
	}
}

func TestInitiateAudio_UsesAudioScheme(t *testing.T) {
	t.Parallel()
	ex := &fakeOSExec{}
	a := newTestAdapter(t, &fakeAttestor{}, ex, &fakeSink{}, nil)
	if err := a.InitiateAudio(context.Background(), "+15551234567"); err != nil {
		t.Fatalf("InitiateAudio: %v", err)
	}
	if len(ex.lastArgs) != 1 || !strings.HasPrefix(ex.lastArgs[0], "facetime-audio://") {
		t.Fatalf("args=%v, want facetime-audio:// prefix", ex.lastArgs)
	}
}

func TestInitiate_AttestationDenied_NoOpen(t *testing.T) {
	t.Parallel()
	att := &fakeAttestor{err: errors.New("denied")}
	ex := &fakeOSExec{}
	a := newTestAdapter(t, att, ex, &fakeSink{}, nil)
	err := a.InitiateVideo(context.Background(), "alice@example.com")
	if !errors.Is(err, ErrAttestationDenied) {
		t.Fatalf("err=%v, want ErrAttestationDenied", err)
	}
	if ex.called != 0 {
		t.Fatalf("OSExec invoked %d times after denial — must be 0", ex.called)
	}
}

func TestInitiate_InvalidCallee_NoExec(t *testing.T) {
	t.Parallel()
	bad := []string{";rm -rf", "a&b", "$(whoami)", "`id`", "a b", "a/b", "", "a|b", "a;b", "a\"b"}
	for _, c := range bad {
		c := c
		t.Run(c, func(t *testing.T) {
			t.Parallel()
			att := &fakeAttestor{}
			ex := &fakeOSExec{}
			a := newTestAdapter(t, att, ex, &fakeSink{}, nil)
			err := a.InitiateVideo(context.Background(), c)
			if !errors.Is(err, ErrInvalidCallee) {
				t.Fatalf("callee=%q err=%v, want ErrInvalidCallee", c, err)
			}
			if att.called != 0 {
				t.Fatalf("attestor called for invalid callee %q", c)
			}
			if ex.called != 0 {
				t.Fatalf("OSExec called for invalid callee %q", c)
			}
		})
	}
}

func TestInitiate_AuditRowHashed(t *testing.T) {
	t.Parallel()
	sink := &fakeSink{}
	a := newTestAdapter(t, &fakeAttestor{}, &fakeOSExec{}, sink, nil)
	callee := "alice@example.com"
	if err := a.InitiateVideo(context.Background(), callee); err != nil {
		t.Fatalf("InitiateVideo: %v", err)
	}
	if len(sink.rows) != 1 {
		t.Fatalf("rows=%d, want 1", len(sink.rows))
	}
	row := sink.rows[0]
	sum := sha256.Sum256([]byte(callee))
	wantHash := hex.EncodeToString(sum[:])[:8]
	if row.CalleeHash != wantHash {
		t.Fatalf("CalleeHash=%q, want %q", row.CalleeHash, wantHash)
	}
	if row.Mode != "video" {
		t.Fatalf("Mode=%q, want video", row.Mode)
	}
	if !row.Success {
		t.Fatal("Success=false, want true on happy path")
	}
	if strings.Contains(row.CalleeHash, "alice") || strings.Contains(row.CalleeHash, "@") {
		t.Fatalf("plaintext callee leaked into hash: %q", row.CalleeHash)
	}
}

func TestInitiate_RateLimit_RejectsBurst(t *testing.T) {
	t.Parallel()
	base := time.Unix(1_700_000_000, 0)
	tick := base
	now := func() time.Time { return tick }
	ex := &fakeOSExec{}
	a := newTestAdapter(t, &fakeAttestor{}, ex, &fakeSink{}, now)
	for i := 0; i < 5; i++ {
		tick = base.Add(time.Duration(i) * time.Second)
		if err := a.InitiateVideo(context.Background(), "alice@example.com"); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}
	tick = base.Add(30 * time.Second)
	err := a.InitiateAudio(context.Background(), "alice@example.com")
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("6th err=%v, want ErrRateLimited", err)
	}
	if ex.called != 5 {
		t.Fatalf("OSExec called %d times, want 5 (6th must not exec)", ex.called)
	}
}

func TestNewRejectsMissingDeps(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		cfg  Config
	}{
		{name: "no attestor", cfg: Config{OSExec: &fakeOSExec{}}},
		{name: "no osexec", cfg: Config{Attestor: &fakeAttestor{}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := New(tc.cfg); err == nil {
				t.Fatal("New: expected error for missing dep")
			}
		})
	}
}
