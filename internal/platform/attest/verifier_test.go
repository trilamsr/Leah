package attest

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeTemp(t *testing.T, name string, body []byte) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, body, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	return p
}

func sha256Hex(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func TestVerifyArtifact_GoodSHA(t *testing.T) {
	payload := []byte("model bytes v1")
	path := writeTemp(t, "wake.mlmodel", payload)

	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("genkey: %v", err)
	}
	v := NewVerifier(Config{Clock: func() time.Time { return time.Unix(1_700_000_000, 0) }})
	att, err := v.VerifyArtifact(context.Background(), path, ManifestRef{
		Path: path, ExpectedSHA256: sha256Hex(payload), Pubkey: pub,
	})
	if err != nil {
		t.Fatalf("VerifyArtifact: %v", err)
	}
	if att.State != Verified {
		t.Fatalf("state=%s reason=%s want Verified", att.State, att.Reason)
	}
	if got, want := att.NextRecheck, att.VerifiedAt.Add(7*24*time.Hour); !got.Equal(want) {
		t.Fatalf("artifact recheck = %v, want VerifiedAt+7d (%v)", got, want)
	}
}

func TestVerifyArtifact_BadSHA(t *testing.T) {
	payload := []byte("real bytes")
	path := writeTemp(t, "wake.mlmodel", payload)
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	v := NewVerifier(Config{})
	att, err := v.VerifyArtifact(context.Background(), path, ManifestRef{
		Path: path, ExpectedSHA256: sha256Hex([]byte("tampered")), Pubkey: pub,
	})
	if err != nil {
		t.Fatalf("VerifyArtifact: %v", err)
	}
	if att.State != Failed {
		t.Fatalf("state=%s want Failed", att.State)
	}
	if att.Reason == "" {
		t.Fatalf("Failed must populate Reason")
	}
}

func TestVerifySelf_Mismatch(t *testing.T) {
	bin := writeTemp(t, "leah-bin", []byte("tampered binary"))
	v := NewVerifier(Config{
		SelfPath:           bin,
		SelfExpectedSHA256: sha256Hex([]byte("trusted original")),
	})
	att, err := v.VerifySelf(context.Background())
	if err != nil {
		t.Fatalf("VerifySelf: %v", err)
	}
	if att.State != Failed {
		t.Fatalf("tampered binary must yield Failed, got %s", att.State)
	}
	if att.Reason == "" {
		t.Fatalf("Failed must include Reason")
	}
}

func TestVerifySelf_Match(t *testing.T) {
	body := []byte("trusted binary")
	bin := writeTemp(t, "leah-bin", body)
	v := NewVerifier(Config{SelfPath: bin, SelfExpectedSHA256: sha256Hex(body)})
	att, err := v.VerifySelf(context.Background())
	if err != nil {
		t.Fatalf("VerifySelf: %v", err)
	}
	if att.State != Verified {
		t.Fatalf("state=%s want Verified", att.State)
	}
}

func TestRecheckPolicy_Self24h(t *testing.T) {
	body := []byte("trusted binary")
	bin := writeTemp(t, "leah-bin", body)
	now := time.Unix(1_700_000_000, 0)
	v := NewVerifier(Config{
		SelfPath:           bin,
		SelfExpectedSHA256: sha256Hex(body),
		Clock:              func() time.Time { return now },
	})
	att, err := v.VerifySelf(context.Background())
	if err != nil {
		t.Fatalf("VerifySelf: %v", err)
	}
	if got, want := att.NextRecheck, now.Add(24*time.Hour); !got.Equal(want) {
		t.Fatalf("NextRecheck = %v, want VerifiedAt+24h (%v)", got, want)
	}
}

func TestRevocationList_OfflineToleranceIs7d(t *testing.T) {
	rl := RevocationList{
		Pubkeys:   []string{"abc"},
		FetchedAt: time.Now().Add(-6 * 24 * time.Hour),
	}
	if rl.Stale(time.Now()) {
		t.Fatalf("6d-old list must not be stale")
	}
	rl.FetchedAt = time.Now().Add(-8 * 24 * time.Hour)
	if !rl.Stale(time.Now()) {
		t.Fatalf("8d-old list must be stale")
	}
}

func TestFetchRevocations_RoundTrip(t *testing.T) {
	want := RevocationList{Pubkeys: []string{"deadbeef", "feedface"}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(struct {
			Pubkeys []string `json:"pubkeys"`
		}{want.Pubkeys})
	}))
	defer srv.Close()
	got, err := FetchRevocations(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("FetchRevocations: %v", err)
	}
	if len(got.Pubkeys) != 2 || got.Pubkeys[0] != "deadbeef" {
		t.Fatalf("pubkeys = %v", got.Pubkeys)
	}
	if got.FetchedAt.IsZero() {
		t.Fatalf("FetchedAt must be set")
	}
}

func TestVerifier_Subscribe_EmitsOnVerdict(t *testing.T) {
	body := []byte("trusted")
	bin := writeTemp(t, "leah-bin", body)
	v := NewVerifier(Config{SelfPath: bin, SelfExpectedSHA256: sha256Hex(body)})
	ch := v.Subscribe()
	if _, err := v.VerifySelf(context.Background()); err != nil {
		t.Fatalf("VerifySelf: %v", err)
	}
	select {
	case att := <-ch:
		if att.State != Verified {
			t.Fatalf("subscriber got state=%s want Verified", att.State)
		}
	case <-time.After(time.Second):
		t.Fatalf("subscriber received no Attestation")
	}
}

func TestLastVerdict_TracksLatest(t *testing.T) {
	v := NewVerifier(Config{})
	if v.LastVerdict().State != Unknown {
		t.Fatalf("initial state must be Unknown, got %s", v.LastVerdict().State)
	}
	body := []byte("ok")
	bin := writeTemp(t, "leah-bin", body)
	v2 := NewVerifier(Config{SelfPath: bin, SelfExpectedSHA256: sha256Hex(body)})
	if _, err := v2.VerifySelf(context.Background()); err != nil {
		t.Fatalf("VerifySelf: %v", err)
	}
	if v2.LastVerdict().State != Verified {
		t.Fatalf("LastVerdict = %s, want Verified", v2.LastVerdict().State)
	}
}

func TestVerifyPlugin_MissingBundle_Failed(t *testing.T) {
	v := NewVerifier(Config{})
	att, err := v.VerifyPlugin(context.Background(), filepath.Join(t.TempDir(), "nope.bundle"))
	if err != nil {
		t.Fatalf("VerifyPlugin err: %v", err)
	}
	if att.State != Failed {
		t.Fatalf("missing plugin bundle must yield Failed, got %s", att.State)
	}
}

func TestVerifyPlugin_RevokedPubkey_Failed(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	dir := t.TempDir()
	bundle := filepath.Join(dir, "p.bundle")
	if err := os.Mkdir(bundle, 0o755); err != nil {
		t.Fatal(err)
	}
	payload := []byte("plugin payload")
	if err := os.WriteFile(filepath.Join(bundle, "plugin.bin"), payload, 0o644); err != nil {
		t.Fatal(err)
	}
	files := []manifestFile{{Path: "plugin.bin", SHA256: sha256Hex(payload)}}
	body, _ := json.Marshal(files)
	sig := ed25519.Sign(priv, body)
	mf := pluginManifest{
		Files:  files,
		Pubkey: hex.EncodeToString(pub),
		Sig:    hex.EncodeToString(sig),
	}
	raw, _ := json.Marshal(mf)
	if err := os.WriteFile(filepath.Join(bundle, "manifest.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}

	v := NewVerifier(Config{Revocations: &RevocationList{
		Pubkeys:   []string{hex.EncodeToString(pub)},
		FetchedAt: time.Now(),
	}})
	att, err := v.VerifyPlugin(context.Background(), bundle)
	if err != nil {
		t.Fatalf("VerifyPlugin: %v", err)
	}
	if att.State != Failed {
		t.Fatalf("revoked pubkey must yield Failed, got %s reason=%s", att.State, att.Reason)
	}
}

func TestVerifyPlugin_GoodSig_Verified(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	dir := t.TempDir()
	bundle := filepath.Join(dir, "p.bundle")
	if err := os.Mkdir(bundle, 0o755); err != nil {
		t.Fatal(err)
	}
	payload := []byte("plugin payload")
	if err := os.WriteFile(filepath.Join(bundle, "plugin.bin"), payload, 0o644); err != nil {
		t.Fatal(err)
	}
	files := []manifestFile{{Path: "plugin.bin", SHA256: sha256Hex(payload)}}
	body, _ := json.Marshal(files)
	sig := ed25519.Sign(priv, body)
	mf := pluginManifest{Files: files, Pubkey: hex.EncodeToString(pub), Sig: hex.EncodeToString(sig)}
	raw, _ := json.Marshal(mf)
	if err := os.WriteFile(filepath.Join(bundle, "manifest.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	v := NewVerifier(Config{})
	att, err := v.VerifyPlugin(context.Background(), bundle)
	if err != nil {
		t.Fatalf("VerifyPlugin: %v", err)
	}
	if att.State != Verified {
		t.Fatalf("state=%s reason=%s want Verified", att.State, att.Reason)
	}
}
