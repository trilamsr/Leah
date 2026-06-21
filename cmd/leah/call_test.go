package main

import (
	"context"
	"errors"
	"testing"
)

type fakeNativeExec struct {
	calls    int
	lastName string
	lastArgs []string
}

func (f *fakeNativeExec) Run(_ context.Context, name string, args []string, _ []byte) ([]byte, error) {
	f.calls++
	f.lastName = name
	f.lastArgs = args
	return nil, nil
}

type stubAttestor struct {
	gotScope string
	err      error
}

func (s *stubAttestor) Attest(_ context.Context, scope string) error {
	s.gotScope = scope
	return s.err
}

func TestRunCall_VideoDefault_BuildsOpenCommand(t *testing.T) {
	ex := &fakeNativeExec{}
	att := &stubAttestor{}
	if code := runCallWith(context.Background(), att, ex, "+15551234567", false); code != 0 {
		t.Fatalf("exit=%d, want 0", code)
	}
	if att.gotScope != scopeCall {
		t.Fatalf("scope=%q, want %q", att.gotScope, scopeCall)
	}
	if ex.calls != 1 {
		t.Fatalf("exec calls=%d, want 1", ex.calls)
	}
	if ex.lastName != "open" {
		t.Fatalf("exec name=%q, want open", ex.lastName)
	}
	if len(ex.lastArgs) != 1 || ex.lastArgs[0] != "facetime://+15551234567" {
		t.Fatalf("args=%v, want facetime:// video scheme", ex.lastArgs)
	}
}

func TestRunCall_AudioFlag_BuildsAudioScheme(t *testing.T) {
	ex := &fakeNativeExec{}
	if code := runCallWith(context.Background(), &stubAttestor{}, ex, "alice@example.com", true); code != 0 {
		t.Fatalf("exit=%d, want 0", code)
	}
	if len(ex.lastArgs) != 1 || ex.lastArgs[0] != "facetime-audio://alice@example.com" {
		t.Fatalf("args=%v, want facetime-audio:// scheme", ex.lastArgs)
	}
}

func TestRunCall_AttestDenied_NoExec(t *testing.T) {
	ex := &fakeNativeExec{}
	att := &stubAttestor{err: errors.New("denied")}
	if code := runCallWith(context.Background(), att, ex, "+15551234567", false); code == 0 {
		t.Fatal("exit=0 on denied attestation, want non-zero")
	}
	if ex.calls != 0 {
		t.Fatalf("exec invoked despite denial: calls=%d", ex.calls)
	}
}

func TestRunShipIMessage_BuildsSendAfterAttest(t *testing.T) {
	ex := &fakeNativeExec{}
	att := &stubAttestor{}
	if code := runShipIMessageWith(context.Background(), att, ex, "+15551234567", "hello there"); code != 0 {
		t.Fatalf("exit=%d, want 0", code)
	}
	if att.gotScope != scopeShipIMessage {
		t.Fatalf("scope=%q, want %q", att.gotScope, scopeShipIMessage)
	}
	if ex.calls != 1 || ex.lastName != "osascript" {
		t.Fatalf("exec name=%q calls=%d, want osascript x1", ex.lastName, ex.calls)
	}
}

func TestRunShipIMessage_AttestDenied_NoExec(t *testing.T) {
	ex := &fakeNativeExec{}
	att := &stubAttestor{err: errors.New("denied")}
	if code := runShipIMessageWith(context.Background(), att, ex, "+15551234567", "hi"); code == 0 {
		t.Fatal("exit=0 on denied attestation, want non-zero")
	}
	if ex.calls != 0 {
		t.Fatalf("exec invoked despite denial: calls=%d", ex.calls)
	}
}

func TestScopesDistinct(t *testing.T) {
	if scopeCall == scopeShipIMessage {
		t.Fatal("call and ship-imessage scopes must be distinct (anti-habituation)")
	}
}
