package a2a

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// PeerID is the hex-encoded SHA-256 fingerprint of a peer's Ed25519 public
// key. Stable across reconnects; canonical key for the a2a_peer table.
type PeerID string

// Fingerprint derives a PeerID from a public key.
func Fingerprint(pub ed25519.PublicKey) PeerID {
	h := sha256.Sum256(pub)
	return PeerID(hex.EncodeToString(h[:]))
}

// IdentityStore is the §5.4.1 contract for the per-device Ed25519 identity.
// Production implementation persists to macOS Keychain via the Swift bridge
// (T14.b); the Go slice ships a FileIdentityStore so the protocol layer is
// testable without Keychain access.
type IdentityStore interface {
	Load() (ed25519.PrivateKey, error)
	Save(key ed25519.PrivateKey) error
}

// EnsureIdentity returns the device identity, generating + persisting one on
// first call. Pairs with IdentityStore so the Keychain-backed Swift bridge
// can drop in without changing call sites.
func EnsureIdentity(s IdentityStore) (ed25519.PrivateKey, error) {
	key, err := s.Load()
	if err == nil && len(key) == ed25519.PrivateKeySize {
		return key, nil
	}
	if err != nil && !errors.Is(err, ErrIdentityMissing) {
		return nil, fmt.Errorf("a2a: load identity: %w", err)
	}
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("a2a: generate identity: %w", err)
	}
	if err := s.Save(priv); err != nil {
		return nil, fmt.Errorf("a2a: save identity: %w", err)
	}
	return priv, nil
}

// ErrIdentityMissing signals a store has no identity yet; callers respond by
// generating one. Distinguished from "store broken" so EnsureIdentity does
// not silently mint a new key when the on-disk key is corrupt.
var ErrIdentityMissing = errors.New("a2a: identity not found")

// FileIdentityStore persists the identity to disk at Path. Keychain is the
// production target on macOS; the file store keeps tests + Linux dev hermetic.
type FileIdentityStore struct {
	Path string
}

// Load reads the private key from Path.
func (f *FileIdentityStore) Load() (ed25519.PrivateKey, error) {
	b, err := os.ReadFile(f.Path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrIdentityMissing
		}
		return nil, err
	}
	if len(b) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("a2a: identity file has %d bytes, want %d", len(b), ed25519.PrivateKeySize)
	}
	return ed25519.PrivateKey(b), nil
}

// Save writes the private key with 0600 perms.
func (f *FileIdentityStore) Save(key ed25519.PrivateKey) error {
	if err := os.MkdirAll(filepath.Dir(f.Path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(f.Path, key, 0o600)
}

// Prove signs nonce with priv — §5.4.1 identity proof.
func Prove(priv ed25519.PrivateKey, nonce []byte) []byte {
	return ed25519.Sign(priv, nonce)
}

// Verify returns nil when sig is a valid signature of nonce under pub.
func Verify(pub ed25519.PublicKey, nonce, sig []byte) error {
	if !ed25519.Verify(pub, nonce, sig) {
		return ErrSignatureInvalid
	}
	return nil
}

// ErrSignatureInvalid is returned when Verify rejects a peer's identity proof.
// Spec §5.8: drop the session, log, do not retry.
var ErrSignatureInvalid = errors.New("a2a: signature invalid")

// NewNonce returns a 32-byte challenge for identity.prove. Random per peering.
func NewNonce() ([]byte, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	return b, nil
}
