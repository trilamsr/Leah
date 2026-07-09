// Package attest implements continuous runtime integrity verification of the
// daemon binary, ML model files, plugin bundles, and the Sparkle appcast.
// Verdict drives behavior: Failed on self blocks watchdog restart; Failed on
// plugin blocks plugin load; Stale only warns.
package attest

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

type AttestState string

const (
	Verified AttestState = "Verified"
	Stale    AttestState = "Stale"
	Failed   AttestState = "Failed"
	Unknown  AttestState = "Unknown"
)

type SignerRef struct {
	Kind        string `json:"kind"`
	Fingerprint string `json:"fingerprint"`
}

type Attestation struct {
	Subject     string      `json:"subject"`
	State       AttestState `json:"state"`
	SignedBy    []SignerRef `json:"signed_by"`
	VerifiedAt  time.Time   `json:"verified_at"`
	NextRecheck time.Time   `json:"next_recheck"`
	Reason      string      `json:"reason,omitempty"`
}

type ManifestRef struct {
	Path           string
	ExpectedSHA256 string
	Pubkey         ed25519.PublicKey
}

type Verifier interface {
	VerifySelf(ctx context.Context) (Attestation, error)
	VerifyArtifact(ctx context.Context, path string, mf ManifestRef) (Attestation, error)
	VerifyPlugin(ctx context.Context, bundlePath string) (Attestation, error)
	LastVerdict() Attestation
	Subscribe() <-chan Attestation
}

type Config struct {
	SelfPath           string
	SelfExpectedSHA256 string
	SelfSignedBy       []SignerRef
	Revocations        *RevocationList
	Clock              func() time.Time
}

type verifier struct {
	cfg  Config
	mu   sync.Mutex
	last Attestation
	subs []chan Attestation
}

func NewVerifier(cfg Config) Verifier {
	if cfg.Clock == nil {
		cfg.Clock = time.Now
	}
	return &verifier{cfg: cfg, last: Attestation{State: Unknown}}
}

func (v *verifier) VerifySelf(ctx context.Context) (Attestation, error) {
	now := v.cfg.Clock()
	att := Attestation{
		Subject:     "self:" + v.cfg.SelfPath,
		VerifiedAt:  now,
		NextRecheck: now.Add(selfRecheck),
		SignedBy:    v.cfg.SelfSignedBy,
	}
	if v.cfg.SelfPath == "" || v.cfg.SelfExpectedSHA256 == "" {
		att.State = Unknown
		att.Reason = "self path or expected digest unset"
		return v.publish(att), nil
	}
	got, err := hashFile(ctx, v.cfg.SelfPath)
	if err != nil {
		att.State = Failed
		att.Reason = fmt.Sprintf("read self: %v", err)
		return v.publish(att), nil
	}
	if got != v.cfg.SelfExpectedSHA256 {
		att.State = Failed
		att.Reason = fmt.Sprintf("digest mismatch: got=%s want=%s", got, v.cfg.SelfExpectedSHA256)
		return v.publish(att), nil
	}
	att.State = Verified
	return v.publish(att), nil
}

func (v *verifier) VerifyArtifact(ctx context.Context, path string, mf ManifestRef) (Attestation, error) {
	now := v.cfg.Clock()
	att := Attestation{
		Subject:     "artifact:" + path,
		VerifiedAt:  now,
		NextRecheck: now.Add(artifactRecheck),
	}
	if mf.ExpectedSHA256 == "" {
		att.State = Unknown
		att.Reason = "manifest digest empty"
		return v.publish(att), nil
	}
	got, err := hashFile(ctx, path)
	if err != nil {
		att.State = Failed
		att.Reason = fmt.Sprintf("read artifact: %v", err)
		return v.publish(att), nil
	}
	if got != mf.ExpectedSHA256 {
		att.State = Failed
		att.Reason = fmt.Sprintf("digest mismatch: got=%s want=%s", got, mf.ExpectedSHA256)
		return v.publish(att), nil
	}
	if len(mf.Pubkey) == ed25519.PublicKeySize {
		att.SignedBy = []SignerRef{{Kind: "ed25519", Fingerprint: hex.EncodeToString(mf.Pubkey)}}
	}
	att.State = Verified
	return v.publish(att), nil
}

func (v *verifier) LastVerdict() Attestation {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.last
}

func (v *verifier) Subscribe() <-chan Attestation {
	ch := make(chan Attestation, subscriberBuffer)
	v.mu.Lock()
	v.subs = append(v.subs, ch)
	v.mu.Unlock()
	return ch
}

func (v *verifier) publish(att Attestation) Attestation {
	v.mu.Lock()
	v.last = att
	subs := append([]chan Attestation(nil), v.subs...)
	v.mu.Unlock()
	for _, ch := range subs {
		// Non-blocking — UI subscribers process at their own cadence; the
		// authoritative verdict already lives in LastVerdict().
		select {
		case ch <- att:
		default:
		}
	}
	return att
}

func hashFile(ctx context.Context, path string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

var errBundleMissing = errors.New("plugin bundle missing")
