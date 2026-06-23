package pair

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"time"
)

// NewMTLSConfig derives a self-signed leaf cert deterministically from the
// shared key plus a per-connection ECDSA ephemeral, then pins the verifier
// to a SHA-256 fingerprint of (sharedKey || peer-cert-pubkey-bytes). The
// fingerprint comparison is constant-time. Trust originates entirely in
// possession of `sharedKey`: a peer that lacks the key cannot produce a cert
// whose fingerprint matches, and Bonjour-record spoofing alone does not
// help an attacker.
func NewMTLSConfig(sharedKey [32]byte) (*tls.Config, error) {
	cert, err := selfSignedCert(sharedKey)
	if err != nil {
		return nil, fmt.Errorf("pair: cert: %w", err)
	}
	expected := PinFingerprint(sharedKey)
	verify := func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
		if len(rawCerts) == 0 {
			return errors.New("pair: peer presented no certificate")
		}
		leaf, err := x509.ParseCertificate(rawCerts[0])
		if err != nil {
			return fmt.Errorf("pair: peer cert parse: %w", err)
		}
		spki, err := x509.MarshalPKIXPublicKey(leaf.PublicKey)
		if err != nil {
			return fmt.Errorf("pair: peer pubkey marshal: %w", err)
		}
		got := fingerprintParts(sharedKey, spki)
		if subtle.ConstantTimeCompare(got[:], expected[:]) != 1 {
			return ErrUnpinnedCert
		}
		return nil
	}
	return &tls.Config{
		Certificates:          []tls.Certificate{cert},
		ClientAuth:            tls.RequireAnyClientCert,
		InsecureSkipVerify:    true, // pin via VerifyPeerCertificate; CA chain irrelevant.
		VerifyPeerCertificate: verify,
		MinVersion:            tls.VersionTLS13,
	}, nil
}

// ErrUnpinnedCert is returned when a peer presents a cert whose fingerprint
// does not match the shared-key pin — i.e. a MITM that lacks the secret.
var ErrUnpinnedCert = errors.New("pair: peer cert fingerprint does not match shared key")

// PinFingerprint computes the expected SHA-256 over (sharedKey || own-spki).
// Exposed so callers can persist it in sync_peer.fingerprint per the §2.5
// CRDT schema.
func PinFingerprint(sharedKey [32]byte) [32]byte {
	cert, err := selfSignedCert(sharedKey)
	if err != nil {
		// selfSignedCert only fails on crypto/rand starvation, which is
		// the same supervisor-restart class as GenerateOTP.
		panic(fmt.Sprintf("pair: PinFingerprint: %v", err))
	}
	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		panic(fmt.Sprintf("pair: PinFingerprint parse: %v", err))
	}
	spki, err := x509.MarshalPKIXPublicKey(leaf.PublicKey)
	if err != nil {
		panic(fmt.Sprintf("pair: PinFingerprint spki: %v", err))
	}
	return fingerprintParts(sharedKey, spki)
}

func fingerprintParts(sharedKey [32]byte, spki []byte) [32]byte {
	h := sha256.New()
	h.Write(sharedKey[:])
	h.Write(spki)
	var out [32]byte
	copy(out[:], h.Sum(nil))
	return out
}

func selfSignedCert(sharedKey [32]byte) (tls.Certificate, error) {
	// Derive a deterministic key from sharedKey so two daemons holding the
	// same secret converge on identical certs — fingerprint match is then
	// the operator's intent, not a coincidence of randomness.
	// crypto/ecdsa.GenerateKey calls randutil.MaybeReadByte which is
	// non-deterministic at process start; build the key directly instead.
	priv, err := deriveECDSAKey(sharedKey)
	if err != nil {
		return tls.Certificate{}, err
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "leah-sync"},
		NotBefore:    time.Unix(1_700_000_000, 0),
		NotAfter:     time.Unix(4_700_000_000, 0),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		DNSNames:     []string{"leah-sync.local"},
		IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		return tls.Certificate{}, err
	}
	keyDER, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return tls.Certificate{}, err
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	return tls.X509KeyPair(certPEM, keyPEM)
}

// deriveECDSAKey constructs a P-256 private key whose scalar is derived
// deterministically from sharedKey via SHA-256-CTR. crypto/ecdsa.GenerateKey
// can't be used here because randutil.MaybeReadByte introduces process-start
// non-determinism — two daemons holding the same secret must converge on
// the same key for the fingerprint pin to mean "shared secret".
func deriveECDSAKey(sharedKey [32]byte) (*ecdsa.PrivateKey, error) {
	curve := elliptic.P256()
	n := curve.Params().N
	var counter uint64
	for {
		h := sha256.New()
		h.Write(sharedKey[:])
		var ctr [8]byte
		for i := 0; i < 8; i++ {
			ctr[7-i] = byte(counter >> (8 * i))
		}
		h.Write(ctr[:])
		d := new(big.Int).SetBytes(h.Sum(nil))
		if d.Sign() > 0 && d.Cmp(n) < 0 {
			priv := new(ecdsa.PrivateKey)
			priv.D = d
			priv.Curve = curve
			priv.X, priv.Y = curve.ScalarBaseMult(d.Bytes()) //nolint:staticcheck // ecdh API doesn't expose the X/Y needed for x509 marshalling
			return priv, nil
		}
		counter++
		if counter > 1<<20 {
			return nil, errors.New("pair: ECDSA scalar derivation exceeded retry budget")
		}
	}
}
