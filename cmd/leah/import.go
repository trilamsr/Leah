package main

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/pbkdf2"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/trilam/leah/internal/audit"
)

// importAttestPhrase is the BR=4 phrase the operator must type to authorize
// an overwrite-import. Sibling pattern to forgetAttest's `purge everything`.
const importAttestPhrase = "overwrite my leah state"

// ErrImportAttestationDenied fires when --overwrite is set but the BR=4
// phrase was not typed (and LEAH_IMPORT_AUTO_ATTEST != 1).
var ErrImportAttestationDenied = errors.New("import: overwrite attestation denied")

// runImport implements `leah import <path> [--overwrite]`. Verifies the
// cleartext header HMAC, derives the AES key from the operator passphrase,
// decrypts the AES-GCM ciphertext, extracts the tar.gz into ~/.leah-state/.
//
// Refuses to clobber a non-empty state dir unless `--overwrite` is set AND
// the BR=4 attestation phrase is typed (or LEAH_IMPORT_AUTO_ATTEST=1).
func runImport(_ context.Context, args []string, errW io.Writer) int {
	fs := flag.NewFlagSet("import", flag.ContinueOnError)
	overwrite := fs.Bool("overwrite", false, "wipe non-empty state dir before extract (BR=4 attested)")
	fs.SetOutput(errW)
	if err := fs.Parse(args); err != nil {
		fs.Usage()
		return 2
	}
	rest := fs.Args()
	if len(rest) != 1 {
		_, _ = fmt.Fprintln(errW, "usage: leah import <archive> [--overwrite]")
		return 2
	}
	archivePath := rest[0]

	sd := stateDir()
	a := &audit.Logger{Path: filepath.Join(sd, "audit.jsonl"), DefaultWorkspace: activeWorkspace}

	// Non-empty state dir without --overwrite ⇒ refuse before reading the
	// archive (avoid spurious passphrase prompts on the safe-fail path). A
	// fresh state dir contains exactly one entry — `audit.jsonl` — after the
	// stateDir() call lazily logs the import attempt, so "non-empty" means
	// "has files other than the just-created audit.jsonl".
	nonEmpty, err := dirNonEmptyExcept(sd, "audit.jsonl")
	if err != nil {
		_, _ = fmt.Fprintf(errW, "leah import: %v\n", err)
		return 1
	}
	if nonEmpty && !*overwrite {
		_, _ = fmt.Fprintf(errW, "leah import: state dir %s is non-empty — pass --overwrite to clobber\n", sd)
		_ = a.Append(audit.Entry{Kind: "import_all", BlastRadius: 4, Outcome: "failed", Detail: "non_empty_no_overwrite"})
		return 1
	}
	if nonEmpty && *overwrite {
		if err := importAttest(); err != nil {
			_, _ = fmt.Fprintf(errW, "leah import: %v\n", err)
			_ = a.Append(audit.Entry{Kind: "import_all", BlastRadius: 4, Outcome: "failed", Detail: "attestation_denied"})
			return 1
		}
	}

	pass, err := readPassphrase("LEAH_IMPORT_PASSPHRASE")
	if err != nil {
		_, _ = fmt.Fprintf(errW, "leah import: %v\n", err)
		_ = a.Append(audit.Entry{Kind: "import_all", BlastRadius: 4, Outcome: "failed", Detail: "passphrase_read"})
		return 1
	}

	if err := readEncryptedArchive(archivePath, sd, pass, *overwrite); err != nil {
		_, _ = fmt.Fprintf(errW, "leah import: %v\n", err)
		br := 3
		if *overwrite {
			br = 4
		}
		_ = a.Append(audit.Entry{Kind: "import_all", BlastRadius: br, Outcome: "failed", Detail: err.Error()})
		return 1
	}

	br := 3
	if *overwrite {
		br = 4
	}
	_ = a.Append(audit.Entry{Kind: "import_all", BlastRadius: br, Outcome: "success", Detail: archivePath})
	_, _ = fmt.Fprintf(os.Stdout, "imported %s → %s\n", archivePath, sd)
	return 0
}

// readEncryptedArchive verifies the cleartext header HMAC, derives the AES
// key via PBKDF2 with the header-provided salt, decrypts AES-GCM, and
// extracts the tar.gz into dstRoot. If overwrite=true, dstRoot is wiped to
// empty before extraction (already attested upstream).
func readEncryptedArchive(srcPath, dstRoot, pass string, overwrite bool) error {
	raw, err := os.ReadFile(srcPath)
	if err != nil {
		return fmt.Errorf("read archive: %w", err)
	}
	sepIdx := bytes.Index(raw, []byte(exportSeparator))
	if sepIdx < 0 {
		return errors.New("archive missing --ENCRYPTED-- separator (not a leah export)")
	}
	headerSection := raw[:sepIdx]
	encSection := raw[sepIdx+len(exportSeparator):]

	// Split header section into <json>\n<hmac-b64>.
	nl := bytes.LastIndexByte(headerSection, '\n')
	if nl < 0 {
		return errors.New("archive header missing HMAC trailer")
	}
	headerJSON := headerSection[:nl]
	macB64 := string(headerSection[nl+1:])

	var hdr archiveHeader
	if err := json.Unmarshal(headerJSON, &hdr); err != nil {
		return fmt.Errorf("parse header: %w", err)
	}
	salt, err := base64.StdEncoding.DecodeString(hdr.Salt)
	if err != nil {
		return fmt.Errorf("decode salt: %w", err)
	}

	// Derive AES + HMAC keys from passphrase + header salt. We use the
	// iteration count the header records (back-compat across future bumps)
	// but fall back to the current baseline when the field is missing.
	iter := hdr.PBKDF2Iter
	if iter <= 0 {
		iter = pbkdf2Iterations
	}
	keys, err := pbkdf2.Key(sha256.New, pass, salt, iter, pbkdf2KeyLen)
	if err != nil {
		return fmt.Errorf("pbkdf2: %w", err)
	}
	aesKey, hmacKey := keys[:32], keys[32:]

	// Verify HMAC over headerJSON BEFORE any other header validation — a
	// tampered header (e.g. version field flip) must be rejected as
	// "tampered" not "unsupported version". This is the operator-facing
	// trust signal in the M3 spec.
	wantMAC, err := base64.StdEncoding.DecodeString(macB64)
	if err != nil {
		return fmt.Errorf("decode hmac: %w", err)
	}
	h := hmac.New(sha256.New, hmacKey)
	_, _ = h.Write(headerJSON)
	if !hmac.Equal(h.Sum(nil), wantMAC) {
		return ErrHeaderHMAC
	}

	if hdr.Version != 1 {
		return fmt.Errorf("unsupported archive version %d", hdr.Version)
	}

	// AES-256-GCM open. Nonce is the first 12 bytes (NonceSize) of encSection.
	block, err := aes.NewCipher(aesKey)
	if err != nil {
		return fmt.Errorf("aes: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return fmt.Errorf("gcm: %w", err)
	}
	if len(encSection) < gcm.NonceSize() {
		return errors.New("ciphertext too short")
	}
	nonce, ciphertext := encSection[:gcm.NonceSize()], encSection[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, headerJSON)
	if err != nil {
		return ErrBadPassphrase
	}

	// Overwrite path: wipe target root before extraction. Attestation already
	// happened upstream — this is the deletion that the BR=4 row attested to.
	if overwrite {
		if err := wipeDirContents(dstRoot); err != nil {
			return fmt.Errorf("wipe target: %w", err)
		}
	}

	return untarGz(plaintext, dstRoot)
}

// untarGz extracts a gzip'd tar stream into dstRoot. Path traversal (entries
// containing `..` or absolute paths) is rejected — defensive against a
// tampered ciphertext that survived GCM (should not happen, but cheap to
// guard).
func untarGz(blob []byte, dstRoot string) error {
	gz, err := gzip.NewReader(bytes.NewReader(blob))
	if err != nil {
		return fmt.Errorf("gunzip: %w", err)
	}
	defer func() { _ = gz.Close() }()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("tar next: %w", err)
		}
		clean := filepath.Clean(hdr.Name)
		if strings.HasPrefix(clean, "..") || filepath.IsAbs(clean) {
			return fmt.Errorf("tar entry escapes root: %q", hdr.Name)
		}
		target := filepath.Join(dstRoot, clean)
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(hdr.Mode)&0o700|0o700); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
				return err
			}
			f, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, os.FileMode(hdr.Mode)&0o600|0o600)
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, tr); err != nil {
				_ = f.Close()
				return err
			}
			if err := f.Close(); err != nil {
				return err
			}
		default:
			// Skip symlinks/devices/etc. — ~/.leah-state should be regular
			// files only; anything else is a corruption signal we ignore
			// rather than fail-loud on first-version archives.
		}
	}
}

// dirNonEmptyExcept reports whether dir has entries other than the named
// allowlist. A missing dir counts as empty (first-run import target). The
// allowlist exists because `stateDir()` lazily creates `audit.jsonl` before
// runImport reaches this check — counting that single bookkeeping file as
// "non-empty" would block legitimate first-run imports.
func dirNonEmptyExcept(dir string, allow ...string) (bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	skip := map[string]bool{}
	for _, s := range allow {
		skip[s] = true
	}
	for _, e := range entries {
		if !skip[e.Name()] {
			return true, nil
		}
	}
	return false, nil
}

// wipeDirContents removes every entry inside dir (but not dir itself) so the
// stateDir() invariant — directory exists, mode 0o700 — survives the wipe.
func wipeDirContents(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, e := range entries {
		if err := os.RemoveAll(filepath.Join(dir, e.Name())); err != nil {
			return err
		}
	}
	return nil
}

// importAttest is the BR=4 confirmation prompt. LEAH_IMPORT_AUTO_ATTEST=1
// short-circuits for tests + scripted invocation (sibling to
// LEAH_FORGET_AUTO_ATTEST / LEAH_PURGE_AUTO_ATTEST).
func importAttest() error {
	if v := os.Getenv("LEAH_IMPORT_AUTO_ATTEST"); v == "1" {
		return nil
	}
	_, _ = fmt.Fprintf(os.Stderr, "This will overwrite existing state. Confirm by typing: %s\n> ", importAttestPhrase)
	r := bufio.NewReader(os.Stdin)
	line, err := r.ReadString('\n')
	if err != nil && err != io.EOF {
		return fmt.Errorf("read attest: %w", err)
	}
	if strings.TrimRight(line, "\r\n") == importAttestPhrase {
		return nil
	}
	return ErrImportAttestationDenied
}
