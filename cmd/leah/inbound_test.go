package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/trilam/leah/internal/attestation"
	"github.com/trilam/leah/internal/inbound"
)

type fakeCLIAttestor struct {
	gotScope string
	deny     error
}

func (f *fakeCLIAttestor) Attest(_ context.Context, scope string) error {
	f.gotScope = scope
	return f.deny
}

// Success path: attest at ScopeInboundEnroll, then the pair lands on disk and
// a fresh FileEnrollStore reads it back. Closes the loop the daemon will
// later consume.
func TestRunInboundEnroll_PersistsAfterAttestation(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)

	att := &fakeCLIAttestor{}
	var w bytes.Buffer
	code := runInboundEnroll(context.Background(), []string{"discord", "peer-A"}, &w, att)
	if code != 0 {
		t.Fatalf("rc = %d (stderr: %s)", code, w.String())
	}
	if att.gotScope != attestation.ScopeInboundEnroll {
		t.Fatalf("attest scope = %q want %q", att.gotScope, attestation.ScopeInboundEnroll)
	}

	store, err := inbound.OpenFileEnrollStore(filepath.Join(dir, "inbound-enroll.json"))
	if err != nil {
		t.Fatalf("OpenFileEnrollStore: %v", err)
	}
	ok, err := store.Enrolled("discord", "peer-A")
	if err != nil || !ok {
		t.Fatalf("Enrolled after enroll = %v, %v want true, nil", ok, err)
	}
}

// Denial path: no enrollment file is created — fail-closed.
func TestRunInboundEnroll_DenyPreventsPersistence(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)

	att := &fakeCLIAttestor{deny: errors.New("denied")}
	var w bytes.Buffer
	code := runInboundEnroll(context.Background(), []string{"discord", "peer-A"}, &w, att)
	if code == 0 {
		t.Fatalf("rc = 0; want non-zero on denial")
	}
	if _, err := os.Stat(filepath.Join(dir, "inbound-enroll.json")); !os.IsNotExist(err) {
		t.Fatalf("enrollment file created despite denial: %v", err)
	}
}

// Missing args print usage and return 2 — same convention as connect.
func TestRunInboundEnroll_MissingArgs(t *testing.T) {
	t.Setenv("LEAH_STATE_DIR", t.TempDir())
	att := &fakeCLIAttestor{}
	var w bytes.Buffer
	if code := runInboundEnroll(context.Background(), []string{"discord"}, &w, att); code != 2 {
		t.Fatalf("rc = %d want 2", code)
	}
	if att.gotScope != "" {
		t.Fatalf("attestor invoked despite arg error: scope=%q", att.gotScope)
	}
}

// Verb dispatch: `leah inbound` (no verb) is usage; `leah inbound unknown`
// rejects with 2.
func TestRunInbound_VerbDispatch(t *testing.T) {
	t.Setenv("LEAH_STATE_DIR", t.TempDir())
	var w bytes.Buffer
	if code := runInbound(context.Background(), nil, &w); code != 2 {
		t.Fatalf("no-args rc = %d want 2", code)
	}
	if code := runInbound(context.Background(), []string{"bogus"}, &w); code != 2 {
		t.Fatalf("bogus-verb rc = %d want 2", code)
	}
}
