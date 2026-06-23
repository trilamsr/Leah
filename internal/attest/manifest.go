package attest

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// pluginManifest is the on-disk format inside every plugin bundle.
// Signed payload is the JSON-encoded Files list (canonical ed25519 over the
// raw json.Marshal output of Files) — pubkey signs the file set, not the
// outer envelope, so manifest cosmetic fields can change without re-signing.
type pluginManifest struct {
	Files  []manifestFile `json:"files"`
	Pubkey string         `json:"pubkey"` // hex ed25519 public key
	Sig    string         `json:"sig"`    // hex ed25519 signature over Files
}

type manifestFile struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

func (v *verifier) VerifyPlugin(ctx context.Context, bundlePath string) (Attestation, error) {
	now := v.cfg.Clock()
	att := Attestation{
		Subject:     "plugin:" + bundlePath,
		VerifiedAt:  now,
		NextRecheck: now.Add(pluginRecheck),
	}
	info, err := os.Stat(bundlePath)
	if err != nil || !info.IsDir() {
		att.State = Failed
		if err != nil {
			att.Reason = err.Error()
		} else {
			att.Reason = errBundleMissing.Error()
		}
		return v.publish(att), nil
	}
	mf, err := readPluginManifest(bundlePath)
	if err != nil {
		att.State = Failed
		att.Reason = fmt.Sprintf("manifest: %v", err)
		return v.publish(att), nil
	}
	if v.cfg.Revocations != nil && v.cfg.Revocations.IsRevoked(mf.Pubkey) {
		att.State = Failed
		att.Reason = "plugin pubkey revoked"
		return v.publish(att), nil
	}
	pub, err := hex.DecodeString(mf.Pubkey)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		att.State = Failed
		att.Reason = "invalid plugin pubkey"
		return v.publish(att), nil
	}
	sig, err := hex.DecodeString(mf.Sig)
	if err != nil || len(sig) != ed25519.SignatureSize {
		att.State = Failed
		att.Reason = "invalid plugin signature encoding"
		return v.publish(att), nil
	}
	body, err := json.Marshal(mf.Files)
	if err != nil {
		att.State = Failed
		att.Reason = fmt.Sprintf("marshal files: %v", err)
		return v.publish(att), nil
	}
	if !ed25519.Verify(pub, body, sig) {
		att.State = Failed
		att.Reason = "plugin signature invalid"
		return v.publish(att), nil
	}
	for _, f := range mf.Files {
		got, err := hashFile(ctx, filepath.Join(bundlePath, f.Path))
		if err != nil {
			att.State = Failed
			att.Reason = fmt.Sprintf("hash %s: %v", f.Path, err)
			return v.publish(att), nil
		}
		if got != f.SHA256 {
			att.State = Failed
			att.Reason = fmt.Sprintf("digest mismatch for %s", f.Path)
			return v.publish(att), nil
		}
	}
	att.SignedBy = []SignerRef{{Kind: "ed25519", Fingerprint: mf.Pubkey}}
	att.State = Verified
	if v.cfg.Revocations != nil && v.cfg.Revocations.Stale(now) {
		att.State = Stale
		att.Reason = "revocation list >7d offline"
	}
	return v.publish(att), nil
}

func readPluginManifest(bundlePath string) (pluginManifest, error) {
	raw, err := os.ReadFile(filepath.Join(bundlePath, "manifest.json"))
	if err != nil {
		return pluginManifest{}, err
	}
	var mf pluginManifest
	if err := json.Unmarshal(raw, &mf); err != nil {
		return pluginManifest{}, err
	}
	if mf.Pubkey == "" || mf.Sig == "" {
		return pluginManifest{}, errors.New("manifest missing pubkey or sig")
	}
	return mf, nil
}
