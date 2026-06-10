package slack

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"
)

type fakeAttestor struct {
	called int
	err    error
}

func (f *fakeAttestor) Attest(_ context.Context, scope string) error {
	f.called++
	if !strings.HasPrefix(scope, "slack:") {
		return errors.New("unexpected scope: " + scope)
	}
	return f.err
}

type fakeTokenSource struct {
	bot, user string
	botErr    error
	botCalls  int
	userCalls int
}

func (f *fakeTokenSource) BotToken(_ context.Context) (string, error) {
	f.botCalls++
	return f.bot, f.botErr
}

func (f *fakeTokenSource) UserToken(_ context.Context) (string, error) {
	f.userCalls++
	return f.user, nil
}

// failingTokenSource fails the test if any Token method is called.
type failingTokenSource struct{ t *testing.T }

func (f *failingTokenSource) BotToken(_ context.Context) (string, error) {
	f.t.Fatal("BotToken must NOT be called when attestation denies / validation fails")
	return "", nil
}
func (f *failingTokenSource) UserToken(_ context.Context) (string, error) {
	f.t.Fatal("UserToken must NOT be called when attestation denies / validation fails")
	return "", nil
}

type fakeTransport struct {
	channels       []Channel
	channelsErr    error
	postErr        error
	thread         Thread
	threadErr      error
	results        []Result
	searchErr      error
	postCalls      int
	searchCalls    int
	lastPostChan   string
	lastPostText   string
	lastSearchUser string
}

func (f *fakeTransport) ListChannels(_ context.Context, _ string) ([]Channel, error) {
	return f.channels, f.channelsErr
}

func (f *fakeTransport) PostMessage(_ context.Context, _, channel, text string) error {
	f.postCalls++
	f.lastPostChan = channel
	f.lastPostText = text
	return f.postErr
}

func (f *fakeTransport) GetThread(_ context.Context, _, _, _ string) (Thread, error) {
	return f.thread, f.threadErr
}

func (f *fakeTransport) Search(_ context.Context, user, _ string) ([]Result, error) {
	f.searchCalls++
	f.lastSearchUser = user
	return f.results, f.searchErr
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

func newTestClient(t *testing.T, att Attestor, ts TokenSource, tr Transport, sink AuditSink, now func() time.Time) *Adapter {
	t.Helper()
	a, err := New(Config{Attestor: att, TokenSource: ts, Transport: tr, Audit: sink, Now: now})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return a
}

func TestListChannels_HappyPath(t *testing.T) {
	t.Parallel()
	att := &fakeAttestor{}
	ts := &fakeTokenSource{bot: "xoxb-tok"}
	want := []Channel{{ID: "C1", Name: "general"}, {ID: "D1", Name: "", IsIM: true}}
	tr := &fakeTransport{channels: want}
	c := newTestClient(t, att, ts, tr, nil, nil)

	got, err := c.ListChannels(context.Background())
	if err != nil {
		t.Fatalf("ListChannels: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("ch[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
	if att.called == 0 {
		t.Fatal("attestor never invoked")
	}
}

func TestListChannels_AttestationDenied(t *testing.T) {
	t.Parallel()
	att := &fakeAttestor{err: errors.New("denied")}
	c := newTestClient(t, att, &failingTokenSource{t: t}, &fakeTransport{}, nil, nil)

	_, err := c.ListChannels(context.Background())
	if !errors.Is(err, ErrAttestationDenied) {
		t.Fatalf("err = %v, want ErrAttestationDenied", err)
	}
}

func TestPostMessage_AttestationDenied_NoCall(t *testing.T) {
	t.Parallel()
	att := &fakeAttestor{err: errors.New("denied")}
	tr := &fakeTransport{}
	c := newTestClient(t, att, &failingTokenSource{t: t}, tr, nil, nil)

	err := c.PostMessage(context.Background(), "C1", "hi")
	if !errors.Is(err, ErrAttestationDenied) {
		t.Fatalf("err = %v, want ErrAttestationDenied", err)
	}
	if tr.postCalls != 0 {
		t.Fatalf("transport.PostMessage called %d times after denial", tr.postCalls)
	}
}

func TestPostMessage_AuditRowHashesChannel(t *testing.T) {
	t.Parallel()
	sink := &recordingSink{}
	c := newTestClient(t, &fakeAttestor{}, &fakeTokenSource{bot: "t"}, &fakeTransport{}, sink, nil)

	const channel = "C0123456789"
	const text = "hello world"
	if err := c.PostMessage(context.Background(), channel, text); err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	rows := sink.snapshot()
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	r := rows[0]
	if r.Kind != "slack_post_message" {
		t.Errorf("Kind = %q, want slack_post_message", r.Kind)
	}
	if !r.Success {
		t.Error("Success = false, want true")
	}
	sum := sha256.Sum256([]byte(channel))
	want := hex.EncodeToString(sum[:])[:8]
	if r.ChannelHash != want {
		t.Errorf("ChannelHash = %q, want %q", r.ChannelHash, want)
	}
	if r.TextLen != utf8.RuneCountInString(text) {
		t.Errorf("TextLen = %d, want %d", r.TextLen, utf8.RuneCountInString(text))
	}
	// plaintext channel must not appear in any audit field
	for _, f := range []string{r.Kind, r.ChannelHash, r.Reason} {
		if strings.Contains(f, channel) {
			t.Errorf("plaintext channel leaked in audit field: %q", f)
		}
	}
}

func TestPostMessage_InvalidChannel_NoAttest(t *testing.T) {
	t.Parallel()
	att := &fakeAttestor{}
	c := newTestClient(t, att, &failingTokenSource{t: t}, &fakeTransport{}, nil, nil)

	if err := c.PostMessage(context.Background(), "", "hi"); !errors.Is(err, ErrInvalidChannel) {
		t.Fatalf("empty channel: err = %v, want ErrInvalidChannel", err)
	}
	if err := c.PostMessage(context.Background(), "C1", ""); !errors.Is(err, ErrInvalidChannel) {
		t.Fatalf("empty text: err = %v, want ErrInvalidChannel", err)
	}
	if att.called != 0 {
		t.Fatalf("attestor invoked %d times on invalid input", att.called)
	}
}

func TestGetThread_NotFound(t *testing.T) {
	t.Parallel()
	tr := &fakeTransport{threadErr: ErrNotFound}
	c := newTestClient(t, &fakeAttestor{}, &fakeTokenSource{bot: "t"}, tr, nil, nil)

	_, err := c.GetThread(context.Background(), "C1", "1234.5678")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestSearch_HappyPath(t *testing.T) {
	t.Parallel()
	ts := &fakeTokenSource{user: "xoxp-tok"}
	want := []Result{{Channel: "C1", TS: "1.0", Snippet: "hit", URL: "https://x"}}
	tr := &fakeTransport{results: want}
	c := newTestClient(t, &fakeAttestor{}, ts, tr, nil, nil)

	got, err := c.Search(context.Background(), "needle")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(got) != 1 || got[0] != want[0] {
		t.Fatalf("results = %+v, want %+v", got, want)
	}
	if tr.lastSearchUser != "xoxp-tok" {
		t.Fatalf("transport saw user token %q, want xoxp-tok", tr.lastSearchUser)
	}
	if ts.botCalls != 0 {
		t.Fatalf("Search loaded BotToken %d times; must be UserToken only", ts.botCalls)
	}
}

func TestSearch_NoUserToken_ErrAuthFailed(t *testing.T) {
	t.Parallel()
	ts := &fakeTokenSource{user: ""}
	tr := &fakeTransport{}
	c := newTestClient(t, &fakeAttestor{}, ts, tr, nil, nil)

	_, err := c.Search(context.Background(), "needle")
	if !errors.Is(err, ErrAuthFailed) {
		t.Fatalf("err = %v, want ErrAuthFailed", err)
	}
	if tr.searchCalls != 0 {
		t.Fatalf("transport.Search called %d times; must short-circuit", tr.searchCalls)
	}
}

func TestPostMessage_RateLimit_RejectsBurst(t *testing.T) {
	t.Parallel()
	now := time.Unix(1_700_000_000, 0)
	clock := &now
	c := newTestClient(t, &fakeAttestor{}, &fakeTokenSource{bot: "t"}, &fakeTransport{}, nil, func() time.Time { return *clock })

	if err := c.PostMessage(context.Background(), "C1", "a"); err != nil {
		t.Fatalf("first send: %v", err)
	}
	if err := c.PostMessage(context.Background(), "C1", "b"); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("burst: err = %v, want ErrRateLimited", err)
	}
	// other channel unaffected
	if err := c.PostMessage(context.Background(), "C2", "c"); err != nil {
		t.Fatalf("other channel: %v", err)
	}
	// advance past window
	*clock = now.Add(2 * time.Second)
	if err := c.PostMessage(context.Background(), "C1", "d"); err != nil {
		t.Fatalf("after window: %v", err)
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
