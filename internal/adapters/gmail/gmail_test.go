package gmail

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// fakeAttestor records whether the attestation gate was invoked. The Gmail
// adapter MUST route every token load through Attest() so the operator-
// attestation flow (see internal/dispatcher/selfbuild.go) gates secret access.
type fakeAttestor struct {
	called int
	err    error
}

func (f *fakeAttestor) Attest(_ context.Context, scope string) error {
	f.called++
	if !strings.HasPrefix(scope, "gmail:") {
		return errors.New("unexpected scope: " + scope)
	}
	return f.err
}

// fakeTokenSource returns a canned token without touching disk; lets tests run
// hermetically without $HOME/.leah-state/secrets/gmail-token.json.
type fakeTokenSource struct {
	tok string
	err error
}

func (f *fakeTokenSource) Token(_ context.Context) (string, error) {
	return f.tok, f.err
}

// fakeTransport is a minimal stand-in for the gmail/v1 HTTP layer; the adapter
// MVP routes RPCs through a swappable Transport so tests don't hit the real API.
type fakeTransport struct {
	listIDs    []string
	listErr    error
	markErr    error
	sendErr    error
	lastSendTo string
}

func (f *fakeTransport) ListUnread(_ context.Context, _ string) ([]string, error) {
	return f.listIDs, f.listErr
}

func (f *fakeTransport) MarkRead(_ context.Context, _, id string) error {
	if id == "" {
		return ErrMessageNotFound
	}
	return f.markErr
}

func (f *fakeTransport) Send(_ context.Context, _ string, msg Message) error {
	f.lastSendTo = msg.To
	return f.sendErr
}

func newTestClient(t *testing.T, att *fakeAttestor, ts *fakeTokenSource, tr *fakeTransport) *Client {
	t.Helper()
	c, err := New(Config{
		Attestor:    att,
		TokenSource: ts,
		Transport:   tr,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

func TestListUnread(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		att     *fakeAttestor
		ts      *fakeTokenSource
		tr      *fakeTransport
		wantN   int
		wantErr error
	}{
		{
			name:  "happy: returns ids",
			att:   &fakeAttestor{},
			ts:    &fakeTokenSource{tok: "t"},
			tr:    &fakeTransport{listIDs: []string{"a", "b", "c"}},
			wantN: 3,
		},
		{
			name:    "auth failure: attestor rejects",
			att:     &fakeAttestor{err: errors.New("denied")},
			ts:      &fakeTokenSource{tok: "t"},
			tr:      &fakeTransport{},
			wantErr: ErrAttestationDenied,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := newTestClient(t, tc.att, tc.ts, tc.tr)
			ids, err := c.ListUnread(context.Background())
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ListUnread: %v", err)
			}
			if len(ids) != tc.wantN {
				t.Fatalf("len(ids) = %d, want %d", len(ids), tc.wantN)
			}
			if tc.att.called == 0 {
				t.Fatal("attestor never invoked — token load bypassed the gate")
			}
		})
	}
}

func TestMarkRead(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		id      string
		tr      *fakeTransport
		wantErr error
	}{
		{name: "happy", id: "msg-1", tr: &fakeTransport{}},
		{name: "nonexistent message", id: "", tr: &fakeTransport{}, wantErr: ErrMessageNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := newTestClient(t, &fakeAttestor{}, &fakeTokenSource{tok: "t"}, tc.tr)
			err := c.MarkRead(context.Background(), tc.id)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("MarkRead: %v", err)
			}
		})
	}
}

func TestSend(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		msg     Message
		tr      *fakeTransport
		wantErr error
	}{
		{
			name: "happy",
			msg:  Message{To: "ops@example.com", Subject: "hi", Body: "body"},
			tr:   &fakeTransport{},
		},
		{
			name:    "send rejected: missing recipient",
			msg:     Message{Subject: "hi", Body: "body"},
			tr:      &fakeTransport{},
			wantErr: ErrSendRejected,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := newTestClient(t, &fakeAttestor{}, &fakeTokenSource{tok: "t"}, tc.tr)
			err := c.Send(context.Background(), tc.msg)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Send: %v", err)
			}
			if tc.tr.lastSendTo != tc.msg.To {
				t.Fatalf("transport saw To=%q, want %q", tc.tr.lastSendTo, tc.msg.To)
			}
		})
	}
}

func TestNewRejectsMissingDeps(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		cfg  Config
	}{
		{name: "no attestor", cfg: Config{TokenSource: &fakeTokenSource{}, Transport: &fakeTransport{}}},
		{name: "no token source", cfg: Config{Attestor: &fakeAttestor{}, Transport: &fakeTransport{}}},
		{name: "no transport", cfg: Config{Attestor: &fakeAttestor{}, TokenSource: &fakeTokenSource{}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := New(tc.cfg); err == nil {
				t.Fatal("New: expected error for missing dep")
			}
		})
	}
}
