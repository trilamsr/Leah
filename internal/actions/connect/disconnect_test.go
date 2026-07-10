package connect

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/oauth2"
)

type fakeProvider struct {
	name      string
	tokenPath string
}

func (f *fakeProvider) Name() string      { return f.name }
func (f *fakeProvider) Scopes() []string  { return nil }
func (f *fakeProvider) TokenPath() string { return f.tokenPath }
func (f *fakeProvider) authorize(_ context.Context, _ PromptFn) (*oauth2.Token, error) {
	return nil, nil
}

type revokingProvider struct {
	*fakeProvider
	revoked   int
	revokeErr error
}

// Revoke is the optional best-effort revoke hook detected via interface assertion.
func (r *revokingProvider) Revoke(_ context.Context) error {
	r.revoked++
	return r.revokeErr
}

// TestDisconnect_HappyPath verifies attest → revoke → remove ordering + file deletion.
func TestDisconnect_HappyPath(t *testing.T) {
	dir := t.TempDir()
	tokenPath := filepath.Join(dir, "gmail-token.json")
	if err := os.WriteFile(tokenPath, []byte(`{"access_token":"x"}`), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	fp := &fakeProvider{name: "gmail", tokenPath: tokenPath}
	rp := &revokingProvider{fakeProvider: fp}
	att := &fakeAttestor{}

	if err := Disconnect(context.Background(), rp, att); err != nil {
		t.Fatalf("Disconnect: %v", err)
	}
	if len(att.calls) != 1 || att.calls[0] != "disconnect:gmail" {
		t.Fatalf("attestor calls: %v", att.calls)
	}
	if rp.revoked != 1 {
		t.Fatalf("revoke calls: %d, want 1", rp.revoked)
	}
	if _, err := os.Stat(tokenPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("token file still present: %v", err)
	}
}

// TestDisconnect_AttestationDenied_KeepsTokenFile pins the gate: denied attest leaves the token intact.
func TestDisconnect_AttestationDenied_KeepsTokenFile(t *testing.T) {
	dir := t.TempDir()
	tokenPath := filepath.Join(dir, "gmail-token.json")
	if err := os.WriteFile(tokenPath, []byte(`{"access_token":"x"}`), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	fp := &fakeProvider{name: "gmail", tokenPath: tokenPath}
	rp := &revokingProvider{fakeProvider: fp}
	att := &fakeAttestor{err: errors.New("operator denied")}

	err := Disconnect(context.Background(), rp, att)
	if !errors.Is(err, ErrAttestationDenied) {
		t.Fatalf("err: got %v, want ErrAttestationDenied", err)
	}
	if rp.revoked != 0 {
		t.Fatalf("revoke called despite denial: %d", rp.revoked)
	}
	if _, err := os.Stat(tokenPath); err != nil {
		t.Fatalf("token file deleted on denial: %v", err)
	}
}

// TestDisconnect_NoTokenFile_ReturnsSentinel reports ErrTokenFileNotFound for nothing-to-do.
func TestDisconnect_NoTokenFile_ReturnsSentinel(t *testing.T) {
	dir := t.TempDir()
	tokenPath := filepath.Join(dir, "gmail-token.json")
	fp := &fakeProvider{name: "gmail", tokenPath: tokenPath}
	att := &fakeAttestor{}

	err := Disconnect(context.Background(), fp, att)
	if !errors.Is(err, ErrTokenFileNotFound) {
		t.Fatalf("err: got %v, want ErrTokenFileNotFound", err)
	}
}

// TestDisconnect_RevokeError_StillDeletesFile keeps local cleanup the primary intent.
func TestDisconnect_RevokeError_StillDeletesFile(t *testing.T) {
	dir := t.TempDir()
	tokenPath := filepath.Join(dir, "gmail-token.json")
	if err := os.WriteFile(tokenPath, []byte(`{"access_token":"x"}`), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	fp := &fakeProvider{name: "gmail", tokenPath: tokenPath}
	rp := &revokingProvider{fakeProvider: fp, revokeErr: errors.New("upstream 500")}
	att := &fakeAttestor{}

	if err := Disconnect(context.Background(), rp, att); err != nil {
		t.Fatalf("Disconnect: %v", err)
	}
	if _, err := os.Stat(tokenPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("file kept despite revoke best-effort: %v", err)
	}
}

// TestDisconnect_NoRevokeInterface_StillDeletes covers providers without Revoke.
func TestDisconnect_NoRevokeInterface_StillDeletes(t *testing.T) {
	dir := t.TempDir()
	tokenPath := filepath.Join(dir, "gmail-token.json")
	if err := os.WriteFile(tokenPath, []byte(`{"access_token":"x"}`), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	fp := &fakeProvider{name: "gmail", tokenPath: tokenPath}
	att := &fakeAttestor{}

	if err := Disconnect(context.Background(), fp, att); err != nil {
		t.Fatalf("Disconnect: %v", err)
	}
	if _, err := os.Stat(tokenPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("file still present: %v", err)
	}
}
