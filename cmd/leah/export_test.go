package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// seedStateDir populates a fake ~/.leah-state with a handful of files so the
// roundtrip tests have something deterministic to compare. The contents are
// arbitrary — only the bytes-after-import-equal-bytes-before-export property
// is being exercised.
func seedStateDir(t *testing.T, dir string) {
	t.Helper()
	files := map[string][]byte{
		"audit.jsonl":          []byte(`{"kind":"seed","blast_radius":0}` + "\n"),
		"memory.db":            []byte("SQLite format 3\x00fake-memory-bytes"),
		"connect/gmail.json":   []byte(`{"provider":"gmail","token":"redacted"}`),
		"mirror/gmail.db":      []byte("SQLite format 3\x00fake-mirror-bytes"),
		"knowledge.db":         []byte("SQLite format 3\x00fake-knowledge-bytes"),
		"nested/sub/deep.json": []byte(`{"deep":true}`),
	}
	for rel, content := range files {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(p), err)
		}
		if err := os.WriteFile(p, content, 0o600); err != nil {
			t.Fatalf("write %s: %v", p, err)
		}
	}
}

// dirSnapshot returns a stable map of relpath → SHA-256-ish (we just use raw
// bytes since the trees are tiny). Roundtrip tests compare snapshots before
// and after the export → wipe → import cycle.
func dirSnapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		out[rel] = string(b)
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	return out
}

// TestExport_EncryptsArchive_RoundTrip seeds a state dir, exports it, wipes
// the state dir, imports back, and asserts every file is byte-identical. The
// archive file itself MUST be non-cleartext (raw bytes from the encrypted
// section should NOT contain any seeded plaintext string).
func TestExport_EncryptsArchive_RoundTrip(t *testing.T) {
	stateDir := t.TempDir()
	outDir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", stateDir)
	t.Setenv("LEAH_EXPORT_PASSPHRASE", "correct horse battery staple")

	seedStateDir(t, stateDir)
	archivePath := filepath.Join(outDir, "leah-export.tar.gz.enc")

	if rc := runExport(context.Background(), []string{"--all", "--out", archivePath}, io.Discard); rc != 0 {
		t.Fatalf("export exit = %d, want 0", rc)
	}

	// Snapshot AFTER export — export appends one audit-row to audit.jsonl, and
	// that row is captured inside the archive. Roundtrip then asserts byte-
	// equality vs the post-export state.
	before := dirSnapshot(t, stateDir)

	// Archive ciphertext MUST NOT contain plaintext sentinel from the seeded
	// state dir. The cleartext header is small and known — we strip past the
	// separator before scanning.
	raw, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatalf("read archive: %v", err)
	}
	sep := []byte("\n--ENCRYPTED--\n")
	idx := bytes.Index(raw, sep)
	if idx < 0 {
		t.Fatalf("archive missing --ENCRYPTED-- separator")
	}
	ciphertext := raw[idx+len(sep):]
	if bytes.Contains(ciphertext, []byte("redacted")) {
		t.Fatalf("ciphertext leaks plaintext token")
	}
	if bytes.Contains(ciphertext, []byte("fake-memory-bytes")) {
		t.Fatalf("ciphertext leaks plaintext db bytes")
	}

	// Wipe state dir then import.
	if err := os.RemoveAll(stateDir); err != nil {
		t.Fatalf("wipe state dir: %v", err)
	}
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		t.Fatalf("recreate state dir: %v", err)
	}
	t.Setenv("LEAH_IMPORT_PASSPHRASE", "correct horse battery staple")

	if rc := runImport(context.Background(), []string{archivePath}, io.Discard); rc != 0 {
		t.Fatalf("import exit = %d, want 0", rc)
	}

	after := dirSnapshot(t, stateDir)
	for rel, want := range before {
		// audit.jsonl legitimately grows by one row at import time (the
		// success audit row appended post-extract). Skip strict equality.
		if rel == "audit.jsonl" {
			got := after[rel]
			if !strings.HasPrefix(got, want) {
				t.Fatalf("audit.jsonl prefix diff after import")
			}
			continue
		}
		got, ok := after[rel]
		if !ok {
			t.Fatalf("import lost file %q", rel)
		}
		if got != want {
			t.Fatalf("import mismatch for %q: got %d bytes, want %d bytes", rel, len(got), len(want))
		}
	}
}

// TestExport_WrongPassphrase_DecryptFails confirms a passphrase mismatch on
// import returns a non-zero exit (decrypt error) and does NOT touch the
// state dir.
func TestExport_WrongPassphrase_DecryptFails(t *testing.T) {
	stateDir := t.TempDir()
	outDir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", stateDir)
	t.Setenv("LEAH_EXPORT_PASSPHRASE", "right-passphrase")

	seedStateDir(t, stateDir)
	archivePath := filepath.Join(outDir, "leah-export.tar.gz.enc")
	if rc := runExport(context.Background(), []string{"--all", "--out", archivePath}, io.Discard); rc != 0 {
		t.Fatalf("export exit = %d, want 0", rc)
	}

	// Wipe + recreate empty state dir; attempt import with wrong passphrase.
	if err := os.RemoveAll(stateDir); err != nil {
		t.Fatalf("wipe: %v", err)
	}
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		t.Fatalf("recreate: %v", err)
	}
	t.Setenv("LEAH_IMPORT_PASSPHRASE", "wrong-passphrase")

	var stderr bytes.Buffer
	if rc := runImport(context.Background(), []string{archivePath}, &stderr); rc == 0 {
		t.Fatalf("import with wrong passphrase exit = 0, want non-zero")
	}

	// State dir MUST NOT contain extracted content. audit.jsonl and an empty
	// memory.db are allowed — the audit logger's DefaultWorkspace callback
	// opens ctxmgr (→ creates an empty memory.db) before writing the failure
	// row. Both are bookkeeping side-effects, not archive extraction.
	for _, name := range []string{"connect", "mirror", "knowledge.db", "nested"} {
		if _, err := os.Stat(filepath.Join(stateDir, name)); err == nil {
			t.Fatalf("state dir leaked extracted entry %q after failed import", name)
		}
	}
}

// TestExport_HMACMismatch_VerifyFails flips one byte in the cleartext header
// after export. Import MUST detect this via the HMAC check before attempting
// AES-GCM decrypt.
func TestExport_HMACMismatch_VerifyFails(t *testing.T) {
	stateDir := t.TempDir()
	outDir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", stateDir)
	t.Setenv("LEAH_EXPORT_PASSPHRASE", "passphrase-for-hmac-test")

	seedStateDir(t, stateDir)
	archivePath := filepath.Join(outDir, "leah-export.tar.gz.enc")
	if rc := runExport(context.Background(), []string{"--all", "--out", archivePath}, io.Discard); rc != 0 {
		t.Fatalf("export: %d", rc)
	}

	// Tamper: flip one byte inside the cleartext header JSON. We rewrite the
	// first occurrence of "version":1 to "version":2 — both still parse, but
	// the HMAC over the original bytes no longer matches.
	raw, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	tampered := bytes.Replace(raw, []byte(`"version":1`), []byte(`"version":2`), 1)
	if bytes.Equal(raw, tampered) {
		t.Fatalf("tamper no-op — header schema may have drifted")
	}
	if err := os.WriteFile(archivePath, tampered, 0o600); err != nil {
		t.Fatalf("write tampered: %v", err)
	}

	// Wipe + recreate empty state dir.
	if err := os.RemoveAll(stateDir); err != nil {
		t.Fatalf("wipe: %v", err)
	}
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		t.Fatalf("recreate: %v", err)
	}
	t.Setenv("LEAH_IMPORT_PASSPHRASE", "passphrase-for-hmac-test")

	var stderr bytes.Buffer
	if rc := runImport(context.Background(), []string{archivePath}, &stderr); rc == 0 {
		t.Fatalf("import on tampered header exit = 0, want non-zero")
	}
	if !strings.Contains(stderr.String(), "hmac") && !strings.Contains(stderr.String(), "HMAC") && !strings.Contains(stderr.String(), "header") {
		t.Fatalf("expected hmac/header error in stderr, got: %s", stderr.String())
	}
}

// TestImport_RefusesOverwriteWithoutAttestation confirms `leah import` refuses
// to clobber a non-empty state dir unless `--overwrite` is passed AND the
// BR=4 attestation phrase is supplied (or LEAH_IMPORT_AUTO_ATTEST=1 in test).
func TestImport_RefusesOverwriteWithoutAttestation(t *testing.T) {
	stateDir := t.TempDir()
	outDir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", stateDir)
	t.Setenv("LEAH_EXPORT_PASSPHRASE", "p")

	seedStateDir(t, stateDir)
	archivePath := filepath.Join(outDir, "leah-export.tar.gz.enc")
	if rc := runExport(context.Background(), []string{"--all", "--out", archivePath}, io.Discard); rc != 0 {
		t.Fatalf("export: %d", rc)
	}

	// State dir is non-empty AND no --overwrite. Import MUST refuse.
	t.Setenv("LEAH_IMPORT_PASSPHRASE", "p")
	var stderr bytes.Buffer
	rc := runImport(context.Background(), []string{archivePath}, &stderr)
	if rc == 0 {
		t.Fatalf("import on non-empty state dir without --overwrite exit = 0, want non-zero")
	}
	if !strings.Contains(stderr.String(), "overwrite") && !strings.Contains(stderr.String(), "non-empty") {
		t.Fatalf("expected overwrite/non-empty error in stderr, got: %s", stderr.String())
	}
}

// TestImport_OverwriteWithBR4_Succeeds confirms the BR=4 attestation path:
// non-empty state dir + --overwrite + LEAH_IMPORT_AUTO_ATTEST=1 succeeds and
// produces a state dir matching the archive's contents (not the prior junk).
func TestImport_OverwriteWithBR4_Succeeds(t *testing.T) {
	stateDir := t.TempDir()
	outDir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", stateDir)
	t.Setenv("LEAH_EXPORT_PASSPHRASE", "phrase")

	seedStateDir(t, stateDir)
	archivePath := filepath.Join(outDir, "leah-export.tar.gz.enc")
	if rc := runExport(context.Background(), []string{"--all", "--out", archivePath}, io.Discard); rc != 0 {
		t.Fatalf("export: %d", rc)
	}
	want := dirSnapshot(t, stateDir)

	// Drop a "junk" file that should be wiped by overwrite.
	junkPath := filepath.Join(stateDir, "junk-from-old-install.txt")
	if err := os.WriteFile(junkPath, []byte("stale"), 0o600); err != nil {
		t.Fatalf("write junk: %v", err)
	}

	t.Setenv("LEAH_IMPORT_PASSPHRASE", "phrase")
	t.Setenv("LEAH_IMPORT_AUTO_ATTEST", "1")

	var stderr bytes.Buffer
	if rc := runImport(context.Background(), []string{"--overwrite", archivePath}, &stderr); rc != 0 {
		t.Fatalf("import --overwrite with attest exit = %d (stderr=%s)", rc, stderr.String())
	}

	got := dirSnapshot(t, stateDir)
	// Junk file MUST be gone.
	if _, ok := got["junk-from-old-install.txt"]; ok {
		t.Fatalf("overwrite did not remove pre-existing file")
	}
	for rel, w := range want {
		if rel == "audit.jsonl" {
			// import-success row is appended → prefix-equality only.
			if !strings.HasPrefix(got[rel], w) {
				t.Fatalf("audit.jsonl prefix diff after overwrite")
			}
			continue
		}
		g, ok := got[rel]
		if !ok {
			t.Fatalf("file %q missing after overwrite", rel)
		}
		if g != w {
			t.Fatalf("file %q diff after overwrite", rel)
		}
	}
}

// TestExport_HeaderClearReadable confirms the cleartext header is JSON the
// operator can `head -c | jq` without decrypting — provenance check.
func TestExport_HeaderClearReadable(t *testing.T) {
	stateDir := t.TempDir()
	outDir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", stateDir)
	t.Setenv("LEAH_EXPORT_PASSPHRASE", "header-read-test")

	seedStateDir(t, stateDir)
	archivePath := filepath.Join(outDir, "leah-export.tar.gz.enc")
	if rc := runExport(context.Background(), []string{"--all", "--out", archivePath}, io.Discard); rc != 0 {
		t.Fatalf("export: %d", rc)
	}

	raw, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	sep := []byte("\n--ENCRYPTED--\n")
	idx := bytes.Index(raw, sep)
	if idx <= 0 {
		t.Fatalf("missing separator")
	}
	header := raw[:idx]
	// Header is `<json>\n<hmac-base64>`. Split off the HMAC trailer for parse.
	lines := bytes.SplitN(header, []byte("\n"), 2)
	if len(lines) != 2 {
		t.Fatalf("header expected 2 lines (json + hmac), got %d", len(lines))
	}
	var h map[string]any
	if err := json.Unmarshal(lines[0], &h); err != nil {
		t.Fatalf("header not valid JSON: %v\n%s", err, lines[0])
	}
	if h["version"] == nil {
		t.Fatalf("header missing version field")
	}
	if h["pbkdf2_iter"] == nil {
		t.Fatalf("header missing pbkdf2_iter field")
	}
	if h["salt"] == nil {
		t.Fatalf("header missing salt field")
	}
}
