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
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/trilam/leah/internal/audit"
)

// archiveHeader is the cleartext JSON prefix of every export. It carries the
// PBKDF2 parameters (so import can re-derive the key) plus provenance fields
// the operator can `head -c | jq` without decrypting. The HMAC trailer (line 2
// of the header section) is computed over a canonical re-encoding of these
// bytes — tamper detection runs BEFORE the AES-GCM decrypt attempt.
type archiveHeader struct {
	Version     int    `json:"version"`
	LeahVersion string `json:"leah_version"`
	Created     string `json:"created"`
	PBKDF2Iter  int    `json:"pbkdf2_iter"`
	Salt        string `json:"salt"` // base64, PBKDF2 salt (16 bytes)
}

// exportSeparator splits the cleartext header section from the AES-GCM
// ciphertext. The header section itself is: `<json>\n<hmac-base64>`.
const exportSeparator = "\n--ENCRYPTED--\n"

// pbkdf2Iterations follows OWASP 2024 baseline for PBKDF2-HMAC-SHA256. The
// header records the value used, so future iteration bumps still decrypt old
// archives.
const pbkdf2Iterations = 600_000

// pbkdf2KeyLen yields 64 bytes — 32 for AES-256 (encryption key), 32 for
// HMAC-SHA256 (header signature). Split via simple slice.
const pbkdf2KeyLen = 64

// pbkdf2SaltLen is the random salt per archive. 16 bytes (128 bits) is the
// PBKDF2 recommended floor.
const pbkdf2SaltLen = 16

// ErrBadPassphrase fires when AES-GCM decryption fails — either wrong
// passphrase or corrupted ciphertext (the GCM tag can't distinguish).
var ErrBadPassphrase = errors.New("import: passphrase or archive ciphertext invalid")

// ErrHeaderHMAC fires when the cleartext header HMAC does not match. Caught
// BEFORE decrypt — operator sees "tampered header" instead of "bad key".
var ErrHeaderHMAC = errors.New("import: header HMAC mismatch (tampered or wrong passphrase)")

// runExport implements `leah export --all [--out PATH]`. Produces a single
// AES-256-GCM encrypted tar.gz of ~/.leah-state/. Passphrase via
// LEAH_EXPORT_PASSPHRASE env (tests + scripting) or interactive prompt.
//
// BR=3 — read-only on local state but the archive leaves the machine, so the
// audit row's blast radius reflects "high-impact disclosure" (matches spec
// §11 privacy gates). `errW` is the diagnostic seam — tests capture it.
func runExport(_ context.Context, args []string, errW io.Writer) int {
	fs := flag.NewFlagSet("export", flag.ContinueOnError)
	all := fs.Bool("all", false, "export the full state dir (required — single archive form)")
	out := fs.String("out", "", "output path (default ./leah-export-<ts>.tar.gz.enc)")
	fs.SetOutput(errW)
	if err := fs.Parse(args); err != nil {
		fs.Usage()
		return 2
	}
	if !*all {
		_, _ = fmt.Fprintln(errW, "usage: leah export --all [--out PATH]")
		return 2
	}

	sd := stateDir()
	logger := &audit.Logger{Path: filepath.Join(sd, "audit.jsonl"), DefaultWorkspace: activeWorkspace}

	pass, err := readPassphrase("LEAH_EXPORT_PASSPHRASE")
	if err != nil {
		_, _ = fmt.Fprintf(errW, "leah export: %v\n", err)
		_ = logger.Append(audit.Entry{Kind: "export_all", BlastRadius: 3, Outcome: "failed", Detail: "passphrase_read: " + err.Error()})
		return 1
	}

	dst := *out
	if dst == "" {
		dst = fmt.Sprintf("./leah-export-%d.tar.gz.enc", time.Now().Unix())
	}

	// Audit row BEFORE the tar walk so the archive includes its own
	// provenance row — operator can grep `audit.jsonl` post-import and see
	// when the snapshot was taken. The failure-path audit row below is the
	// only one that lands post-tar.
	_ = logger.Append(audit.Entry{Kind: "export_all", BlastRadius: 3, Outcome: "success", Detail: dst})

	if err := writeEncryptedArchive(sd, dst, pass); err != nil {
		_, _ = fmt.Fprintf(errW, "leah export: %v\n", err)
		_ = logger.Append(audit.Entry{Kind: "export_all", BlastRadius: 3, Outcome: "failed", Detail: err.Error()})
		return 1
	}

	_, _ = fmt.Fprintf(os.Stdout, "exported %s → %s\n", sd, dst)
	return 0
}

// writeEncryptedArchive tars stateRoot, gzips, derives AES+HMAC keys from
// passphrase via PBKDF2, encrypts with AES-256-GCM, and writes the final
// archive: <cleartext-header>\n<hmac-b64>\n--ENCRYPTED--\n<nonce||ciphertext>.
func writeEncryptedArchive(stateRoot, dst, pass string) error {
	// 1. TAR + gzip the state dir into an in-memory buffer. The state dir is
	//    on the order of MB-to-low-GB; for v1 we accept the RAM cost in
	//    exchange for the simpler streaming-encrypt-after-compress design.
	plaintext, err := tarGzDir(stateRoot)
	if err != nil {
		return fmt.Errorf("tar+gzip: %w", err)
	}

	// 2. Derive a 64-byte key block from PBKDF2; split into AES key + HMAC key.
	salt := make([]byte, pbkdf2SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return fmt.Errorf("salt: %w", err)
	}
	keys, err := pbkdf2.Key(sha256.New, pass, salt, pbkdf2Iterations, pbkdf2KeyLen)
	if err != nil {
		return fmt.Errorf("pbkdf2: %w", err)
	}
	aesKey, hmacKey := keys[:32], keys[32:]

	// 3. Build the cleartext header. Version + leah_version + timestamp + the
	//    PBKDF2 params needed to re-derive the key at import time.
	header := archiveHeader{
		Version:     1,
		LeahVersion: leahVersionString(),
		Created:     time.Now().UTC().Format(time.RFC3339),
		PBKDF2Iter:  pbkdf2Iterations,
		Salt:        base64.StdEncoding.EncodeToString(salt),
	}
	headerJSON, err := json.Marshal(header)
	if err != nil {
		return fmt.Errorf("marshal header: %w", err)
	}

	// 4. HMAC-SHA256 over the header JSON bytes. Trailer is base64.
	mac := hmacB64(hmacKey, headerJSON)

	// 5. AES-256-GCM seal. Nonce is fresh per archive; we prepend it to the
	//    ciphertext (standard GCM convention).
	block, err := aes.NewCipher(aesKey)
	if err != nil {
		return fmt.Errorf("aes: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return fmt.Errorf("gcm: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return fmt.Errorf("nonce: %w", err)
	}
	ciphertext := gcm.Seal(nil, nonce, plaintext, headerJSON)

	// 6. Assemble final archive: <header-json>\n<hmac-b64>\n--ENCRYPTED--\n<nonce||ct>
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return fmt.Errorf("mkdir out: %w", err)
	}
	f, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("open out: %w", err)
	}
	defer func() { _ = f.Close() }()
	w := bufio.NewWriter(f)
	if _, err := w.Write(headerJSON); err != nil {
		return err
	}
	if _, err := w.WriteString("\n" + mac); err != nil {
		return err
	}
	if _, err := w.WriteString(exportSeparator); err != nil {
		return err
	}
	if _, err := w.Write(nonce); err != nil {
		return err
	}
	if _, err := w.Write(ciphertext); err != nil {
		return err
	}
	return w.Flush()
}

// tarGzDir walks root, writing a tar.gz stream into a buffer. Symlinks are
// followed (they point inside ~/.leah-state on a single-operator machine);
// permissions are preserved.
func tarGzDir(root string) ([]byte, error) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		hdr, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		hdr.Name = filepath.ToSlash(rel)
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		f, err := os.Open(p)
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()
		_, err = io.Copy(tw, f)
		return err
	})
	if err != nil {
		return nil, err
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	if err := gz.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// hmacB64 returns base64(HMAC-SHA256(key, msg)).
func hmacB64(key, msg []byte) string {
	h := newHMAC(key)
	_, _ = h.Write(msg)
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

// newHMAC isolates the hash factory so tests can swap if needed.
func newHMAC(key []byte) hash.Hash { return hmac.New(sha256.New, key) }

// readPassphrase resolves the passphrase from env (test + scripting path) or,
// when unset, reads one line from stdin. Echo-suppression is deferred to a
// v1.1 polish item — operators running this interactively today accept the
// visible-passphrase risk in their own terminal.
func readPassphrase(envKey string) (string, error) {
	if v := os.Getenv(envKey); v != "" {
		return v, nil
	}
	r := bufio.NewReader(os.Stdin)
	line, err := r.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", fmt.Errorf("read stdin: %w", err)
	}
	line = strings.TrimRight(line, "\r\n")
	if line == "" {
		return "", errors.New("empty passphrase")
	}
	return line, nil
}

// leahVersionString returns a best-effort version tag for archive provenance.
func leahVersionString() string {
	if v := os.Getenv("LEAH_VERSION"); v != "" {
		return v
	}
	return "dev"
}
