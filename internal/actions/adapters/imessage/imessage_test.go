package imessage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/trilam/leah/internal/platform/contracts"
)

// fakeAttestor counts invocations and returns a canned error.
type fakeAttestor struct {
	called int
	err    error
}

func (f *fakeAttestor) Attest(_ context.Context, scope string) error {
	f.called++
	if !strings.HasPrefix(scope, "imessage:") {
		return errors.New("unexpected scope: " + scope)
	}
	return f.err
}

// fakeOSExec captures invocations and returns canned (stdout, err).
type fakeOSExec struct {
	called    int
	lastName  string
	lastArgs  []string
	lastStdin []byte
	stdout    []byte
	err       error
}

func (f *fakeOSExec) Run(_ context.Context, name string, args []string, stdin []byte) ([]byte, error) {
	f.called++
	f.lastName = name
	f.lastArgs = args
	f.lastStdin = stdin
	return f.stdout, f.err
}

// recordingSink captures emitted audit rows.
type recordingSink struct {
	rows []AuditRow
}

func (r *recordingSink) Record(row AuditRow) { r.rows = append(r.rows, row) }

func newAdapter(t *testing.T, att contracts.Attestor, ex OSExec, sink AuditSink, now func() time.Time) *Adapter {
	t.Helper()
	a, err := New(Config{Attestor: att, OSExec: ex, Audit: sink, Now: now})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return a
}

// Happy path: subprocess invoked with osascript, stdin carries recipient + body.
func TestSend_HappyPath(t *testing.T) {
	t.Parallel()
	att := &fakeAttestor{}
	ex := &fakeOSExec{}
	a := newAdapter(t, att, ex, &recordingSink{}, nil)
	if err := a.Send(context.Background(), Message{To: "+15551234567", Body: "hello there"}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if ex.called != 1 {
		t.Fatalf("osexec called = %d, want 1", ex.called)
	}
	if ex.lastName != "osascript" {
		t.Fatalf("exec name = %q, want osascript", ex.lastName)
	}
	hasDash := false
	for _, a := range ex.lastArgs {
		if a == "-" {
			hasDash = true
		}
	}
	if !hasDash {
		t.Fatalf("args missing %q to read script from stdin: %v", "-", ex.lastArgs)
	}
	stdin := string(ex.lastStdin)
	if !strings.Contains(stdin, "+15551234567") {
		t.Fatalf("stdin missing recipient: %q", stdin)
	}
	if !strings.Contains(stdin, "hello there") {
		t.Fatalf("stdin missing body: %q", stdin)
	}
	if !strings.Contains(stdin, "Messages") {
		t.Fatalf("stdin not a Messages AppleScript: %q", stdin)
	}
}

// Attestation denial MUST short-circuit before any subprocess spawn.
func TestSend_AttestationDenied_NoExec(t *testing.T) {
	t.Parallel()
	att := &fakeAttestor{err: errors.New("operator denied")}
	ex := &fakeOSExec{}
	a := newAdapter(t, att, ex, &recordingSink{}, nil)
	err := a.Send(context.Background(), Message{To: "+15551234567", Body: "x"})
	if !errors.Is(err, ErrAttestationDenied) {
		t.Fatalf("err = %v, want ErrAttestationDenied", err)
	}
	if ex.called != 0 {
		t.Fatalf("osexec invoked despite denial: called=%d", ex.called)
	}
}

// osascript's "not authorized" stderr signal maps to ErrPermissionDenied.
func TestSend_PermissionDenied_PassThrough(t *testing.T) {
	t.Parallel()
	ex := &fakeOSExec{err: errors.New("exit 1: not authorized to send Apple events to Messages")}
	a := newAdapter(t, &fakeAttestor{}, ex, &recordingSink{}, nil)
	err := a.Send(context.Background(), Message{To: "+15551234567", Body: "x"})
	if !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("err = %v, want ErrPermissionDenied", err)
	}
}

// Empty recipient: reject before attestation and before exec.
func TestSend_RecipientEmpty_NoExec(t *testing.T) {
	t.Parallel()
	att := &fakeAttestor{}
	ex := &fakeOSExec{}
	a := newAdapter(t, att, ex, &recordingSink{}, nil)
	err := a.Send(context.Background(), Message{To: "", Body: "x"})
	if !errors.Is(err, ErrInvalidRecipient) {
		t.Fatalf("err = %v, want ErrInvalidRecipient", err)
	}
	if att.called != 0 || ex.called != 0 {
		t.Fatalf("gate/exec ran for empty recipient: att=%d ex=%d", att.called, ex.called)
	}
}

// Shell-special recipient fails regex; no attestation, no exec.
func TestSend_RecipientShellSpecials_Rejected(t *testing.T) {
	t.Parallel()
	att := &fakeAttestor{}
	ex := &fakeOSExec{}
	a := newAdapter(t, att, ex, &recordingSink{}, nil)
	err := a.Send(context.Background(), Message{To: "$(rm -rf ~)", Body: "x"})
	if !errors.Is(err, ErrInvalidRecipient) {
		t.Fatalf("err = %v, want ErrInvalidRecipient", err)
	}
	if att.called != 0 || ex.called != 0 {
		t.Fatalf("gate/exec ran for invalid recipient: att=%d ex=%d", att.called, ex.called)
	}
}

// Body quotes and backslashes are escaped into the AppleScript string literal.
func TestSend_BodyWithQuotesAndBackslashes_Escaped(t *testing.T) {
	t.Parallel()
	ex := &fakeOSExec{}
	a := newAdapter(t, &fakeAttestor{}, ex, &recordingSink{}, nil)
	body := `He said "hi" \ bye`
	if err := a.Send(context.Background(), Message{To: "+15551234567", Body: body}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	stdin := string(ex.lastStdin)
	if !strings.Contains(stdin, `\"hi\"`) {
		t.Fatalf("stdin missing escaped quotes: %q", stdin)
	}
	if !strings.Contains(stdin, `\\`) {
		t.Fatalf("stdin missing escaped backslash: %q", stdin)
	}
	if strings.Contains(stdin, `"hi"`) && !strings.Contains(stdin, `\"hi\"`) {
		t.Fatalf("raw unescaped quotes leaked into script: %q", stdin)
	}
}

// Audit row stores sha256(to)[:8], never the plaintext recipient or body bytes.
func TestSend_AuditRowHashed(t *testing.T) {
	t.Parallel()
	sink := &recordingSink{}
	to := "+15551234567"
	body := "hello world"
	a := newAdapter(t, &fakeAttestor{}, &fakeOSExec{}, sink, nil)
	if err := a.Send(context.Background(), Message{To: to, Body: body}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if len(sink.rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(sink.rows))
	}
	row := sink.rows[0]
	sum := sha256.Sum256([]byte(to))
	want := hex.EncodeToString(sum[:])[:8]
	if row.RecipientHash != want {
		t.Fatalf("recipient_hash = %q, want %q", row.RecipientHash, want)
	}
	if strings.Contains(row.RecipientHash, to) || row.Reason == to {
		t.Fatalf("plaintext recipient leaked into audit row: %+v", row)
	}
	if row.BodyLen != utf8.RuneCountInString(body) {
		t.Fatalf("body_len = %d, want %d", row.BodyLen, utf8.RuneCountInString(body))
	}
	if !row.Success {
		t.Fatalf("row.Success = false, want true on happy path")
	}
}

// Rolling window: 10 sends in 30s succeed; 11th rejects; window advance heals.
func TestSend_RateLimit_RejectsBurst(t *testing.T) {
	t.Parallel()
	base := time.Unix(1_700_000_000, 0)
	clock := base
	now := func() time.Time { return clock }
	att := &fakeAttestor{}
	ex := &fakeOSExec{}
	a := newAdapter(t, att, ex, &recordingSink{}, now)
	for i := 0; i < 10; i++ {
		if err := a.Send(context.Background(), Message{To: "+15551234567", Body: "x"}); err != nil {
			t.Fatalf("send %d: %v", i, err)
		}
		clock = clock.Add(3 * time.Second) // 10 sends spanning 27s
	}
	// 11th still inside the rolling 60s window
	attBefore, exBefore := att.called, ex.called
	err := a.Send(context.Background(), Message{To: "+15551234567", Body: "x"})
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("11th err = %v, want ErrRateLimited", err)
	}
	if att.called != attBefore {
		t.Fatalf("attestor called on rate-limited send: before=%d after=%d", attBefore, att.called)
	}
	if ex.called != exBefore {
		t.Fatalf("osexec called on rate-limited send: before=%d after=%d", exBefore, ex.called)
	}
	// Advance past the window: oldest send recorded at base+0; need clock > base+60s
	clock = base.Add(61 * time.Second)
	if err := a.Send(context.Background(), Message{To: "+15551234567", Body: "x"}); err != nil {
		t.Fatalf("send after window: %v", err)
	}
}

// Fail-closed: nil Attestor and nil OSExec each reject construction.
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
