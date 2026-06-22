package higgsfield

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
	"testing"
)

type fakeAttestor struct {
	called int
	err    error
}

func (f *fakeAttestor) Attest(_ context.Context, scope string) error {
	f.called++
	if !strings.HasPrefix(scope, "higgsfield:") {
		return errors.New("unexpected scope: " + scope)
	}
	return f.err
}

type fakeTokenSource struct {
	key    string
	err    error
	calls  int
}

func (f *fakeTokenSource) Token(_ context.Context) (string, error) {
	f.calls++
	return f.key, f.err
}

type failingTokenSource struct{ t *testing.T }

func (f *failingTokenSource) Token(_ context.Context) (string, error) {
	f.t.Fatal("Token must NOT be called when attestation denies / validation fails")
	return "", nil
}

type fakeTransport struct {
	imgBytes   []byte
	imgMime    string
	imgErr     error
	jobID      string
	submitErr  error
	pollStatus string
	pollMP4    []byte
	pollErr    error

	imgCalls     int
	submitCalls  int
	pollCalls    int
	lastImgKey   string
	lastImgPrompt string
	lastSubmitSecs int
}

func (f *fakeTransport) GenerateImage(_ context.Context, key, prompt string) ([]byte, string, error) {
	f.imgCalls++
	f.lastImgKey = key
	f.lastImgPrompt = prompt
	return f.imgBytes, f.imgMime, f.imgErr
}

func (f *fakeTransport) SubmitClip(_ context.Context, _ string, _ string, seconds int) (string, error) {
	f.submitCalls++
	f.lastSubmitSecs = seconds
	return f.jobID, f.submitErr
}

func (f *fakeTransport) PollClip(_ context.Context, _ string, _ string) (string, []byte, error) {
	f.pollCalls++
	return f.pollStatus, f.pollMP4, f.pollErr
}

type recordingSink struct {
	mu   sync.Mutex
	rows []AuditRow
}

func (s *recordingSink) Record(r AuditRow) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rows = append(s.rows, r)
}

func (s *recordingSink) snapshot() []AuditRow {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]AuditRow, len(s.rows))
	copy(out, s.rows)
	return out
}

func newTestAdapter(t *testing.T, att Attestor, ts TokenSource, tr Transport, sink AuditSink) *Adapter {
	t.Helper()
	a, err := New(Config{Attestor: att, TokenSource: ts, Transport: tr, Audit: sink})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return a
}

func TestGenerateImage_HappyPath(t *testing.T) {
	t.Parallel()
	att := &fakeAttestor{}
	ts := &fakeTokenSource{key: "hf-key"}
	tr := &fakeTransport{imgBytes: []byte{0x89, 'P', 'N', 'G'}, imgMime: "image/png"}
	a := newTestAdapter(t, att, ts, tr, nil)

	const prompt = "a sleek cyberpunk skyline"
	b, mime, err := a.GenerateImage(context.Background(), prompt)
	if err != nil {
		t.Fatalf("GenerateImage: %v", err)
	}
	if mime != "image/png" || len(b) != 4 {
		t.Fatalf("got mime=%q len=%d", mime, len(b))
	}
	if tr.lastImgKey != "hf-key" {
		t.Fatalf("transport saw key %q, want hf-key", tr.lastImgKey)
	}
	if tr.lastImgPrompt != prompt {
		t.Fatalf("prompt = %q, want %q", tr.lastImgPrompt, prompt)
	}
	if att.called == 0 {
		t.Fatal("attestor never invoked")
	}
}

func TestGenerateImage_AttestationDenied_NoCall(t *testing.T) {
	t.Parallel()
	att := &fakeAttestor{err: errors.New("denied")}
	tr := &fakeTransport{}
	a := newTestAdapter(t, att, &failingTokenSource{t: t}, tr, nil)

	_, _, err := a.GenerateImage(context.Background(), "x")
	if !errors.Is(err, ErrAttestationDenied) {
		t.Fatalf("err = %v, want ErrAttestationDenied", err)
	}
	if tr.imgCalls != 0 {
		t.Fatalf("transport.GenerateImage called %d times after denial", tr.imgCalls)
	}
}

func TestGenerateImage_InvalidPrompt_NoAttest(t *testing.T) {
	t.Parallel()
	att := &fakeAttestor{}
	a := newTestAdapter(t, att, &failingTokenSource{t: t}, &fakeTransport{}, nil)

	if _, _, err := a.GenerateImage(context.Background(), ""); !errors.Is(err, ErrInvalidPrompt) {
		t.Fatalf("empty prompt: err = %v, want ErrInvalidPrompt", err)
	}
	if att.called != 0 {
		t.Fatalf("attestor invoked %d times on invalid input", att.called)
	}
}

func TestGenerateImage_AuditRowHashesPrompt(t *testing.T) {
	t.Parallel()
	sink := &recordingSink{}
	a := newTestAdapter(t, &fakeAttestor{}, &fakeTokenSource{key: "k"}, &fakeTransport{imgBytes: []byte{0}, imgMime: "image/png"}, sink)

	const prompt = "secret-internal-codename-thunderclap"
	if _, _, err := a.GenerateImage(context.Background(), prompt); err != nil {
		t.Fatalf("GenerateImage: %v", err)
	}
	rows := sink.snapshot()
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	r := rows[0]
	if r.Kind != "higgsfield_generate_image" {
		t.Errorf("Kind = %q", r.Kind)
	}
	if !r.Success {
		t.Error("Success = false, want true")
	}
	sum := sha256.Sum256([]byte(prompt))
	want := hex.EncodeToString(sum[:])[:8]
	if r.PromptHash != want {
		t.Errorf("PromptHash = %q, want %q", r.PromptHash, want)
	}
	for _, f := range []string{r.Kind, r.PromptHash, r.Reason} {
		if strings.Contains(f, prompt) {
			t.Errorf("plaintext prompt leaked in audit field: %q", f)
		}
	}
}

func TestGenerateImage_RateLimited_AuditRecords(t *testing.T) {
	t.Parallel()
	sink := &recordingSink{}
	tr := &fakeTransport{imgErr: ErrRateLimited}
	a := newTestAdapter(t, &fakeAttestor{}, &fakeTokenSource{key: "k"}, tr, sink)

	_, _, err := a.GenerateImage(context.Background(), "p")
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("err = %v, want ErrRateLimited", err)
	}
	rows := sink.snapshot()
	if len(rows) != 1 || rows[0].Success {
		t.Fatalf("expected one failure row, got %+v", rows)
	}
}

func TestSubmitClip_HappyPath(t *testing.T) {
	t.Parallel()
	tr := &fakeTransport{jobID: "job-42"}
	a := newTestAdapter(t, &fakeAttestor{}, &fakeTokenSource{key: "k"}, tr, nil)

	id, err := a.SubmitClip(context.Background(), "a serene mountain", 8)
	if err != nil {
		t.Fatalf("SubmitClip: %v", err)
	}
	if id != "job-42" {
		t.Fatalf("jobID = %q, want job-42", id)
	}
	if tr.lastSubmitSecs != 8 {
		t.Fatalf("seconds = %d, want 8", tr.lastSubmitSecs)
	}
}

func TestSubmitClip_PolicyRefused(t *testing.T) {
	t.Parallel()
	tr := &fakeTransport{submitErr: ErrPolicyRefused}
	a := newTestAdapter(t, &fakeAttestor{}, &fakeTokenSource{key: "k"}, tr, nil)
	_, err := a.SubmitClip(context.Background(), "p", 8)
	if !errors.Is(err, ErrPolicyRefused) {
		t.Fatalf("err = %v, want ErrPolicyRefused", err)
	}
}

func TestSubmitClip_InvalidPrompt(t *testing.T) {
	t.Parallel()
	a := newTestAdapter(t, &fakeAttestor{}, &failingTokenSource{t: t}, &fakeTransport{}, nil)
	if _, err := a.SubmitClip(context.Background(), "", 8); !errors.Is(err, ErrInvalidPrompt) {
		t.Fatalf("err = %v, want ErrInvalidPrompt", err)
	}
}

func TestSubmitClip_InvalidSeconds(t *testing.T) {
	t.Parallel()
	a := newTestAdapter(t, &fakeAttestor{}, &failingTokenSource{t: t}, &fakeTransport{}, nil)
	if _, err := a.SubmitClip(context.Background(), "p", 0); !errors.Is(err, ErrInvalidPrompt) {
		t.Fatalf("0 sec: err = %v, want ErrInvalidPrompt", err)
	}
	if _, err := a.SubmitClip(context.Background(), "p", 999); !errors.Is(err, ErrInvalidPrompt) {
		t.Fatalf("999 sec: err = %v, want ErrInvalidPrompt", err)
	}
}

func TestPollClip_HappyPath(t *testing.T) {
	t.Parallel()
	tr := &fakeTransport{pollStatus: "ready", pollMP4: []byte("mp4-bytes")}
	a := newTestAdapter(t, &fakeAttestor{}, &fakeTokenSource{key: "k"}, tr, nil)

	status, mp4, err := a.PollClip(context.Background(), "job-42")
	if err != nil {
		t.Fatalf("PollClip: %v", err)
	}
	if status != "ready" || string(mp4) != "mp4-bytes" {
		t.Fatalf("status=%q mp4=%q", status, mp4)
	}
}

func TestPollClip_InvalidJobID(t *testing.T) {
	t.Parallel()
	a := newTestAdapter(t, &fakeAttestor{}, &failingTokenSource{t: t}, &fakeTransport{}, nil)
	if _, _, err := a.PollClip(context.Background(), ""); !errors.Is(err, ErrInvalidPrompt) {
		t.Fatalf("err = %v, want ErrInvalidPrompt", err)
	}
}

func TestPollClip_AuthFailed(t *testing.T) {
	t.Parallel()
	tr := &fakeTransport{pollErr: ErrAuthFailed}
	a := newTestAdapter(t, &fakeAttestor{}, &fakeTokenSource{key: "k"}, tr, nil)
	if _, _, err := a.PollClip(context.Background(), "job-42"); !errors.Is(err, ErrAuthFailed) {
		t.Fatalf("err = %v, want ErrAuthFailed", err)
	}
}

func TestNewRejectsMissingDeps(t *testing.T) {
	t.Parallel()
	att := &fakeAttestor{}
	ts := &fakeTokenSource{}
	tr := &fakeTransport{}
	cases := []struct {
		name string
		cfg  Config
	}{
		{name: "no attestor", cfg: Config{TokenSource: ts, Transport: tr}},
		{name: "no token source", cfg: Config{Attestor: att, Transport: tr}},
		{name: "no transport", cfg: Config{Attestor: att, TokenSource: ts}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := New(tc.cfg); err == nil {
				t.Fatal("New: expected error for missing dep")
			}
		})
	}
}
