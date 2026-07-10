package plugin

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/trilam/leah/internal/platform/attest"
	"github.com/trilam/leah/internal/memory/sqlstore"
	"github.com/trilam/leah/pkg/leahplugin"

	_ "modernc.org/sqlite"
)

func TestHost_InstallVerifiedBundleRoundTrip(t *testing.T) {
	db := openMigratedDB(t)
	bundle := mkSignedBundle(t)
	h := mustHost(t, db, alwaysVerified{})

	id, err := h.Install(context.Background(), bundle)
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if id != "com.example.weather" {
		t.Fatalf("got id %q", id)
	}
	list := h.List()
	if len(list) != 1 || list[0].ID != id || !list[0].Enabled {
		t.Fatalf("list: %+v", list)
	}
}

func TestHost_InstallRefusesFailedAttest(t *testing.T) {
	db := openMigratedDB(t)
	bundle := mkSignedBundle(t)
	h := mustHost(t, db, alwaysFailed{})
	_, err := h.Install(context.Background(), bundle)
	if !errors.Is(err, ErrPluginAttestFailed) {
		t.Fatalf("want ErrPluginAttestFailed, got %v", err)
	}
}

func TestHost_InstallRefusesBadManifest(t *testing.T) {
	db := openMigratedDB(t)
	dir := t.TempDir()
	contents := filepath.Join(dir, "Contents")
	_ = os.MkdirAll(contents, 0o755)
	_ = os.WriteFile(filepath.Join(contents, "manifest.json"), []byte(`{"schema_version":1}`), 0o644)
	h := mustHost(t, db, alwaysVerified{})
	_, err := h.Install(context.Background(), dir)
	if !errors.Is(err, ErrManifestRequiredField) {
		t.Fatalf("want ErrManifestRequiredField, got %v", err)
	}
}

func TestHost_EnableDisableUninstall(t *testing.T) {
	db := openMigratedDB(t)
	bundle := mkSignedBundle(t)
	h := mustHost(t, db, alwaysVerified{})
	id, err := h.Install(context.Background(), bundle)
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if err := h.Disable(context.Background(), id); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if h.List()[0].Enabled {
		t.Fatal("expected disabled")
	}
	if err := h.Enable(context.Background(), id); err != nil {
		t.Fatalf("enable: %v", err)
	}
	if !h.List()[0].Enabled {
		t.Fatal("expected enabled")
	}
	if err := h.Uninstall(context.Background(), id); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if len(h.List()) != 0 {
		t.Fatal("expected empty list after uninstall")
	}
	if err := h.Disable(context.Background(), id); !errors.Is(err, ErrPluginNotFound) {
		t.Fatalf("want ErrPluginNotFound on missing id, got %v", err)
	}
}

func TestHost_ReloadFailedDisables(t *testing.T) {
	db := openMigratedDB(t)
	bundle := mkSignedBundle(t)
	v := &switchVerifier{state: attest.Verified}
	h := mustHost(t, db, v)
	id, err := h.Install(context.Background(), bundle)
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	v.state = attest.Failed
	if err := h.Reload(context.Background(), id); !errors.Is(err, ErrPluginAttestFailed) {
		t.Fatalf("want ErrPluginAttestFailed on reload, got %v", err)
	}
	if h.List()[0].Enabled {
		t.Fatal("expected reload-failed plugin to be disabled")
	}
}

func TestHost_LogsTailNewestFirst(t *testing.T) {
	db := openMigratedDB(t)
	bundle := mkSignedBundle(t)
	h := mustHost(t, db, alwaysVerified{})
	id, err := h.Install(context.Background(), bundle)
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	concrete := h.(*host)
	for i := 0; i < 5; i++ {
		if err := concrete.AppendLog(context.Background(), id, leahplugin.LogInfo, "msg", nil); err != nil {
			t.Fatalf("log %d: %v", i, err)
		}
	}
	logs, err := h.Logs(context.Background(), id, 3)
	if err != nil {
		t.Fatalf("logs: %v", err)
	}
	if len(logs) != 3 {
		t.Fatalf("want 3 rows, got %d", len(logs))
	}
}

func TestHost_NilVerifierRefused(t *testing.T) {
	db := openMigratedDB(t)
	_, err := NewHost(HostConfig{DB: db})
	if err == nil {
		t.Fatal("expected error when Verifier is nil")
	}
}

// --- helpers ---

func openMigratedDB(t *testing.T) *sql.DB {
	t.Helper()
	dir := t.TempDir()
	db, err := sqlstore.OpenWAL(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := sqlstore.MigrateUp(context.Background(), db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func mustHost(t *testing.T, db *sql.DB, v attest.Verifier) Host {
	t.Helper()
	h, err := NewHost(HostConfig{
		DB:       db,
		Verifier: v,
		Sandbox:  NewSandbox(nil),
		Clock:    func() time.Time { return time.Unix(1700000000, 0) },
	})
	if err != nil {
		t.Fatalf("NewHost: %v", err)
	}
	return h
}

// mkSignedBundle builds a minimal MyPlugin.leahplugin layout with a valid attest manifest so happy-path install+reload work.
func mkSignedBundle(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	contents := filepath.Join(dir, "Contents")
	if err := os.MkdirAll(contents, 0o755); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(contents, "binary")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(contents, "manifest.json"), []byte(validManifestJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	// also write an attest manifest in case a real verifier is wired — tests using fake verifiers skip this anyway.
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	binBytes, _ := os.ReadFile(bin)
	mfBytes, _ := os.ReadFile(filepath.Join(contents, "manifest.json"))
	sumBin := sha256.Sum256(binBytes)
	sumMf := sha256.Sum256(mfBytes)
	files := []map[string]string{
		{"path": "Contents/binary", "sha256": hex.EncodeToString(sumBin[:])},
		{"path": "Contents/manifest.json", "sha256": hex.EncodeToString(sumMf[:])},
	}
	body, _ := json.Marshal(files)
	sig := ed25519.Sign(priv, body)
	attMf := map[string]any{
		"files":  files,
		"pubkey": hex.EncodeToString(pub),
		"sig":    hex.EncodeToString(sig),
	}
	raw, _ := json.Marshal(attMf)
	_ = os.MkdirAll(filepath.Join(dir, "Signature"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, "manifest.json"), raw, 0o644)
	return dir
}

type alwaysVerified struct{}

func (alwaysVerified) VerifySelf(context.Context) (attest.Attestation, error) {
	return attest.Attestation{State: attest.Verified}, nil
}
func (alwaysVerified) VerifyArtifact(context.Context, string, attest.ManifestRef) (attest.Attestation, error) {
	return attest.Attestation{State: attest.Verified}, nil
}
func (alwaysVerified) VerifyPlugin(context.Context, string) (attest.Attestation, error) {
	return attest.Attestation{State: attest.Verified}, nil
}
func (alwaysVerified) LastVerdict() attest.Attestation {
	return attest.Attestation{State: attest.Verified}
}
func (alwaysVerified) Subscribe() <-chan attest.Attestation {
	ch := make(chan attest.Attestation)
	close(ch)
	return ch
}

type alwaysFailed struct{}

func (alwaysFailed) VerifySelf(context.Context) (attest.Attestation, error) {
	return attest.Attestation{State: attest.Failed, Reason: "test"}, nil
}
func (alwaysFailed) VerifyArtifact(context.Context, string, attest.ManifestRef) (attest.Attestation, error) {
	return attest.Attestation{State: attest.Failed}, nil
}
func (alwaysFailed) VerifyPlugin(context.Context, string) (attest.Attestation, error) {
	return attest.Attestation{State: attest.Failed, Reason: "test"}, nil
}
func (alwaysFailed) LastVerdict() attest.Attestation { return attest.Attestation{State: attest.Failed} }
func (alwaysFailed) Subscribe() <-chan attest.Attestation {
	ch := make(chan attest.Attestation)
	close(ch)
	return ch
}

type switchVerifier struct{ state attest.AttestState }

func (s *switchVerifier) VerifySelf(context.Context) (attest.Attestation, error) {
	return attest.Attestation{State: s.state}, nil
}
func (s *switchVerifier) VerifyArtifact(context.Context, string, attest.ManifestRef) (attest.Attestation, error) {
	return attest.Attestation{State: s.state}, nil
}
func (s *switchVerifier) VerifyPlugin(context.Context, string) (attest.Attestation, error) {
	return attest.Attestation{State: s.state, Reason: "switch"}, nil
}
func (s *switchVerifier) LastVerdict() attest.Attestation { return attest.Attestation{State: s.state} }
func (s *switchVerifier) Subscribe() <-chan attest.Attestation {
	ch := make(chan attest.Attestation)
	close(ch)
	return ch
}
